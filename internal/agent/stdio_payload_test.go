package agent_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shotah/george/internal/agent"
	"github.com/shotah/george/internal/channel/stdio"
	"github.com/shotah/george/internal/provider"
)

// Frozen Pacific noon so testdata/stdio/*.txt is a readable Completer dump,
// not a moving NOW. Production still uses time.Now.
func payloadClock() (loc *time.Location, now time.Time) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return loc, time.Date(2026, time.September, 14, 12, 2, 0, 0, loc)
}

func formatCompleterRequest(req provider.Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tools: %d\n", len(req.Tools))
	for i, m := range req.Messages {
		fmt.Fprintf(&b, "\n## [%d] %s\n%s\n", i, m.Role, m.Content)
		for _, u := range m.ImageURLs {
			fmt.Fprintf(&b, "image: %s\n", u)
		}
	}
	return b.String()
}

// updateGoldens rewrites testdata/stdio/*.txt from the current run:
// go test ./internal/agent/ -run Payload -update. Read the diff before
// trusting it — the golden is the contract.
var updateGoldens = flag.Bool("update", false, "rewrite Completer payload goldens")

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	if *updateGoldens {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v\n--- got (Completer payload) ---\n%s", path, err, got)
	}
	if string(want) != got {
		t.Fatalf("completer payload mismatch\n--- want (%s) ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

// wireBody is the slice of the chat.completions request the goldens pin.
type wireBody struct {
	Model    string            `json:"model"`
	Messages []wireMessage     `json:"messages"`
	Tools    []json.RawMessage `json:"tools"`
}

type wireMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// text is the message content whether the SDK sent a string or content parts.
func (m wireMessage) text() string {
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return string(m.Content)
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// format mirrors formatCompleterRequest so the wire can be diffed against
// the agent-side goldens byte for byte.
func (b wireBody) format() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "tools: %d\n", len(b.Tools))
	for i, m := range b.Messages {
		fmt.Fprintf(&sb, "\n## [%d] %s\n%s\n", i, m.Role, m.text())
	}
	return sb.String()
}

// stdioInbound is the coding turn the stdio goldens pin: one line on stdin.
const stdioInbound = "greet.txt says hi. Change that line to hello, then run wc -l greet.txt.\n"

// stdioTurn runs stdioInbound through the real REPL into a fresh agent and
// returns what the Completer was asked. testdata/stdio/*.txt is that request.
func stdioTurn(t *testing.T, completer provider.Completer, model string) {
	t.Helper()
	loc, now := payloadClock()
	a, err := agent.New(agent.Options{
		Persona:   "You are Kit.",
		Completer: completer,
		Sessions:  newMemHistory(),
		Model:     model,
		Location:  loc,
		TZName:    "America/Los_Angeles",
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := &stdio.Channel{In: strings.NewReader(stdioInbound), Out: io.Discard, Err: io.Discard}
	if err := ch.Run(context.Background(), a.Handle); err != nil {
		t.Fatal(err)
	}
}

func TestStdioInbound_CompleterPayload(t *testing.T) {
	var captured provider.Request
	stdioTurn(t, &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		captured = req
		return &provider.Result{Content: "ok"}, nil
	}}, "m")
	assertGolden(t, filepath.Join("testdata", "stdio", "completer.txt"), formatCompleterRequest(captured))
}

// End to end: stdin line → Handle → real provider.Client → the
// OpenAI-compat HTTP body, diffed against the same golden.
func TestStdioInbound_WireBody(t *testing.T) {
	var body wireBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "x",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	const model = "qwen3-coder:30b-a3b-q4_K_M"
	stdioTurn(t, provider.New(srv.URL, "k", model), model)
	assertGolden(t, filepath.Join("testdata", "stdio", "completer.txt"), body.format())
}
