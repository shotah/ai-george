package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shotah/george/internal/provider"
)

func view(name, path string) callView {
	args, _ := json.Marshal(map[string]string{"path": path})
	return callView{name: name, args: args}
}

func TestLanes_PatchWaitsOnRead(t *testing.T) {
	l := lanesFrom(withTurnLanes(context.Background(), "fix the bug"))

	// A read and a patch in one batch: the patch is refused either way.
	got := l.check([]callView{view("fs__file_get", "a.go"), view("fs__file_patch", "a.go")})
	if got[0] != nil || !errors.Is(got[1], errLaneRefused) {
		t.Fatalf("same batch = %v", got)
	}
	l.record("fs__file_get", view("", "./a.go").args)
	for _, p := range []string{"a.go", "/a.go", "/work/repo/a.go"} {
		if err := l.check([]callView{view("fs__file_patch", p)})[0]; err != nil {
			t.Errorf("patch %s after read: %v", p, err)
		}
	}
	if err := l.check([]callView{view("fs.file_patch", "b.go")})[0]; !errors.Is(err, errLaneRefused) {
		t.Errorf("mangled patch of unread b.go = %v", err)
	}
	if err := l.check([]callView{view("fs__file_patch", "xa.go")})[0]; err == nil {
		t.Error("xa.go is not a.go")
	}
	l.record("fs__file_create", view("", "c.go").args)
	if err := l.check([]callView{view("fs__file_patch", "c.go")})[0]; err != nil {
		t.Errorf("patch after create: %v", err)
	}
}

func TestLanes_GitWaitsOnAsk(t *testing.T) {
	for _, tc := range []struct {
		inbound string
		allowed bool
	}{
		{"fix the typo in readme", false},
		{"fix it and commit", true},
		{"stage the readme change", true},
		{"Commit that please", true},
		{"git add the new file", true},
	} {
		l := lanesFrom(withTurnLanes(context.Background(), tc.inbound))
		got := l.check([]callView{{name: "git__commit_create"}, {name: "git__stage_update"}, {name: "git__status_get"}})
		if (got[0] == nil) != tc.allowed || (got[1] == nil) != tc.allowed || got[2] != nil {
			t.Errorf("%q: %v", tc.inbound, got)
		}
	}
}

func TestLanes_Contradiction(t *testing.T) {
	fresh := func() *turnLanes { return lanesFrom(withTurnLanes(context.Background(), "fix Add")) }

	l := fresh()
	for _, reply := range []string{
		"Fixed `Add` in calc.go to return `a + b`. `go test ./...` passes.",
		"Done; tests pass.",
		"Build succeeded (exit code 0).",
	} {
		if l.contradiction(reply) == "" {
			t.Errorf("no command ran, %q should nudge", reply)
		}
	}
	for _, reply := range []string{
		"`Add` in calc.go now returns `a + b`. No tests failed (only Mul had tests).",
		"Fixed `Add` in calc.go. I didn't run the tests.",
		"Here’s the `## Still on the radar` section (I inserted it right before Get started):",
		"I've added a Troubleshooting section to readme.md.",
		"I updated the install steps in `readme.md`.",
	} {
		if l.contradiction(reply) == "" {
			t.Errorf("nothing written, %q should nudge", reply)
		}
	}
	for _, reply := range []string{
		"`Add` returns `a - b`; it hasn't been fixed.",
		"calc.go has `Add`, `Mul`, and `Div`.",
		"Want me to insert this above Get started? Once it's added, the readme covers setup.",
		"Here's a draft; I haven't inserted it yet.",
		"I've added a little note to my internal compass as the example copy.",
	} {
		if got := l.contradiction(reply); got != "" {
			t.Errorf("%q should hold, got %q", reply, got)
		}
	}
	created := fresh()
	created.record("fs__file_create", view("", "sub.go").args)
	if got := created.contradiction("Added `Sub` in sub.go; it now returns `a - b`. Tests were not run."); got != "" {
		t.Errorf("a created file is a write: %q", got)
	}

	l.record("fs__file_patch", view("", "calc.go").args)
	for _, reply := range []string{"No code changed.", "`Add` already returns `a + b`, so no change was needed."} {
		if got := l.contradiction(reply); got == "" {
			t.Errorf("%q after a patch should nudge", reply)
		}
	}
	if got := l.contradiction("Fixed `Add`; tests pass."); !strings.Contains(got, "shell__command_run") {
		t.Errorf("after the patch the nudge asks for the check: %q", got)
	}
	if got := l.contradiction("Fixed `Add`. I haven't run the tests, so I can't say they pass."); got != "" {
		t.Errorf("an honest not-run: %q", got)
	}
	l.record("shell__command_run", json.RawMessage(`{"command":"go test ./..."}`))
	if got := l.contradiction("Fixed `Add`; `go test ./...` passes."); got != "" {
		t.Errorf("a check that ran: %q", got)
	}
	if got := fresh().contradiction("`Add` is already `a + b`. No changes were needed."); got == "" {
		t.Error("no change with nothing read should nudge")
	}
	looked := fresh()
	looked.record("fs__file_get", view("", "calc.go").args)
	if got := looked.contradiction("No code changed: the file already adds."); got != "" {
		t.Errorf("read, no patch, no change is true: %q", got)
	}
}

func write(l *turnLanes, name string, args map[string]string) {
	raw, _ := json.Marshal(args)
	l.record(name, raw)
}

func TestLanes_Unfinished(t *testing.T) {
	fresh := func() *turnLanes { return lanesFrom(withTurnLanes(context.Background(), "add Mul")) }

	l := fresh()
	write(l, "fs__file_patch", map[string]string{"path": "calc.go", "old": "}\n", "new": "}\n\nfunc Mul(a, b int) int {\n\treturn a * b\n}\n"})
	for range 2 {
		if got := l.unfinished(true); !strings.Contains(got, "Mul") || !strings.Contains(got, "test") {
			t.Fatalf("new func, no test: %q", got)
		}
	}
	if got := l.unfinished(true); !strings.Contains(got, "shell__command_run") {
		t.Fatalf("then the check: %q", got)
	}
	if got := l.unfinished(true); got != "" {
		t.Fatalf("each fires once: %q", got)
	}

	l = fresh()
	write(l, "fs__file_patch", map[string]string{"path": "calc.go", "diff": "@@\n+func (c *Calc) Mul(a, b int) int { return a * b }\n"})
	write(l, "fs__file_patch", map[string]string{"path": "calc_test.go", "new": "func TestMul(t *testing.T) {}\n"})
	write(l, "shell__command_run", map[string]string{"command": "go test ./..."})
	if got := l.unfinished(true); got != "" {
		t.Fatalf("test written and run: %q", got)
	}

	l = fresh()
	write(l, "fs__file_patch", map[string]string{"path": "calc.go", "old": "\treturn a - b", "new": "\treturn a + b"})
	write(l, "shell__command_run", map[string]string{"command": "go test ./..."})
	write(l, "fs__file_patch", map[string]string{"path": "calc.go", "old": "a + b", "new": "b + a"})
	if got := l.unfinished(false); got != "" {
		t.Fatalf("no command tool, no check nudge: %q", got)
	}
	if got := l.unfinished(true); !strings.Contains(got, "shell__command_run") {
		t.Fatalf("a write after the run needs another run: %q", got)
	}

	l = fresh()
	write(l, "fs__file_patch", map[string]string{"path": "a.go", "old": "func Hello(", "new": "func Hello("})
	write(l, "fs__file_patch", map[string]string{"path": "greet.txt", "old": "hi", "new": "hello"})
	write(l, "fs__file_create", map[string]string{"path": "notes.md", "body": "func Fake() {}\n"})
	write(l, "shell__command_run", map[string]string{"command": "go test ./..."})
	if got := l.unfinished(true); got != "" {
		t.Fatalf("no new func, text files, run after: %q", got)
	}
	if !canRunCommand([]provider.ToolDef{{Name: "fs__file_get"}, {Name: "shell__command_run"}}) || canRunCommand([]provider.ToolDef{{Name: "fs__file_get"}}) {
		t.Fatal("canRunCommand")
	}
}

func TestLanes_PagedReadContinuation(t *testing.T) {
	l := lanesFrom(withTurnLanes(context.Background(), "edit the readme"))
	get := func(path string) json.RawMessage { return view("", path).args }

	if _, ok := l.continuation(get("readme.md")); ok {
		t.Fatal("no read yet")
	}
	// fs-mcp 0.0.6 page.
	l.noteResult("fs__file_get", get("readme.md"), "range: 1-39 of 230; next offset 40\n1: a\n")
	if m, ok := l.continuation(get("./readme.md")); !ok || m.last != 39 || m.next != 40 {
		t.Fatalf("after page 1: %+v %v", m, ok)
	}
	// The host's own cut of an older server's read.
	l.noteResult("fs__file_get", get("docs/todo.md"), "range: 1-59 of 184\nx\n…[cut at 6000 chars: lines 1-59 of 184 shown; read again with offset 60 for the rest]")
	if m, ok := l.continuation(get("docs/todo.md")); !ok || m.next != 60 {
		t.Fatalf("host cut: %+v %v", m, ok)
	}
	// The last page ends the continuation.
	l.noteResult("fs__file_get", get("readme.md"), "range: 40-230 of 230; end\n40: b\n")
	if _, ok := l.continuation(get("readme.md")); ok {
		t.Fatal("end page still continues")
	}
	// A header-less result (a short file) is not a page.
	l.noteResult("fs__file_get", get("a.go"), "package a\n")
	if _, ok := l.continuation(get("a.go")); ok {
		t.Fatal("plain read continues")
	}
	var none *turnLanes
	none.noteResult("fs__file_get", get("a.go"), "range: 1-2 of 9; next offset 3\n")
	if _, ok := none.continuation(get("a.go")); ok {
		t.Fatal("nil lanes")
	}
}

func TestLanes_Unlanded(t *testing.T) {
	fresh := func() *turnLanes { return lanesFrom(withTurnLanes(context.Background(), "add the section")) }
	patch := func(path string) json.RawMessage { return view("", path).args }
	miss := errors.New("0 matches for old in readme.md; nothing written. Nearest: line 57")

	// A read-only turn that declines the edit is never nudged here.
	l := fresh()
	l.record("fs__file_get", patch("readme.md"))
	if got := l.unlanded("It covers install and usage; nothing to add."); got != "" {
		t.Fatalf("no attempt: %q", got)
	}

	// Every write failed and the reply moves on: one nudge, naming the miss.
	l = fresh()
	l.record("fs__file_get", patch("readme.md"))
	l.noteFailure("fs__file_patch", patch("readme.md"), miss)
	l.noteFailure("fs__file_patch", patch("readme.md"), miss)
	got := l.unlanded("The file ends at line 184. I'll append the new section after the last line.")
	if !strings.Contains(got, "No write landed") || !strings.Contains(got, "fs__file_patch readme.md: 0 matches") || !strings.Contains(got, "after_line") {
		t.Fatalf("nudge = %q", got)
	}
	if again := l.unlanded("### Adding the section"); again != "" {
		t.Fatalf("once a turn: %q", again)
	}

	// A reply that owns the miss ships.
	l = fresh()
	l.noteFailure("fs__file_patch", patch("readme.md"), miss)
	for _, reply := range []string{
		"I couldn't apply the edit: the text in readme.md did not match.",
		"The patch failed twice (0 matches); the section was not written.",
	} {
		if got := l.unlanded(reply); got != "" {
			t.Errorf("%q: %q", reply, got)
		}
	}

	// A write that landed after the misses clears it.
	l = fresh()
	l.noteFailure("fs__file_patch", patch("readme.md"), miss)
	l.record("fs__file_patch", patch("readme.md"))
	if got := l.unlanded("Added the section at the end of readme.md."); got != "" {
		t.Fatalf("landed: %q", got)
	}

	// Only writes count as failures; a failed read or command does not.
	l = fresh()
	l.noteFailure("fs__file_get", patch("nope.md"), errors.New("no such file"))
	l.noteFailure("shell__command_run", nil, errors.New("exit 1"))
	if got := l.unlanded("nope.md does not exist."); got != "" {
		t.Fatalf("read failure: %q", got)
	}
	var none *turnLanes
	none.noteFailure("fs__file_patch", patch("a"), miss)
	if none.unlanded("x") != "" {
		t.Fatal("nil lanes")
	}
}

// go test, then the patch, then "tests pass": the run is from before the
// change, so the claim is sent back; a run after the patch clears it.
func TestLanes_StaleCheckClaim(t *testing.T) {
	l := lanesFrom(withTurnLanes(context.Background(), "fix calc.go so the tests pass, then run go test"))
	write(l, "shell__command_run", map[string]string{"command": "go test ./..."})
	write(l, "fs__file_patch", map[string]string{"path": "calc.go", "old": "a - b", "new": "a + b"})
	got := l.contradiction("Tests pass now. Changed `a - b` to `a + b` in calc.go.")
	if !strings.Contains(got, "no command ran after the last code change") {
		t.Fatalf("stale run: %q", got)
	}
	if got := l.contradiction("Changed `a - b` to `a + b` in calc.go; not re-run yet."); got != "" {
		t.Fatalf("owned: %q", got)
	}
	write(l, "shell__command_run", map[string]string{"command": "go test ./..."})
	if got := l.contradiction("Tests pass now."); got != "" {
		t.Fatalf("re-run: %q", got)
	}
	// A text-file write does not make an earlier run stale.
	l = lanesFrom(withTurnLanes(context.Background(), "fix the readme typo and run make lint"))
	write(l, "shell__command_run", map[string]string{"command": "make lint"})
	write(l, "fs__file_patch", map[string]string{"path": "readme.md", "old": "hi", "new": "hello"})
	if got := l.contradiction("Fixed the typo; lint passed."); got != "" {
		t.Fatalf("text write: %q", got)
	}
}

// The 2026-10-04 todo.md turns: zero tool calls and a reply that is a
// unified diff of a file the model never read. No tool produced it, so it
// is a claim. A diff after a command (git diff) or a write, or one offered
// as a proposal, holds.
func TestLanes_InventedDiff(t *testing.T) {
	fresh := func() *turnLanes {
		return lanesFrom(withTurnLanes(context.Background(), "move the self-update item up to the work list, don't ask, just do"))
	}
	diff := "```diff\n--- a/docs/todo.md\n+++ b/docs/todo.md\n@@ -48,7 +48,6 @@\n- - [ ] **`george self-update` command.**\n```"
	hunkOnly := "    @@ -59,3 +58,4 @@ This file tracks\n    +- [ ] **`george self-update` command.**"
	for _, reply := range []string{diff, hunkOnly, "diff --git a/docs/todo.md b/docs/todo.md\nindex 9a8f0c3..b5e4a8d 100644"} {
		got := fresh().contradiction(reply)
		if !strings.Contains(got, "no tool produced it") || !strings.Contains(got, "fs__file_patch") {
			t.Errorf("invented diff %q: %q", reply[:20], got)
		}
	}
	if got := fresh().contradiction("Proposed, not applied:\n" + diff); got != "" {
		t.Errorf("owned proposal: %q", got)
	}
	l := fresh()
	l.record("shell__command_run", json.RawMessage(`{"command":"git diff"}`))
	if got := l.contradiction(diff); got != "" {
		t.Errorf("after git diff: %q", got)
	}
	l = fresh()
	l.record("fs__file_patch", view("", "docs/todo.md").args)
	if got := l.contradiction("Moved it. " + diff); got != "" {
		t.Errorf("after a write: %q", got)
	}
	// Prose with a "-" bullet and a "+" is not a diff.
	if got := fresh().contradiction("- todo.md keeps the item\n+ one more thing: the budget"); got != "" {
		t.Errorf("bullets: %q", got)
	}
}

// An edit was asked for, the turn only read, and the reply is a diagnosis
// with no question: sent back once. Questions, reviews, and "no change
// needed" ship.
func TestLanes_UnlandedDiagnosis(t *testing.T) {
	ask := func(inbound string) *turnLanes { return lanesFrom(withTurnLanes(context.Background(), inbound)) }
	get := func(l *turnLanes, path string) { l.record("fs__file_get", view("", path).args) }

	l := ask("go test fails in this repo. Fix calc.go so the tests pass, then run go test.")
	get(l, "calc.go")
	get(l, "calc_test.go")
	got := l.unlanded("Two bugs in `calc.go`:\n\n1. `Add` returns `a - b`\n2. `Double` returns `n * 3`")
	if !strings.Contains(got, "only read") || !strings.Contains(got, "fs__file_patch") {
		t.Fatalf("diagnosis: %q", got)
	}
	if again := l.unlanded("Still two bugs."); again != "" {
		t.Fatalf("once: %q", again)
	}

	// In order: a question; no edit verb; the reply asks; nothing to do;
	// already so; declined; owned.
	for _, tc := range []struct{ inbound, reply string }{
		{"What does calc.go do?", "It adds and doubles."},
		{"Review calc.go and tell me what is wrong.", "Add subtracts; Double triples."},
		{"Fix calc.go.", "Which of the two functions should I fix first?"},
		{"Fix the typo in calc.go.", "There is no typo in calc.go; no change was needed."},
		{"Add a Mul function to calc.go.", "calc.go already has Mul, at line 12."},
		{"Fix Add in calc.go.", "Add is correct as written; it should not be changed."},
		{"Fix calc.go so the tests pass.", "I couldn't find a test file, so nothing was changed."},
	} {
		l := ask(tc.inbound)
		get(l, "calc.go")
		if got := l.unlanded(tc.reply); got != "" {
			t.Errorf("%q / %q: %q", tc.inbound, tc.reply, got)
		}
	}
	// Nothing read: the theater and claim checks own that turn.
	l = ask("Fix calc.go.")
	if got := l.unlanded("Fixed."); got != "" {
		t.Fatalf("no read: %q", got)
	}
}

// A question before any read, when the inbound named a file and asked for
// a change: sent back once, naming the file. After a read, or with no file
// named, or with no question, it ships.
func TestLanes_BlindAsk(t *testing.T) {
	ask := func(inbound string) *turnLanes { return lanesFrom(withTurnLanes(context.Background(), inbound)) }

	l := ask("can you take a look at the readme.md file and add a self-update command?")
	got := l.blindAsk("Which repo holds the current version command? Can you link it?")
	if !strings.Contains(got, "readme.md") || !strings.Contains(got, "fs__file_get") {
		t.Fatalf("blind ask: %q", got)
	}
	if again := l.blindAsk("Still: which repo?"); again != "" {
		t.Fatalf("once: %q", again)
	}

	// In order: read first; no file named; no edit asked; not a question;
	// a command ran.
	l = ask("add a Mul function to calc.go")
	l.record("fs__file_get", view("", "calc.go").args)
	if got := l.blindAsk("calc.go has Mul already; did you mean Div?"); got != "" {
		t.Errorf("after a read: %q", got)
	}
	if got := ask("fix the bug").blindAsk("Which file is the bug in?"); got != "" {
		t.Errorf("no file named: %q", got)
	}
	if got := ask("what does readme.md say about e.g. the release page?").blindAsk("Do you mean the install section?"); got != "" {
		t.Errorf("no edit asked: %q", got)
	}
	if got := ask("add a Mul function to calc.go").blindAsk("Adding Mul."); got != "" {
		t.Errorf("not a question: %q", got)
	}
	l = ask("fix calc.go so go test passes")
	l.record("shell__command_run", json.RawMessage(`{"command":"go test ./..."}`))
	if got := l.blindAsk("go test passes already; is there a different failure?"); got != "" {
		t.Errorf("after a run: %q", got)
	}
	var none *turnLanes
	if none.blindAsk("x?") != "" {
		t.Fatal("nil lanes")
	}
}

// The caveat says what the turn did, from state: nothing, a read only, a
// code write with no run after it. Empty once a write landed and was
// checked.
func TestLanes_Caveat(t *testing.T) {
	l := lanesFrom(withTurnLanes(context.Background(), "update readme.md"))
	if got := l.caveat(); !strings.Contains(got, "No file was written and no command ran") {
		t.Fatalf("nothing: %q", got)
	}
	l.record("fs__file_get", view("", "readme.md").args)
	if got := withCaveat(l, "Here are the diffs:"); !strings.HasPrefix(got, "No file was written and no command ran this turn; what follows was proposed, not done.\n\nHere are") {
		t.Fatalf("read only: %q", got)
	}
	l.record("shell__command_run", json.RawMessage(`{"command":"go test ./..."}`))
	if got := l.caveat(); got != "No file was written this turn; what follows was proposed, not done." {
		t.Fatalf("ran, no write: %q", got)
	}
	write(l, "fs__file_patch", map[string]string{"path": "calc.go", "old": "a - b", "new": "a + b"})
	if got := l.caveat(); !strings.Contains(got, "No command ran after the last code change") {
		t.Fatalf("stale: %q", got)
	}
	l.record("shell__command_run", json.RawMessage(`{"command":"go test ./..."}`))
	if got := withCaveat(l, "Tests pass."); got != "Tests pass." {
		t.Fatalf("clean: %q", got)
	}
	var none *turnLanes
	if withCaveat(none, "x") != "x" {
		t.Fatal("nil lanes")
	}
}

func TestLanes_NoTurnNoRefusal(t *testing.T) {
	var l *turnLanes
	if got := l.check([]callView{view("fs__file_patch", "a.go")}); got[0] != nil {
		t.Fatalf("nil lanes refused: %v", got)
	}
	l.record("fs__file_get", nil)
}
