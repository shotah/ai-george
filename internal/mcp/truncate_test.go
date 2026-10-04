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
	var first, last, total, next int
	if _, err := fmt.Sscanf(head, "range: %d-%d of %d; next offset %d", &first, &last, &total, &next); err != nil || first != 11 || total != 300 || last >= 209 || next != last+1 {
		t.Fatalf("header = %q", head)
	}
	wantTail := fmt.Sprintf("line %03d %s\n…[cut at 6000 chars: lines 11-%d of 300 shown; read again with offset %d for the rest]",
		last, strings.Repeat("é", 30), last, last+1)
	if !strings.HasSuffix(got, wantTail) {
		t.Fatalf("tail = %q", got[len(got)-200:])
	}
}

// fs-mcp 0.0.6 numbers lines and ends the header with "; next offset N" or
// "; end". A backstop cut keeps that dialect, numbered lines and all.
func TestTruncate_NewHeaderDialect(t *testing.T) {
	var b strings.Builder
	b.WriteString("range: 1-230 of 230; end\n")
	for i := 1; i <= 230; i++ {
		fmt.Fprintf(&b, "%d: line %s\n", i, strings.Repeat("x", 40))
	}
	got := mcp.Truncate(b.String(), 2000)
	head, _, _ := strings.Cut(got, "\n")
	var first, last, total, next int
	if _, err := fmt.Sscanf(head, "range: %d-%d of %d; next offset %d", &first, &last, &total, &next); err != nil || first != 1 || total != 230 || next != last+1 {
		t.Fatalf("header = %q", head)
	}
	if !strings.Contains(got, fmt.Sprintf("\n%d: line ", last)) || strings.Contains(got, fmt.Sprintf("\n%d: line ", last+1)) {
		t.Fatalf("body does not end at line %d: %q", last, got[len(got)-200:])
	}
	// Under the cap the server's page is left exactly as it came.
	page := "range: 1-2 of 2; end\n1: a\n2: b\n"
	if mcp.Truncate(page, 6000) != page {
		t.Fatal("page under the cap was changed")
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
