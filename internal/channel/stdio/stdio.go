// Package stdio implements a REPL channel for local development.
package stdio

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"golang.org/x/term"

	"github.com/shotah/george/internal/channel"
	"github.com/shotah/george/internal/slash"
)

const userID = "local"

// Channel is an interactive line-oriented REPL on stdin/stdout.
type Channel struct {
	In            io.Reader
	Out           io.Writer
	Err           io.Writer
	StreamReplies bool
	// SessionID is the conversation every line belongs to (the repo id).
	// Empty is channel.AgentSession.
	SessionID string
}

// New returns a Channel bound to process stdio.
func New() *Channel {
	return &Channel{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
}

// Run reads messages, invokes handle, and prints replies until EOF, /quit,
// or ctx cancel. On a terminal a bracketed paste is one message and Ctrl-C
// at the prompt exits; piped input is one message per line. Ctrl-C during a
// turn cancels that turn only.
func (c *Channel) Run(ctx context.Context, handle channel.Handler) error {
	in := c.In
	if in == nil {
		in = os.Stdin
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := c.Err
	if errOut == nil {
		errOut = os.Stderr
	}
	sessionID := c.SessionID
	if sessionID == "" {
		sessionID = channel.AgentSession
	}

	_, _ = fmt.Fprintln(errOut, slash.ReadyLine())

	read := c.lineReader(in, out)
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		var done func()
		read, done = terminalReader(f, out)
		defer done()
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		text, err := read()
		if err == io.EOF {
			_, _ = fmt.Fprintln(out)
			return nil
		}
		if err != nil {
			return fmt.Errorf("stdio: read: %w", err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		switch strings.ToLower(text) {
		case "/quit", "/exit", "/q":
			return nil
		}
		if stop := c.turn(ctx, handle, out, errOut, channel.Message{
			SessionID: sessionID,
			UserID:    userID,
			Text:      text,
		}); stop {
			return nil
		}
	}
}

// turn runs one message. A SIGINT while it runs cancels this turn's context
// and nothing else; stop is true when the whole channel should end.
func (c *Channel) turn(ctx context.Context, handle channel.Handler, out, errOut io.Writer, msg channel.Message) (stop bool) {
	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	defer signal.Stop(sigs)
	go func() {
		select {
		case <-sigs:
			cancel()
		case <-turnCtx.Done():
		}
	}()

	var stream *printStream
	handleCtx := turnCtx
	if c.StreamReplies {
		stream = newPrintStream(out)
		handleCtx = channel.WithReplyWriter(turnCtx, stream)
	}
	reply, err := handle(handleCtx, msg)
	if ctx.Err() != nil {
		return true
	}
	if turnCtx.Err() != nil {
		if stream != nil && stream.Started() {
			_, _ = fmt.Fprintln(out)
		}
		_, _ = fmt.Fprintln(errOut, "cancelled")
		return false
	}
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "error: %v\n", err)
		return false
	}
	if stream != nil && stream.Started() {
		_ = stream.Finish(ctx, reply)
		return false
	}
	if reply != "" {
		_, _ = fmt.Fprintln(out, reply)
	}
	return false
}

// lineReader is the piped path: one message per line.
func (c *Channel) lineReader(in io.Reader, out io.Writer) func() (string, error) {
	scanner := bufio.NewScanner(in)
	// Allow long paste dumps in the REPL.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return func() (string, error) {
		_, _ = fmt.Fprint(out, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", io.EOF
		}
		return scanner.Text(), nil
	}
}

// terminalReader puts the terminal in raw mode only while a message is being
// typed, so Ctrl-C during a turn is a SIGINT and at the prompt is EOF.
func terminalReader(f *os.File, out io.Writer) (read func() (string, error), done func()) {
	fd := int(f.Fd())
	t := term.NewTerminal(struct {
		io.Reader
		io.Writer
	}{f, out}, "> ")
	t.SetBracketedPasteMode(true)
	read = func() (string, error) {
		old, err := term.MakeRaw(fd)
		if err != nil {
			return "", err
		}
		defer func() { _ = term.Restore(fd, old) }()
		return readMessage(t)
	}
	return read, func() { t.SetBracketedPasteMode(false) }
}

// lineEditor is the part of *term.Terminal readMessage drives.
type lineEditor interface {
	ReadLine() (string, error)
	SetPrompt(string)
}

// readMessage returns one message: the lines of a bracketed paste, plus
// whatever is typed after it, sent by the next Enter.
func readMessage(t lineEditor) (string, error) {
	var pasted []string
	for {
		if len(pasted) > 0 {
			t.SetPrompt("… ")
		} else {
			t.SetPrompt("> ")
		}
		line, err := t.ReadLine()
		if errors.Is(err, term.ErrPasteIndicator) {
			pasted = append(pasted, line)
			continue
		}
		if err != nil {
			return "", err
		}
		if len(pasted) == 0 {
			return line, nil
		}
		if line != "" {
			pasted = append(pasted, line)
		}
		return strings.Join(pasted, "\n"), nil
	}
}
