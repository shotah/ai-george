package provider_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shotah/george/internal/provider"
)

func TestParseToolCallText(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		wantName string
		wantArgs string // substring
	}{{
		name:     "bare object with parameters key",
		content:  "{\n  \"name\": \"garmin__get_daily_activity\",\n  \"parameters\": {\n    \"date\": \"2026-07-28\"\n  }\n}",
		wantName: "garmin__get_daily_activity",
		wantArgs: "2026-07-28",
	}, {
		name:     "arguments key",
		content:  `{"name":"garmin__hrv_get","arguments":{"date":"2026-07-27"}}`,
		wantName: "garmin__hrv_get",
		wantArgs: "2026-07-27",
	}, {
		name:     "stringified arguments",
		content:  `{"name":"garmin__hrv_get","arguments":"{\"date\":\"2026-07-27\"}"}`,
		wantName: "garmin__hrv_get",
		wantArgs: "2026-07-27",
	}, {
		name:     "markdown fence",
		content:  "```json\n{\"name\":\"garmin__hrv_get\",\"parameters\":{}}\n```",
		wantName: "garmin__hrv_get",
		wantArgs: "{}",
	}, {
		name:     "tool_call tags",
		content:  "<tool_call>\n{\"name\": \"math__expression_evaluate\", \"arguments\": {\"expr\": \"2+2\"}}\n</tool_call>",
		wantName: "math__expression_evaluate",
		wantArgs: "2+2",
	}, {
		name:     "embedded in prose",
		content:  `Pulling that up now: {"name":"garmin__sleep_get","parameters":{"date":"2026-07-28"}} — one sec.`,
		wantName: "garmin__sleep_get",
		wantArgs: "2026-07-28",
	}, {
		name:     "no arguments at all still runs",
		content:  `{"name":"garmin__hrv_get"}`,
		wantName: "garmin__hrv_get",
		wantArgs: "{}",
	}, {
		// Braces inside a string value must not end the object early.
		name:     "braces inside string values",
		content:  `{"name":"math__expression_evaluate","arguments":{"expr":"{2+2}"}}`,
		wantName: "math__expression_evaluate",
		wantArgs: "{2+2}",
	}, {
		// Qwen prints MCP calls as {server__tool(k="v")} instead of tool_calls.
		name: "fn-style braced kwargs",
		content: `{google__calendar_list_events(time_min="2026-07-29T00:00:00-07:00", ` +
			`time_max="2026-07-30T00:00:00-07:00", calendar_id="primary", ` +
			`user_google_email="christopherblodgett@gmail.com")}`,
		wantName: "google__calendar_list_events",
		wantArgs: "christopherblodgett@gmail.com",
	}, {
		name:     "fn-style without braces",
		content:  `Calling garmin__sleep_get(date="2026-07-29") now.`,
		wantName: "garmin__sleep_get",
		wantArgs: "2026-07-29",
	}, {
		name: "fn-style duplicated in reply (take first)",
		content: `{google__calendar_list_events(calendar_id="primary", time_min="2026-07-29T00:00:00-07:00")}` +
			"\n\n" +
			`{google__calendar_list_events(calendar_id="primary", time_min="2026-07-29T00:00:00-07:00")}`,
		wantName: "google__calendar_list_events",
		wantArgs: "primary",
	}, {
		// Brat mode: name in prose, args in a markdown json fence.
		name: "args-only json fence with tool name nearby",
		content: "google__calendar_list_events\n```json\n" +
			`{"calendar_id":"primary","time_min":"2026-07-29T00:00:00-07:00","time_max":"2026-07-30T00:00:00-07:00"}` +
			"\n```",
		wantName: "google__calendar_list_events",
		wantArgs: "time_min",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call, ok := provider.ParseToolCallText(tc.content)
			if !ok {
				t.Fatalf("ParseToolCallText(%q) = not a call", tc.content)
			}
			if call.Name != tc.wantName {
				t.Errorf("name = %q, want %q", call.Name, tc.wantName)
			}
			if !strings.Contains(call.Arguments, tc.wantArgs) {
				t.Errorf("arguments = %q, want to contain %q", call.Arguments, tc.wantArgs)
			}
			if call.ID == "" {
				t.Error("id empty; the tool result could not be matched to the call")
			}
		})
	}
}

func TestParseToolCallText_Rejects(t *testing.T) {
	for _, content := range []string{
		"",
		"You rode 21mi yesterday.",
		`{"protein":180,"carbs":220}`,           // JSON, but no name
		`{"name":"","arguments":{}}`,            // empty name
		`{"nickname":"SAM","arguments":{}}`,     // name-ish key that is not name
		"{\"name\":\"x\", \"arguments\": {oops", // unbalanced
		// Real answer that mentions a tool and includes unrelated JSON — do not hijack.
		`I already used google__calendar_list_events earlier. Macros: {"protein":180,"carbs":220}`,
	} {
		if call, ok := provider.ParseToolCallText(content); ok {
			t.Errorf("ParseToolCallText(%q) = %+v, want rejected", content, call)
		}
	}
}

func TestParseToolCallTextHinted_ArgsOnlyFromPriorTurn(t *testing.T) {
	content := "```json\n" +
		`{"calendar_id":"primary","time_max":"2026-07-30T00:00:00-07:00","time_min":"2026-07-29T00:00:00-07:00","user_google_email":"christopherblodgett@gmail.com"}` +
		"\n```"
	call, ok := provider.ParseToolCallTextHinted(content, []string{"google__calendar_list_events"})
	if !ok {
		t.Fatal("expected salvage from hint + args-only fence")
	}
	if call.Name != "google__calendar_list_events" {
		t.Fatalf("name = %q", call.Name)
	}
	if !strings.Contains(call.Arguments, "christopherblodgett@gmail.com") {
		t.Fatalf("arguments = %q", call.Arguments)
	}
}

func TestParseToolCallText_QwenXML(t *testing.T) {
	content := `<tool_call>
<function=fs__file_get>
<parameter=path>
readme.md
</parameter>
</function>
</tool_call>`
	call, ok := provider.ParseToolCallText(content)
	if !ok {
		t.Fatalf("ParseToolCallText(%q) = not a call", content)
	}
	if call.Name != "fs__file_get" {
		t.Errorf("name = %q, want fs__file_get", call.Name)
	}
	if call.Arguments != `{"path":"readme.md"}` {
		t.Errorf("arguments = %q, want %q", call.Arguments, `{"path":"readme.md"}`)
	}
	if !strings.HasPrefix(call.ID, "call_text_") {
		t.Errorf("id = %q, want call_text_ prefix", call.ID)
	}
}

func TestParseToolCallTexts_QwenXMLBatch(t *testing.T) {
	content := `<tool_call>
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
	calls := provider.ParseToolCallTexts(content)
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	want := []struct {
		name string
		args string
	}{
		{name: "fs__file_get", args: `{"path":"readme.md"}`},
		{name: "fs__dir_list", args: `{"path":"src"}`},
	}
	for i, w := range want {
		if calls[i].Name != w.name {
			t.Errorf("calls[%d].Name = %q, want %q", i, calls[i].Name, w.name)
		}
		if calls[i].Arguments != w.args {
			t.Errorf("calls[%d].Arguments = %q, want %q", i, calls[i].Arguments, w.args)
		}
		if !strings.HasPrefix(calls[i].ID, "call_text_") {
			t.Errorf("calls[%d].ID = %q, want call_text_ prefix", i, calls[i].ID)
		}
	}
	if calls[0].ID == calls[1].ID {
		t.Errorf("both calls got id %q", calls[0].ID)
	}
	first, ok := provider.ParseToolCallText(content)
	if !ok || first.Name != "fs__file_get" || first.Arguments != want[0].args {
		t.Fatalf("ParseToolCallText = %+v, ok=%v, want the first call", first, ok)
	}
}

func TestParseToolCallText_QwenXMLObjectParam(t *testing.T) {
	content := `<tool_call>
<function=fs__search>
<parameter=query>
{"path":"readme.md","limit":2}
</parameter>
<parameter=tags>
["a","b"]
</parameter>
</function>
</tool_call>`
	call, ok := provider.ParseToolCallText(content)
	if !ok {
		t.Fatal("expected a call")
	}
	if strings.Contains(call.Arguments, `"query":"`) || strings.Contains(call.Arguments, `"tags":"`) {
		t.Fatalf("structured param encoded as a string: %s", call.Arguments)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		t.Fatalf("arguments %q: %v", call.Arguments, err)
	}
	q, ok := args["query"].(map[string]any)
	if !ok {
		t.Fatalf("query type = %T, want object", args["query"])
	}
	if q["path"] != "readme.md" || q["limit"] != float64(2) {
		t.Fatalf("query = %#v", q)
	}
	tags, ok := args["tags"].([]any)
	if !ok {
		t.Fatalf("tags type = %T, want array", args["tags"])
	}
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Fatalf("tags = %#v", tags)
	}
}

func TestParseToolCallText_QwenXMLDriftedClose(t *testing.T) {
	content := `<tool_call>
<function=fs__file_get>
<parameter=path>
readme.md
</parameter>
</function>
</function>
</function_invocation>
</tool_call>`
	call, ok := provider.ParseToolCallText(content)
	if !ok {
		t.Fatalf("ParseToolCallText(%q) = not a call", content)
	}
	if call.Name != "fs__file_get" {
		t.Errorf("name = %q, want fs__file_get", call.Name)
	}
	if call.Arguments != `{"path":"readme.md"}` {
		t.Errorf("arguments = %q, want %q", call.Arguments, `{"path":"readme.md"}`)
	}
}

func TestParseToolCallText_QwenXMLTrimsOneNewline(t *testing.T) {
	// Two trailing newlines and a leading space: only one newline on each
	// side is removed, so the space and the extra newline stay in the string.
	content := "<tool_call>\n<function=fs__file_get>\n<parameter=path>\n readme.md\n\n</parameter>\n</function>\n</tool_call>"
	call, ok := provider.ParseToolCallText(content)
	if !ok {
		t.Fatal("expected a call")
	}
	if call.Arguments != "{\"path\":\" readme.md\\n\"}" {
		t.Fatalf("arguments = %q", call.Arguments)
	}
}

func TestParseToolCallText_QwenXMLNoParams(t *testing.T) {
	content := `<tool_call>
<function=fs__file_get>
</function>
</tool_call>`
	call, ok := provider.ParseToolCallText(content)
	if !ok {
		t.Fatal("expected a call")
	}
	if call.Name != "fs__file_get" || call.Arguments != "{}" {
		t.Fatalf("got name %q arguments %q", call.Name, call.Arguments)
	}
}

func TestParseToolCallText_QwenXMLProse(t *testing.T) {
	content := "I would use fs__file_get to read readme.md if you want the contents."
	if call, ok := provider.ParseToolCallText(content); ok {
		t.Errorf("ParseToolCallText(%q) = %+v, want rejected", content, call)
	}
	if calls := provider.ParseToolCallTexts(content); len(calls) != 0 {
		t.Errorf("ParseToolCallTexts(%q) = %+v, want rejected", content, calls)
	}
}

// Ids must not collide: the tool result is matched back by id.
func TestParseToolCallText_UniqueIDs(t *testing.T) {
	a, _ := provider.ParseToolCallText(`{"name":"garmin__hrv_get"}`)
	b, _ := provider.ParseToolCallText(`{"name":"garmin__hrv_get"}`)
	if a.ID == b.ID {
		t.Fatalf("both calls got id %q", a.ID)
	}
}
