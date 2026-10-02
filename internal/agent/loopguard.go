package agent

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/shotah/george/internal/provider"
)

// loopNote replaces budgetExhaustedNote when the landing call is forced by a
// repeated cycle of rounds rather than by the cap.
const loopNote = "[system] Loop stopped: your last tool rounds repeated the rounds before them exactly, so another round would find nothing new and no more tool calls are possible. " +
	"Write your final reply to the user now: summarize what you accomplished, what you found, and what is still unfinished."

// roundSig is one round's calls and what they returned, order-free: the same
// batch in a new order (or with different JSON spacing) is the same round. A
// re-run whose result changed (tests now pass) is progress, not a repeat.
// results must be in calls order, as runToolRound returns them.
func roundSig(calls []provider.ToolCall, results []toolRoundResult) string {
	parts := make([]string, len(calls))
	for i, c := range calls {
		args := strings.TrimSpace(c.Arguments)
		var buf bytes.Buffer
		if json.Compact(&buf, []byte(args)) == nil {
			args = buf.String()
		}
		parts[i] = c.Name + "\x00" + args
		if i < len(results) {
			parts[i] += "\x00" + results[i].out
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, "\x01")
}

// repeatsCycle reports whether the last k rounds repeat the k before them,
// for any k. A test, fix, re-test loop is not a cycle while each fix differs.
func repeatsCycle(sigs []string) bool {
	n := len(sigs)
	for k := 1; 2*k <= n; k++ {
		if slices.Equal(sigs[n-k:], sigs[n-2*k:n-k]) {
			return true
		}
	}
	return false
}
