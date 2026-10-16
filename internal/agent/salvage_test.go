package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/george/internal/agent"
	"github.com/shotah/george/internal/channel"
	"github.com/shotah/george/internal/mcp"
	"github.com/shotah/george/internal/provider"
)

// Observed on Qwen after a tool-call nudge: it printed the call as JSON with a
// "parameters" key and no tool_calls, and george shipped that JSON to Telegram as
// SAM's reply. The call must be executed instead — and since the printed name was
// not real either, the failure has to chain into the constrained retry.
func TestAgent_PrintedToolCallIsExecutedNotSpoken(t *testing.T) {
	printed := "{\n  \"name\": \"garmin__get_daily_activity\",\n  \"parameters\": {\n    \"date\": \"2026-07-28\"\n  }\n}"

	var forced [][]string
	n := 0
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		forced = append(forced, req.ForceToolNames)
		n++
		switch n {
		case 1:
			return &provider.Result{Content: printed, FinishReason: "stop"}, nil
		case 2:
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c2", Name: "garmin__activities_list", Arguments: `{"date":"2026-07-28"}`},
			}}, nil
		default:
			return &provider.Result{Content: "You rode 21mi yesterday."}, nil
		}
	}}
	tools := &unknownThenOK{
		defs: []provider.ToolDef{{Name: "garmin__activities_list"}},
		unknown: &mcp.UnknownToolError{
			Name:       "garmin__get_daily_activity",
			Hint:       "no such tool; valid garmin tools are: garmin__activities_list",
			Candidates: []string{"garmin__activities_list"},
		},
	}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        tools,
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "what did I ride?"})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(reply, "{") || strings.Contains(reply, "garmin__") {
		t.Fatalf("reply = %q, want prose — raw wire format must never reach the user", reply)
	}
	if reply != "You rode 21mi yesterday." {
		t.Fatalf("reply = %q", reply)
	}
	// The printed name was executed as written, which is what surfaces the real
	// catalog and arms the constrained retry.
	if len(tools.calls) != 2 || tools.calls[0] != "garmin__get_daily_activity" {
		t.Fatalf("tool calls = %v, want the printed name attempted first", tools.calls)
	}
	if len(forced) != 3 || len(forced[1]) != 1 || forced[1][0] != "garmin__activities_list" {
		t.Fatalf("forced names per call = %v, want the retry constrained", forced)
	}
}

// A reply that merely contains JSON is not a tool call. Hijacking it would eat
// legitimate answers.
func TestAgent_JSONReplyWithoutToolNameStaysAReply(t *testing.T) {
	answer := `Here are your macros: {"protein": 180, "carbs": 220, "name": "tuesday"}`
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: answer}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        &fakeTools{defs: []provider.ToolDef{{Name: "garmin__hrv_get"}}},
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "macros?"})
	if err != nil {
		t.Fatal(err)
	}
	if got != answer {
		t.Fatalf("reply = %q, want it delivered untouched", got)
	}
}

// Two Qwen3.6 <tool_call> blocks in one assistant message are one batch.
// Both run, in order, and the XML is not also the reply.
func TestAgent_PrintedXMLToolCallsAllRun(t *testing.T) {
	printed := `<tool_call>
<function=fs__file_get>
<parameter=path>
readme.md
</parameter>
</function>
</tool_call>
<tool_call>
<function=fs__dir_list>
<parameter=path>
src
</parameter>
</function>
</tool_call>`

	var reqs []provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		reqs = append(reqs, req)
		if len(reqs) == 1 {
			return &provider.Result{Content: printed, FinishReason: "stop"}, nil
		}
		return &provider.Result{Content: "readme.md and src."}, nil
	}}
	tools := &fakeTools{defs: []provider.ToolDef{
		{Name: "fs__file_get"},
		{Name: "fs__dir_list"},
	}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        tools,
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "what's in readme and src?"})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "readme.md and src." {
		t.Fatalf("reply = %q", reply)
	}
	if strings.Contains(reply, "<tool_call>") || strings.Contains(reply, "<function=") {
		t.Fatalf("reply still contains the printed call: %q", reply)
	}
	if len(reqs) != 2 {
		t.Fatalf("model calls = %d, want the printed batch then the reply", len(reqs))
	}

	var asst provider.Message
	found := false
	for _, m := range reqs[1].Messages {
		if m.Role == provider.RoleAssistant && len(m.ToolCalls) > 0 {
			asst = m
			found = true
		}
	}
	if !found {
		t.Fatal("printed XML was not turned into tool calls")
	}
	if asst.Content != "" {
		t.Fatalf("assistant content = %q, want it cleared", asst.Content)
	}
	if len(asst.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v, want two", asst.ToolCalls)
	}
	if asst.ToolCalls[0].Name != "fs__file_get" || asst.ToolCalls[0].Arguments != `{"path":"readme.md"}` {
		t.Fatalf("first call = %+v", asst.ToolCalls[0])
	}
	if asst.ToolCalls[1].Name != "fs__dir_list" || asst.ToolCalls[1].Arguments != `{"path":"src"}` {
		t.Fatalf("second call = %+v", asst.ToolCalls[1])
	}
	var resultIDs []string
	for _, m := range reqs[1].Messages {
		if m.Role == provider.RoleTool {
			resultIDs = append(resultIDs, m.ToolCallID)
		}
	}
	if len(resultIDs) != 2 || resultIDs[0] != asst.ToolCalls[0].ID || resultIDs[1] != asst.ToolCalls[1].ID {
		t.Fatalf("tool results = %v, want %s then %s", resultIDs, asst.ToolCalls[0].ID, asst.ToolCalls[1].ID)
	}
	seen := map[string]int{}
	for _, name := range tools.calls {
		seen[name]++
	}
	if seen["fs__file_get"] != 1 || seen["fs__dir_list"] != 1 || len(tools.calls) != 2 {
		t.Fatalf("tools reached = %v", tools.calls)
	}
}

// A sentence that names a tool and has no <function= block is a reply.
// The prose-narration nudge may take another sample; that is not a salvage.
func TestAgent_ProseToolNameIsNotSalvaged(t *testing.T) {
	answer := "I would use fs__file_get to read readme.md if you want the contents."
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: answer}, nil
	}}
	tools := &fakeTools{defs: []provider.ToolDef{{Name: "fs__file_get"}}}
	a, err := agent.New(agent.Options{
		Completer:    fc,
		Sessions:     newMemHistory(),
		Tools:        tools,
		Model:        "m",
		MaxToolIters: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "how would you read it?"})
	if err != nil {
		t.Fatal(err)
	}
	if got != answer {
		t.Fatalf("reply = %q, want it delivered untouched", got)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("tools reached = %v", tools.calls)
	}
}
