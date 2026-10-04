package mcp_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/shotah/george/internal/mcp"
)

func TestTruncate(t *testing.T) {
	if got := mcp.Truncate("hi", 10); got != "hi" {
		t.Fatal(got)
	}
	long := strings.Repeat("x", 100)
	got := mcp.Truncate(long, 20)
	if utf8.RuneCountInString(got) > 20 {
		t.Fatalf("len=%d", utf8.RuneCountInString(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("%q", got)
	}
}

func TestTruncate_FileReadSaysWhereToGoOn(t *testing.T) {
	var b strings.Builder
	b.WriteString("range: 11-209 of 300\n")
	for i := 11; i <= 209; i++ {
		fmt.Fprintf(&b, "line %03d %s\n", i, strings.Repeat("é", 30))
	}
	got := mcp.Truncate(b.String(), 6000)
	if n := utf8.RuneCountInString(got); n > 6000 {
		t.Fatalf("len=%d", n)
	}
	head, _, _ := strings.Cut(got, "\n")
	var first, last, total int
	if _, err := fmt.Sscanf(head, "range: %d-%d of %d", &first, &last, &total); err != nil || first != 11 || total != 300 || last >= 209 {
		t.Fatalf("header = %q", head)
	}
	wantTail := fmt.Sprintf("line %03d %s\n…[cut at 6000 chars: lines 11-%d of 300 shown; read again with offset %d for the rest]",
		last, strings.Repeat("é", 30), last, last+1)
	if !strings.HasSuffix(got, wantTail) {
		t.Fatalf("tail = %q", got[len(got)-200:])
	}
}

func TestTruncate_PlainTextKeepsOldMarker(t *testing.T) {
	got := mcp.Truncate("range: nope\n"+strings.Repeat("x\n", 50), 20)
	if !strings.HasSuffix(got, "\n…[truncated]") {
		t.Fatalf("%q", got)
	}
}

func TestPrefixedName(t *testing.T) {
	got, err := mcp.PrefixedName("google-workspace", "gmail.search")
	if err != nil {
		t.Fatal(err)
	}
	if got != "google-workspace__gmail_search" {
		t.Fatalf("%q", got)
	}
}
