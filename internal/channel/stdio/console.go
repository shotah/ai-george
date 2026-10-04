package stdio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Console owns the terminal's stderr: a spinner naming what the turn is
// doing, a dim line per finished tool call, and log lines printed around
// both so they never tear the spinner line.
type Console struct {
	mu       sync.Mutex
	out      *os.File
	err      io.Writer
	spin     *spinner.Spinner // nil unless stdout and stderr are terminals
	spinning bool
	dim, bad lipgloss.Style
	accent   lipgloss.Style
}

// NewConsole binds a Console to the process's stdout and stderr. Off a
// terminal it only passes writes through.
func NewConsole(out, errOut *os.File) *Console {
	c := &Console{out: out, err: errOut}
	if term.IsTerminal(int(out.Fd())) && term.IsTerminal(int(errOut.Fd())) {
		c.spin = spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithWriterFile(errOut))
		re := lipgloss.NewRenderer(errOut)
		c.dim = re.NewStyle().Faint(true)
		c.bad = re.NewStyle().Foreground(lipgloss.Color("1"))
		c.accent = re.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	}
	return c
}

// banner opens the session: name, what it runs on, and how to drive it.
func (c *Console) banner(w io.Writer, detail string) {
	_, _ = fmt.Fprintf(w, "%s %s\n%s\n\n",
		c.accent.Render("george"),
		c.dim.Render(detail),
		c.dim.Render("/help for commands · Ctrl-C cancels a turn, or quits at the prompt"))
}

// Fancy reports whether the console draws a spinner and styled lines.
func (c *Console) Fancy() bool { return c != nil && c.spin != nil }

// Write prints p (a log line) on stderr, lifting the spinner around it.
func (c *Console) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.above(func() (int, error) { return c.err.Write(p) })
}

// above runs write with the spinner line cleared, then puts it back.
func (c *Console) above(write func() (int, error)) (int, error) {
	if !c.spinning {
		return write()
	}
	c.spin.Stop()
	defer c.spin.Start()
	return write()
}

// status sets the spinner text, starting it if needed.
func (c *Console) status(note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spin.Lock()
	c.spin.Suffix = " " + note
	c.spin.Unlock()
	if !c.spinning {
		c.spin.Start()
		c.spinning = true
	}
}

// line prints one persistent line above the spinner.
func (c *Console) line(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = c.above(func() (int, error) { return fmt.Fprintln(c.err, s) })
}

// stop takes the spinner down.
func (c *Console) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.spinning {
		c.spin.Stop()
		c.spinning = false
	}
}

// say prints a turn outcome on w: dim, or red when bad, on a fancy console.
func (c *Console) say(w io.Writer, bad bool, s string) {
	if c.Fancy() {
		if bad {
			s = c.bad.Render(s)
		} else {
			s = c.dim.Render(s)
		}
	}
	_, _ = fmt.Fprintln(w, s)
}

// consoleStream is one turn on a Console. It holds the reply back while the
// spinner runs and prints it once, whole, at Finish.
type consoleStream struct {
	con     *Console
	mu      sync.Mutex
	text    string
	note    string   // spin-up status from the agent
	running []string // labels of tool calls in flight
	started bool
	done    bool
}

func newConsoleStream(con *Console) *consoleStream {
	s := &consoleStream{con: con}
	s.refresh()
	return s
}

// refresh puts the most specific thing going on in the spinner. Callers hold
// s.mu, except newConsoleStream.
func (s *consoleStream) refresh() {
	if s.done {
		return
	}
	note := "thinking…"
	switch {
	case len(s.running) > 0:
		note = s.running[len(s.running)-1] + "…"
	case s.note != "":
		note = s.note
	case s.text != "":
		note = "writing…"
	}
	s.con.status(note)
}

func (s *consoleStream) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

func (s *consoleStream) Update(_ context.Context, fullText string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	s.text = fullText
	s.refresh()
	return nil
}

func (s *consoleStream) UpdateStatus(_ context.Context, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// The spinner is the wait marker; the note's own hourglass doubles it.
	s.note = strings.TrimPrefix(note, "⏳ ")
	s.refresh()
	return nil
}

func (s *consoleStream) ToolStart(_ context.Context, name string, args json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = append(s.running, toolLabel(name, args))
	s.refresh()
}

func (s *consoleStream) ToolDone(_ context.Context, name string, args json.RawMessage, err error) {
	label := toolLabel(name, args)
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := slices.Index(s.running, label); i >= 0 {
		s.running = slices.Delete(s.running, i, i+1)
	}
	if err != nil {
		s.con.line(s.con.bad.Render("✗ " + label + ": " + clip(firstLine(err.Error()), 80)))
	} else {
		s.con.line(s.con.dim.Render("✓ " + label))
	}
	s.refresh()
}

// Finish takes the spinner down and prints the reply once; later calls are
// no-ops. An empty final falls back to the last streamed text.
func (s *consoleStream) Finish(_ context.Context, final string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return nil
	}
	s.done = true
	s.con.stop()
	if final == "" {
		final = s.text
	}
	if final == "" {
		return nil
	}
	_, err := fmt.Fprintln(s.con.out, s.con.render(final))
	return err
}

// render formats markdown for the terminal, set off by a blank line each
// side, falling back to the raw text.
// The style is fixed (GLAMOUR_STYLE overrides "dark") because auto-detect
// queries the terminal and can clash with the prompt's raw-mode input.
func (c *Console) render(md string) string {
	if c.spin == nil || !term.IsTerminal(int(c.out.Fd())) {
		return md
	}
	width := 80
	if w, _, err := term.GetSize(int(c.out.Fd())); err == nil && w > 0 {
		width = min(w, 100)
	}
	style := os.Getenv("GLAMOUR_STYLE")
	if style == "" {
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(glamour.WithStylePath(style), glamour.WithWordWrap(width-4))
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return "\n" + strings.Trim(out, "\n") + "\n"
}

// abort takes the spinner down without printing a reply.
func (s *consoleStream) abort() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	s.con.stop()
}

// toolVerbs names the common tools the way a person would.
var toolVerbs = map[string]string{
	"file_get":    "read",
	"file_patch":  "edit",
	"file_create": "write",
	"command_run": "run",
}

// toolLabel is a short "verb target" for a tool call, e.g. "read main.go".
func toolLabel(name string, args json.RawMessage) string {
	_, verb, ok := strings.Cut(name, "__")
	if !ok {
		verb = name
	}
	if v, ok := toolVerbs[verb]; ok {
		verb = v
	} else {
		verb = strings.ReplaceAll(verb, "_", " ")
	}
	var m map[string]any
	_ = json.Unmarshal(args, &m)
	for _, k := range []string{"command", "path", "query", "pattern", "url", "name"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return verb + " " + clip(firstLine(v), 60)
		}
	}
	return verb
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
