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

// A coding turn that ends on a promise ("let me first patch…") after a read
// is a deferral; a finished summary or a "let me know" closer is not.
func TestDefersPendingWork_CodingCues(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{`I'll rename "Hello" to "Greet" in both files. Let me first patch a.go, then a_test.go.`, true},
		{"Now I'll patch a.go.", true},
		{"Next, I'll run the tests.", true},
		{"Now I need to update the test file to pass the new parameter.", true},
		{"Give me one moment while I confirm the connection.", true},
		{"Renamed Hello to Greet in a.go and a_test.go.", false},
		{"Changed hi to hello; wc -l says 1. Let me know if you want more.", false},
	} {
		if got := defersPendingWork(tc.text); got != tc.want {
			t.Errorf("defersPendingWork(%q) = %v, want %v", tc.text, got, tc.want)
		}
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
