package agent

import "testing"

func TestStripHarnessContext_TrailingClock(t *testing.T) {
	in := "rename Foo to Bar\n\n[current time] NOW: Saturday\nthis week: x"
	if got := stripHarnessContext(in); got != "rename Foo to Bar" {
		t.Fatalf("got %q", got)
	}
}

func TestStripHarnessContext_LeadingHarness(t *testing.T) {
	in := "[harness] Not user text — clock for this turn.\n[current time] NOW: x\n\nhello"
	if got := stripHarnessContext(in); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestStripHarnessContext_GluedFooter(t *testing.T) {
	in := "hello\n[current time] NOW: x\nalready today: y"
	if got := stripHarnessContext(in); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestStripHarnessContext_KeepsMention(t *testing.T) {
	in := "what does [current time] mean in the footer"
	if got := stripHarnessContext(in); got != in {
		t.Fatalf("got %q", got)
	}
}

func TestStripHarnessContext_MemoryBlock(t *testing.T) {
	in := "[memory]\n- (fact) x: y\n\nreal ask"
	if got := stripHarnessContext(in); got != "real ask" {
		t.Fatalf("got %q", got)
	}
}

func TestStripDanglingToolTags(t *testing.T) {
	for in, want := range map[string]string{
		"Updated greet.txt. Anything else?<tool_call>": "Updated greet.txt. Anything else?",
		"Done.\n<tool_call>\n</tool_call>\n":           "Done.",
		"Done. </function_call>":                       "Done.",
		"Use `<tool_call>` tags to print a call.":      "Use `<tool_call>` tags to print a call.",
		"```go\nfunc main() {}\n```":                   "```go\nfunc main() {}\n```",
		"<tool_call>":                                  "",
	} {
		if got := stripDanglingToolTags(in); got != want {
			t.Errorf("stripDanglingToolTags(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatHarnessClock(t *testing.T) {
	got := formatHarnessClock("[current time] NOW: x")
	if got != "[harness] Not user text — clock for this turn.\n[current time] NOW: x" {
		t.Fatalf("got %q", got)
	}
	if formatHarnessClock("  ") != "" {
		t.Fatal("empty clock")
	}
}

func TestHarnessNote_NamesOnlyPresentTags(t *testing.T) {
	cases := map[string]string{
		"[current time] y":           "clock",
		"[current time] y\nmore":     "clock",
		"no tag on this line at all": "context",
	}
	for clock, want := range cases {
		if got := harnessNote(clock); got != harnessNotePrefix+want+" for this turn." {
			t.Fatalf("%q → %q", clock, got)
		}
	}
}
