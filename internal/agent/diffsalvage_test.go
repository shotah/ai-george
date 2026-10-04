package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/george/internal/provider"
)

// The 2026-10-04 reply, shape for shape: a diff --git block with an index
// line, and a bare ---/+++ block for the same file above it. One entry,
// hunks in the order written, header rebuilt.
func TestPrintedDiffs_SameFileMerges(t *testing.T) {
	content := strings.Join([]string{
		"--- a/docs/todo.md",
		"+++ b/docs/todo.md",
		"@@ -59,4 +59,5 @@ This file tracks",
		" - **No YAML.** flat key=value",
		"+- [ ] **`george self-update` command.**",
		"diff --git a/docs/todo.md b/docs/todo.md",
		"index 9a8f0c3..b5e4a8d 100644",
		"--- a/docs/todo.md",
		"+++ b/docs/todo.md",
		"@@ -48,7 +48,6 @@",
		" - [ ] **Unit tests.**",
		"-- [ ] **`george self-update` command.**",
		"",
		" ### Not doing",
	}, "\n")
	got := printedDiffs(content)
	if len(got) != 1 || got[0].path != "docs/todo.md" {
		t.Fatalf("diffs = %+v", got)
	}
	want := "--- a/docs/todo.md\n+++ b/docs/todo.md\n" +
		"@@ -59,4 +59,5 @@ This file tracks\n - **No YAML.** flat key=value\n+- [ ] **`george self-update` command.**\n" +
		"@@ -48,7 +48,6 @@\n - [ ] **Unit tests.**\n-- [ ] **`george self-update` command.**\n \n ### Not doing\n"
	if got[0].diff() != want {
		t.Fatalf("diff =\n%s\nwant\n%s", got[0].diff(), want)
	}
}

func TestPrintedDiffs_Shapes(t *testing.T) {
	fenced := "Here is the change:\n\n```diff\n--- a/a.go\n+++ b/a.go\n@@ -1,3 +1,3 @@\n package a\n-func Hello() {}\n+func Greet() {}\n```\n\nDone."
	got := printedDiffs(fenced)
	if len(got) != 1 || got[0].path != "a.go" || strings.Contains(got[0].hunks, "Done") || !strings.HasSuffix(got[0].hunks, "+func Greet() {}\n") {
		t.Fatalf("fenced: %+v", got)
	}

	two := "--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-x\n+y\n--- a/b.go\n+++ b/b.go\n@@ -1 +1 @@\n-x\n+y\n"
	got = printedDiffs(two)
	if len(got) != 2 || got[0].path != "a.go" || got[1].path != "b.go" || strings.Contains(got[0].hunks, "b.go") {
		t.Fatalf("two files: %+v", got)
	}

	// A removed line that starts with "--" is not a file header.
	tricky := "--- a/a.md\n+++ b/a.md\n@@ -1,2 +1,2 @@\n--- old rule\n+-- new rule\n done\n"
	got = printedDiffs(tricky)
	if len(got) != 1 || !strings.Contains(got[0].hunks, "--- old rule\n") {
		t.Fatalf("tricky: %+v", got)
	}

	// Blank lines inside a hunk become blank context; the trailing gap
	// before prose is dropped.
	blank := "--- a/a.md\n+++ b/a.md\n@@ -1,3 +1,3 @@\n # Title\n\n-old\n+new\n\nThat is the edit."
	got = printedDiffs(blank)
	if len(got) != 1 || got[0].hunks != "@@ -1,3 +1,3 @@\n # Title\n \n-old\n+new\n" {
		t.Fatalf("blank: %q", got)
	}

	for name, content := range map[string]string{
		"hunks without a file":  "@@ -1 +1 @@\n-x\n+y\n",
		"header without a hunk": "--- a/a.go\n+++ b/a.go\nnothing here",
		"bullets":               "- a.go keeps Hello\n+ one more: the budget",
		"prose":                 "I changed a.go.",
	} {
		if got := printedDiffs(content); len(got) != 0 {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if got := printedDiffs("--- a/a.go\r\n+++ b/a.go\r\n@@ -1 +1 @@\r\n-x\r\n+y\r\n"); len(got) != 1 || strings.Contains(got[0].hunks, "\r") {
		t.Fatalf("crlf: %+v", got)
	}
}

func TestSalvagePrintedDiff(t *testing.T) {
	defs := []provider.ToolDef{{Name: "fs__file_get"}, {Name: "fs__file_patch"}}
	diff := "--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-x\n+y\n"
	fresh := func() *turnLanes { return lanesFrom(withTurnLanes(context.Background(), "change x to y in a.go")) }

	calls := salvagePrintedDiff(diff, defs, fresh())
	if len(calls) != 1 || calls[0].Name != "fs__file_patch" || calls[0].ID != "printed-diff-1" {
		t.Fatalf("calls = %+v", calls)
	}
	if !strings.Contains(calls[0].Arguments, `"path":"a.go"`) || !strings.Contains(calls[0].Arguments, `"diff":"--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-x\n+y\n"`) {
		t.Fatalf("args = %s", calls[0].Arguments)
	}

	// A write already landed: the diff is the model showing its work.
	l := fresh()
	l.record("fs__file_patch", view("", "a.go").args)
	if got := salvagePrintedDiff(diff, defs, l); got != nil {
		t.Errorf("after a write: %+v", got)
	}
	// A proposal stays a proposal.
	if got := salvagePrintedDiff("Proposed, not applied:\n"+diff, defs, fresh()); got != nil {
		t.Errorf("proposal: %+v", got)
	}
	// No patch tool published, or no lanes: nothing to apply with.
	if got := salvagePrintedDiff(diff, []provider.ToolDef{{Name: "fs__file_get"}}, fresh()); got != nil {
		t.Errorf("no patch tool: %+v", got)
	}
	if got := salvagePrintedDiff(diff, defs, nil); got != nil {
		t.Errorf("nil lanes: %+v", got)
	}
	// Diff markers with no placeable file: nothing salvaged (the
	// contradiction nudge takes that one).
	if got := salvagePrintedDiff("@@ -1 +1 @@\n-x\n+y\n", defs, fresh()); got != nil {
		t.Errorf("hunks only: %+v", got)
	}
}
