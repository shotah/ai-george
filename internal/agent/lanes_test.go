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
	} {
		if l.contradiction(reply) == "" {
			t.Errorf("nothing written, %q should nudge", reply)
		}
	}
	for _, reply := range []string{
		"`Add` returns `a - b`; it hasn't been fixed.",
		"calc.go has `Add`, `Mul`, and `Div`.",
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

func TestLanes_NoTurnNoRefusal(t *testing.T) {
	var l *turnLanes
	if got := l.check([]callView{view("fs__file_patch", "a.go")}); got[0] != nil {
		t.Fatalf("nil lanes refused: %v", got)
	}
	l.record("fs__file_get", nil)
}
