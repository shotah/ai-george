package agent

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/shotah/george/internal/provider"
)

// An aged round loses its payload, never its arguments: a `{}` stub there
// taught the model to send `fs__file_get {}`.
func TestCollapse_OldArgsStayWhole(t *testing.T) {
	fat := `{"origin":"37.4,-122.1","destination":"37.8,-122.4"}` + strings.Repeat("x", 200)

	orig := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c1", Name: "maps__route_eta", Arguments: fat},
		}},
		{Role: provider.RoleTool, ToolCallID: "c1", Content: strings.Repeat("R", 800)},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c2", Name: "maps__place_search", Arguments: `{"query":"coffee"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c2", Content: "ok"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c3", Name: "math__expression_evaluate", Arguments: `{"expression":"1+1"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c3", Content: "2"},
	}
	// Keep a copy of the original args so we can prove the input is not mutated.
	origArgs := orig[0].ToolCalls[0].Arguments

	out := collapseOldToolResults(orig)
	if orig[0].ToolCalls[0].Arguments != origArgs {
		t.Fatal("collapse mutated the caller's ToolCalls")
	}
	if !strings.HasPrefix(out[1].Content, "[tool maps__route_eta:") {
		t.Fatalf("old result not collapsed: %q", out[1].Content)
	}
	if out[0].ToolCalls[0].Arguments != fat {
		t.Fatalf("old args = %q, want them whole", out[0].ToolCalls[0].Arguments)
	}
	if out[2].ToolCalls[0].Arguments != `{"query":"coffee"}` {
		t.Fatalf("recent args collapsed: %q", out[2].ToolCalls[0].Arguments)
	}
	if out[4].ToolCalls[0].Arguments != `{"expression":"1+1"}` {
		t.Fatalf("newest args collapsed: %q", out[4].ToolCalls[0].Arguments)
	}
}

// denverBatch is the round-1 batch the Denver eval fixture produces: two
// same-name searches for two possible Fridays plus the calendar, in one
// parallel batch.
func denverBatch() []provider.Message {
	return []provider.Message{
		{Role: provider.RoleUser, Content: "I need to be in Denver next Friday for a client thing."},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c1", Name: "flights__offers_search", Arguments: `{"origin":"SEA","destination":"DEN","outbound_date":"2026-09-25"}`},
			{ID: "c2", Name: "flights__offers_search", Arguments: `{"origin":"SEA","destination":"DEN","outbound_date":"2026-09-18"}`},
			{ID: "c3", Name: "google__calendar_list_events", Arguments: `{"time_min":"2026-09-18T00:00:00-07:00"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c1", Content: `{"offers":[{"airline":"Alaska","price":189}],"date":"09-25"}`},
		{Role: provider.RoleTool, ToolCallID: "c2", Content: `{"offers":[{"airline":"United","price":218}],"date":"09-18"}`},
		{Role: provider.RoleTool, ToolCallID: "c3", Content: `{"events":[]}`},
	}
}

// The batch the model just made is read back whole next round — every
// payload, both same-name searches, arguments intact. This was the eval's
// duplicated paid search and its invented fare: the first result of a
// three-call batch was a "[truncated]" stub before the model ever saw it.
func TestCollapse_UnreadBatchStaysWhole(t *testing.T) {
	msgs := denverBatch()
	out := collapseOldToolResults(msgs)
	for i := 2; i <= 4; i++ {
		if strings.HasPrefix(out[i].Content, "[tool ") {
			t.Fatalf("result %s collapsed before it was read: %q", out[i].ToolCallID, out[i].Content)
		}
	}
	for j, tc := range out[1].ToolCalls {
		if tc.Arguments != msgs[1].ToolCalls[j].Arguments {
			t.Fatalf("call %d args changed in the unread batch: %q", j, tc.Arguments)
		}
	}
}

// The window counts rounds, not payloads: a three-call round followed by a
// one-call round is two rounds and stays whole for the reply; a third round
// ages the first one out — all three of its payloads together, args kept.
func TestCollapse_WindowIsRoundsNotPayloads(t *testing.T) {
	msgs := append(denverBatch(),
		provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c4", Name: "memory_store", Arguments: `{"subject":"event/denver"}`},
		}},
		provider.Message{Role: provider.RoleTool, ToolCallID: "c4", Content: "stored"},
	)
	out := collapseOldToolResults(msgs)
	for _, i := range []int{2, 3, 4, 6} {
		if strings.HasPrefix(out[i].Content, "[tool ") {
			t.Fatalf("two rounds should stay whole; %s collapsed", out[i].ToolCallID)
		}
	}

	msgs = append(msgs,
		provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c5", Name: "cron_schedule", Arguments: `{"in":"2h"}`},
		}},
		provider.Message{Role: provider.RoleTool, ToolCallID: "c5", Content: "scheduled"},
	)
	out = collapseOldToolResults(msgs)
	for _, i := range []int{2, 3, 4} {
		if !strings.HasPrefix(out[i].Content, "[tool ") {
			t.Fatalf("round 1 should age out at round 3; %s kept: %q", out[i].ToolCallID, out[i].Content)
		}
	}
	for j, tc := range out[1].ToolCalls {
		if tc.Arguments != msgs[1].ToolCalls[j].Arguments {
			t.Fatalf("aged round's call %d args changed: %q", j, tc.Arguments)
		}
	}
	if out[6].Content != "stored" || out[8].Content != "scheduled" {
		t.Fatalf("last two rounds must stay whole: %q %q", out[6].Content, out[8].Content)
	}
	if msgs[2].Content == out[2].Content {
		t.Fatal("expected a collapsed copy, got the original")
	}
}

// The live readme turn: the head read in round 1 and the tail in round 3
// were stubs by the patch in round 4, so `old` came from memory and missed.
// The newest good read of each page now outlives the window until a write
// to that file lands; a re-read of the same page replaces the older one.
func TestCollapse_NewestReadOfAFileStays(t *testing.T) {
	round := func(id, name, args, result string) []provider.Message {
		return []provider.Message{
			{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: id, Name: name, Arguments: args}}},
			{Role: provider.RoleTool, ToolCallID: id, Content: result},
		}
	}
	head := "range: 1-130 of 199\n# george…"
	tail := "range: 131-199 of 199\n## Read next…"
	msgs := slices.Concat(
		round("r1", "fs__file_get", `{"path":"readme.md"}`, head),
		round("r2", "fs__file_get", `{"path":"readme.md","offset":200}`, "tool error: offset 200 is past end (199 lines)"),
		round("r3", "fs__file_get", `{"path":"./readme.md","offset":131}`, tail),
		round("r4", "fs__file_patch", `{"path":"readme.md","old":"x","new":"y"}`, "tool error: 0 matches for old in readme.md; nothing written"),
		round("r5", "fs__file_search", `{"query":"Get started"}`, "readme.md:12"),
	)
	out := collapseOldToolResults(msgs)
	if out[1].Content != head || out[5].Content != tail {
		t.Fatalf("both pages should stay whole for the next patch: %q / %q", out[1].Content, out[5].Content)
	}
	if !strings.HasPrefix(out[3].Content, "[tool ") {
		t.Fatalf("a failed read is not pinned: %q", out[3].Content)
	}

	msgs = slices.Concat(msgs,
		round("r6", "fs__file_get", `{"path":"readme.md"}`, head+" again"),
		round("r7", "git__status_get", `{}`, "clean"),
		round("r8", "git__diff_get", `{}`, ""),
	)
	out = collapseOldToolResults(msgs)
	if !strings.HasPrefix(out[1].Content, "[tool ") || out[11].Content != head+" again" {
		t.Fatalf("a re-read of the page replaces the older one: %q / %q", out[1].Content, out[11].Content)
	}

	msgs = slices.Concat(msgs,
		round("r9", "fs__file_patch", `{"path":"readme.md","old":"a","new":"b"}`, "patched readme.md"),
		round("r10", "git__status_get", `{}`, "M readme.md"),
		round("r11", "git__diff_get", `{}`, "+b"),
	)
	out = collapseOldToolResults(msgs)
	for _, i := range []int{5, 11} {
		if !strings.HasPrefix(out[i].Content, "[tool ") {
			t.Fatalf("a read from before the write landed is stale and ages out: %q", out[i].Content)
		}
	}
}

func TestPinnedReads_Bounded(t *testing.T) {
	var msgs []provider.Message
	for i := range maxPinnedReads + 3 {
		id := fmt.Sprintf("c%d", i)
		msgs = append(msgs,
			provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
				{ID: id, Name: "fs__file_get", Arguments: fmt.Sprintf(`{"path":"f%d.go"}`, i)},
			}},
			provider.Message{Role: provider.RoleTool, ToolCallID: id, Content: "range: 1-1 of 1\nx\n"},
		)
	}
	got := pinnedReads(msgs)
	if len(got) != maxPinnedReads || !got[fmt.Sprintf("c%d", maxPinnedReads+2)] || got["c0"] {
		t.Fatalf("pinned = %v, want the newest %d", got, maxPinnedReads)
	}
}

// Same tool name across two kept rounds is two different answers (two
// dates), not a stack to squeeze; both stay.
func TestCollapse_SameNameAcrossKeptRoundsStays(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c1", Name: "flights__offers_search", Arguments: `{"outbound_date":"2026-09-25"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c1", Content: `{"date":"09-25"}`},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "c2", Name: "flights__offers_search", Arguments: `{"outbound_date":"2026-09-18"}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "c2", Content: `{"date":"09-18"}`},
	}
	out := collapseOldToolResults(msgs)
	if out[1].Content != `{"date":"09-25"}` || out[3].Content != `{"date":"09-18"}` {
		t.Fatalf("both dates should survive: %q %q", out[1].Content, out[3].Content)
	}
}
