package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/shotah/george/internal/provider"
)

// keepRecentToolRounds is how many trailing tool rounds — one model-emitted
// batch each — keep their payloads in full. Older rounds collapse to a
// one-line marker (readme §5 bounding rules). The unit is the round, not the
// payload: a model that asks three things at once — two flight dates and the
// calendar, as the persona's "prefer parallel tool calls" invites — has to
// see all three answers next round. Counting payloads (and squeezing
// same-name results to the newest) hid the first one behind a "[truncated]"
// stub before it was ever read, and the model did the only sensible things:
// searched again (a paid call, paid twice) or filled the hole in with a fare
// no tool returned. Search-heavy MCPs (flights/rentals/cars) re-send results
// each iteration, so 2 rounds (not 4) still bounds in-turn prefill.
const keepRecentToolRounds = 2

// collapseOldToolResults shortens tool payloads from rounds older than the
// recent window. Everything in the last keepRecentToolRounds rounds stays
// whole, however many calls a round made and whatever they were named, and
// so do the newest reads of each file (pinnedReads). Tool-call arguments always stay whole: the model copies the shape of its
// own past calls, and a `{}` stub there came back as `fs__file_get {}` from
// round 4 on.
func collapseOldToolResults(messages []provider.Message) []provider.Message {
	roundOf := make(map[string]int)
	rounds := 0
	for _, m := range messages {
		if m.Role != provider.RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			roundOf[tc.ID] = rounds
		}
		rounds++
	}
	if rounds <= keepRecentToolRounds {
		return messages
	}
	oldestKept := rounds - keepRecentToolRounds
	names := toolCallNames(messages)
	pinned := pinnedReads(messages)

	out := make([]provider.Message, len(messages))
	copy(out, messages)
	seen := 0 // rounds started before this position — the round of an unpaired result
	for i, m := range messages {
		if m.Role == provider.RoleAssistant && len(m.ToolCalls) > 0 {
			seen++
			continue
		}
		if m.Role != provider.RoleTool {
			continue
		}
		round, ok := roundOf[m.ToolCallID]
		if !ok {
			round = seen - 1
		}
		if round >= oldestKept || pinned[m.ToolCallID] {
			continue
		}
		name := names[m.ToolCallID]
		if name == "" {
			name = "result"
		}
		out[i].Content = fmt.Sprintf("[tool %s: %d chars, truncated]", name, len(m.Content))
	}
	return out
}

// maxPinnedReads bounds how many file reads outlive the round window.
const maxPinnedReads = 4

// pinnedReads is the tool call ids of the newest good read of each file (and
// page of it) that no later good write to that file made stale, newest
// first, up to maxPinnedReads. A patch quotes what it read; with the read
// collapsed two rounds later, the model wrote `old` from memory, missed, and
// read the file again.
func pinnedReads(messages []provider.Message) map[string]bool {
	failed := make(map[string]bool)
	for _, m := range messages {
		if m.Role == provider.RoleTool && strings.HasPrefix(m.Content, "tool error") {
			failed[m.ToolCallID] = true
		}
	}
	type read struct {
		id, path string
		at       int
	}
	newest := make(map[string]read) // path and page -> newest read
	wrote := make(map[string]int)   // path -> position of its last write
	at := 0
	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			at++
			if failed[tc.ID] {
				continue
			}
			args := json.RawMessage(tc.Arguments)
			p := argPath(args)
			if p == "" {
				continue
			}
			n := strings.ToLower(tc.Name)
			switch {
			case strings.HasSuffix(n, "file_get"):
				newest[p+"\x00"+readPage(args)] = read{id: tc.ID, path: p, at: at}
			case strings.HasSuffix(n, "file_create"), laneOf(tc.Name) == lanePatch:
				wrote[p] = at
			}
		}
	}
	live := make([]read, 0, len(newest))
	for _, r := range newest {
		if wrote[r.path] < r.at {
			live = append(live, r)
		}
	}
	slices.SortFunc(live, func(a, b read) int { return b.at - a.at })
	out := make(map[string]bool)
	for _, r := range live[:min(len(live), maxPinnedReads)] {
		out[r.id] = true
	}
	return out
}

// readPage is a read's arguments other than its path, so each page of a
// file is its own read.
func readPage(args json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(args, &m) != nil {
		return ""
	}
	delete(m, "path")
	b, _ := json.Marshal(m)
	return string(b)
}

func toolCallNames(messages []provider.Message) map[string]string {
	out := make(map[string]string)
	for _, m := range messages {
		if m.Role != provider.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			out[tc.ID] = tc.Name
		}
	}
	return out
}
