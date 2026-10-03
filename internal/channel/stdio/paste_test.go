package stdio

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"golang.org/x/term"
)

// fakeTTY feeds keystrokes to a term.Terminal and swallows its echo.
func fakeTTY(keys string) *term.Terminal {
	return term.NewTerminal(struct {
		io.Reader
		io.Writer
	}{strings.NewReader(keys), &bytes.Buffer{}}, "> ")
}

func TestReadMessage_FortyLinePasteIsOneMessage(t *testing.T) {
	var lines []string
	for i := range 40 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	keys := "\x1b[200~" + strings.Join(lines, "\r") + "\r\x1b[201~" + "\r"
	got, err := readMessage(fakeTTY(keys))
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(lines, "\n"); got != want {
		t.Fatalf("got %d lines, want 40:\n%s", strings.Count(got, "\n")+1, got)
	}
}

func TestReadMessage_TypedAfterPasteJoins(t *testing.T) {
	keys := "fix this:\r"
	got, err := readMessage(fakeTTY(keys))
	if err != nil || got != "fix this:" {
		t.Fatalf("typed line: %q %v", got, err)
	}

	keys = "\x1b[200~a\rb\x1b[201~ please\r"
	got, err = readMessage(fakeTTY(keys))
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\nb please" {
		t.Fatalf("got %q", got)
	}
}

func TestReadMessage_CtrlCAtPromptIsEOF(t *testing.T) {
	if _, err := readMessage(fakeTTY("half typed\x03")); err != io.EOF {
		t.Fatalf("err=%v, want EOF", err)
	}
}
