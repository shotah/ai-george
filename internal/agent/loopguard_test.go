package agent

import (
	"testing"

	"github.com/shotah/george/internal/provider"
)

func getCall(args string) provider.ToolCall {
	return provider.ToolCall{Name: "fs__file_get", Arguments: args}
}

func outs(s ...string) []toolRoundResult {
	r := make([]toolRoundResult, len(s))
	for i, o := range s {
		r[i] = toolRoundResult{out: o}
	}
	return r
}

func TestRoundSig_OrderAndWhitespaceFree(t *testing.T) {
	a := roundSig([]provider.ToolCall{getCall(`{"path":"a.go"}`), getCall(`{"path":"b.go"}`)}, outs("A", "B"))
	b := roundSig([]provider.ToolCall{getCall(`{ "path": "b.go" }`), getCall(`{"path":"a.go"}`)}, outs("B", "A"))
	if a != b {
		t.Fatalf("same batch in another order or spacing should match:\n%q\n%q", a, b)
	}
	if a == roundSig([]provider.ToolCall{getCall(`{"path":"a.go"}`)}, outs("A")) {
		t.Fatal("a smaller batch is a different round")
	}
}

func TestRoundSig_ChangedResultIsNewRound(t *testing.T) {
	run := []provider.ToolCall{{Name: "shell__command_run", Arguments: `{"command":"go test ./..."}`}}
	if roundSig(run, outs("exit 1\nFAIL")) == roundSig(run, outs("exit 0\nok")) {
		t.Fatal("a re-run with a different result is progress, not a repeat")
	}
}

func TestRepeatsCycle(t *testing.T) {
	get, patchA, patchB := "get", "patchA", "patchB"
	run, status := "run", "status"
	for _, tc := range []struct {
		name string
		sigs []string
		want bool
	}{
		{"first round", []string{get}, false},
		{"same round twice", []string{get, get}, true},
		{"test, fix, test, other fix, test", []string{run, patchA, run, patchB, run}, false},
		{"same fix twice", []string{run, patchA, run, patchA}, true},
		{"whole chain repeated", []string{get, patchA, run, status, get, patchA, run, status}, true},
		{"chain then one more round", []string{get, patchA, run, status, get}, false},
	} {
		if got := repeatsCycle(tc.sigs); got != tc.want {
			t.Errorf("%s: repeatsCycle = %v, want %v", tc.name, got, tc.want)
		}
	}
}
