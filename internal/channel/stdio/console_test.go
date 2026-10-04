package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/briandowns/spinner"
)

// testConsole is a fancy Console whose spinner never draws (its writer is
// not a terminal), so only the persistent lines and the reply land.
func testConsole(t *testing.T) (*Console, *bytes.Buffer, *os.File) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = out.Close() })
	var errOut bytes.Buffer
	return &Console{
		out:  out,
		err:  &errOut,
		spin: spinner.New(spinner.CharSets[14], time.Millisecond, spinner.WithWriter(&errOut)),
	}, &errOut, out
}

func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestConsoleStream_ToolLinesAndOneReply(t *testing.T) {
	con, errOut, out := testConsole(t)
	ctx := context.Background()
	s := newConsoleStream(con)

	s.ToolStart(ctx, "fs__file_get", json.RawMessage(`{"path":"nope.go"}`))
	s.ToolDone(ctx, "fs__file_get", json.RawMessage(`{"path":"nope.go"}`), errors.New("open nope.go: no such file\nmore"))
	s.ToolStart(ctx, "shell__command_run", json.RawMessage(`{"command":"go vet ./..."}`))
	s.ToolDone(ctx, "shell__command_run", json.RawMessage(`{"command":"go vet ./..."}`), nil)
	_ = s.Update(ctx, "Reading it now.")
	_ = s.Update(ctx, "It is gone.")

	lines := errOut.String()
	if !strings.Contains(lines, "✗ read nope.go: open nope.go: no such file …") {
		t.Fatalf("missing failure line: %q", lines)
	}
	if !strings.Contains(lines, "✓ run go vet ./...") {
		t.Fatalf("missing done line: %q", lines)
	}
	if got := readAll(t, out); got != "" {
		t.Fatalf("reply printed before Finish: %q", got)
	}

	_ = s.Finish(ctx, "It is gone.")
	_ = s.Finish(ctx, "It is gone.")
	if got := readAll(t, out); got != "It is gone.\n" {
		t.Fatalf("reply = %q, want it once", got)
	}
}

func TestConsoleStream_FinishFallsBackToStreamed(t *testing.T) {
	con, _, out := testConsole(t)
	s := newConsoleStream(con)
	_ = s.Update(context.Background(), "streamed")
	_ = s.Finish(context.Background(), "")
	if got := readAll(t, out); got != "streamed\n" {
		t.Fatalf("reply = %q", got)
	}
}

func TestConsoleStream_StatusDropsHourglass(t *testing.T) {
	con, _, _ := testConsole(t)
	s := newConsoleStream(con)
	_ = s.UpdateStatus(context.Background(), "⏳ daisy, daisy…")
	if got := con.spin.Suffix; got != " daisy, daisy…" {
		t.Fatalf("spinner text = %q", got)
	}
	_ = s.UpdateStatus(context.Background(), "")
	if got := con.spin.Suffix; got != " thinking…" {
		t.Fatalf("cleared spinner text = %q", got)
	}
}

func TestConsoleStream_AbortPrintsNothing(t *testing.T) {
	con, _, out := testConsole(t)
	s := newConsoleStream(con)
	_ = s.Update(context.Background(), "half a reply")
	s.abort()
	_ = s.Finish(context.Background(), "late")
	if got := readAll(t, out); got != "" {
		t.Fatalf("aborted turn printed %q", got)
	}
}

func TestConsole_PlainPassesThrough(t *testing.T) {
	var errOut bytes.Buffer
	con := &Console{err: &errOut}
	if con.Fancy() || (*Console)(nil).Fancy() {
		t.Fatal("plain console claims fancy")
	}
	_, _ = con.Write([]byte("log line\n"))
	con.say(&errOut, true, "error: boom")
	if got := errOut.String(); got != "log line\nerror: boom\n" {
		t.Fatalf("plain output = %q", got)
	}
}

func TestToolLabel(t *testing.T) {
	for _, tc := range []struct {
		name, args, want string
	}{
		{"fs__file_get", `{"path":"readme.md"}`, "read readme.md"},
		{"fs__file_patch", `{"path":"a.go","old":"x"}`, "edit a.go"},
		{"shell__command_run", `{"command":"go test ./...\ngo vet"}`, "run go test ./... …"},
		{"git__status_get", `{}`, "status get"},
		{"memory_search", `{"query":"bridges"}`, "memory search bridges"},
		{"fs__file_get", `not json`, "read"},
	} {
		if got := toolLabel(tc.name, json.RawMessage(tc.args)); got != tc.want {
			t.Errorf("toolLabel(%s, %s) = %q, want %q", tc.name, tc.args, got, tc.want)
		}
	}
}
