package selfnote_test

import (
	"strings"
	"testing"

	"github.com/shotah/george/internal/selfnote"
)

func TestRestoreQuotedLines_KeepsDroppedJoke(t *testing.T) {
	prior := "# SELF.md — Who You Are Becoming\n\n> header\n\n- gag: \"that gull had a mortgage\"\n- dry humor"
	next := "# SELF.md — Who You Are Becoming\n- dry humor"
	got := selfnote.RestoreQuotedLines(prior, next)
	if !strings.Contains(got, "gull") || !strings.Contains(got, "dry humor") {
		t.Fatalf("got %q", got)
	}
	again := selfnote.RestoreQuotedLines(prior, got)
	if strings.Count(again, "gull") != strings.Count(got, "gull") {
		t.Fatalf("duplicated: %q", again)
	}
	if selfnote.RestoreQuotedLines("", next) != next {
		t.Fatal("empty prior mutated next")
	}
}
