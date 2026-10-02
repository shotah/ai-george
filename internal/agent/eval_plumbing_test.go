package agent_test

// Plumbing for the behavioral eval, run in `go test ./...` with a scripted
// Completer. Pins that the fixtures are well-formed and that the harness
// records what it should: tool calls through the recorder, mcp_enable
// filtering, sequenced tool results, scripted rounds, memory rows.
// The live model is eval_integration_test.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/george/internal/mcp"
	"github.com/shotah/george/internal/provider"
)

// evalManifest lists the servers a tools_from fixture may name; the
// integration run fetches and boots them for their real catalogs.
var evalManifest = filepath.Join(evalFixtureDir, "mcp.toml")

// evalLiveServers reads the eval manifest's server names (no network).
func evalLiveServers(t *testing.T) map[string]mcp.ServerSpec {
	t.Helper()
	m, err := mcp.LoadManifest(evalManifest)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]mcp.ServerSpec{}
	for _, s := range m.Servers {
		if s.DownloadURL == "" {
			t.Errorf("%s: server %q needs download_url so the eval can fetch its latest release", evalManifest, s.Name)
		}
		out[s.Name] = s
	}
	return out
}

// scriptCompleter answers round i with res[i]; the last result repeats.
type scriptCompleter struct {
	mu   sync.Mutex
	reqs []provider.Request
	res  []*provider.Result
}

func (s *scriptCompleter) Complete(_ context.Context, req provider.Request) (*provider.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	i := len(s.reqs) - 1
	if i >= len(s.res) {
		i = len(s.res) - 1
	}
	return s.res[i], nil
}

func toolCall(id, name string, args map[string]any) provider.ToolCall {
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return provider.ToolCall{ID: id, Name: name, Arguments: string(raw)}
}

func hasToolDef(defs []provider.ToolDef, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}

func compileEvalRegexes(t *testing.T, name string, e evalExpect) {
	t.Helper()
	for _, re := range []string{e.ReplyRegex, e.ReplyNot} {
		if re == "" {
			continue
		}
		if _, err := regexp.Compile(re); err != nil {
			t.Errorf("%s: bad regex %q: %v", name, re, err)
		}
	}
	calls := append([]evalCallExpect(nil), e.ToolsCalled...)
	for _, batch := range e.SameRound {
		if len(batch) < 2 {
			t.Errorf("%s: same_round entry needs at least two calls", name)
		}
		calls = append(calls, batch...)
	}
	for _, c := range calls {
		if c.ArgsRegex == "" {
			continue
		}
		if _, err := regexp.Compile(c.ArgsRegex); err != nil {
			t.Errorf("%s: bad args_regex %q: %v", name, c.ArgsRegex, err)
		}
	}
	for _, alt := range e.AnyOf {
		compileEvalRegexes(t, name, alt)
	}
}

func TestEvalFixtures_WellFormed(t *testing.T) {
	fixtures := loadEvalFixtures(t, evalFixtureDir)
	liveServers := evalLiveServers(t)
	seen := map[string]bool{}
	for _, fx := range fixtures {
		if seen[fx.Name] {
			t.Errorf("duplicate fixture name %q", fx.Name)
		}
		seen[fx.Name] = true
		if fx.Why == "" {
			t.Errorf("%s: why is empty", fx.Name)
		}
		if fx.Inbound == "" {
			t.Errorf("%s: needs inbound", fx.Name)
		}
		for _, tl := range fx.Tools {
			if !strings.Contains(tl.Name, "__") {
				t.Errorf("%s: canned tool %q needs an MCP prefix (server__name)", fx.Name, tl.Name)
			}
			if tl.Result != "" && len(tl.Results) > 0 {
				t.Errorf("%s: %s sets both result and results", fx.Name, tl.Name)
			}
			for _, ba := range tl.ByArgs {
				if _, err := regexp.Compile(ba.ArgsRegex); ba.ArgsRegex == "" || err != nil {
					t.Errorf("%s: %s by_args regex %q: %v", fx.Name, tl.Name, ba.ArgsRegex, err)
				}
			}
		}
		for i, round := range fx.Script {
			if (len(round.ToolCalls) == 0) == (round.Content == "") {
				t.Errorf("%s: script[%d] needs tool_calls or content, not both or neither", fx.Name, i)
			}
		}
		if fx.Now != "" {
			if _, err := time.Parse(time.RFC3339, fx.Now); err != nil {
				t.Errorf("%s: now %q: %v", fx.Name, fx.Now, err)
			}
		}
		for _, server := range fx.ToolsFrom {
			if _, ok := liveServers[server]; !ok {
				t.Errorf("%s: tools_from %q is not a server in %s", fx.Name, server, evalManifest)
			}
			for _, tl := range fx.Tools {
				if strings.HasPrefix(tl.Name, server+"__") && (tl.Description != "" || tl.Params != nil) {
					t.Errorf("%s: %s comes from the live catalog; drop its description/params", fx.Name, tl.Name)
				}
			}
		}
		compileEvalRegexes(t, fx.Name, fx.Expect)
	}
}

// The live-catalog merge: real defs win, canned results attach by name, a
// name the live server does not publish is drift and fails before a model
// call, other servers pass through hand-written.
func TestMergeLiveTools(t *testing.T) {
	live := []provider.ToolDef{
		{Name: "fs__file_get", Description: "read one file", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}},
		{Name: "fs__file_patch", Description: "patch one file"},
		{Name: "git__status_get", Description: "not asked for"},
	}
	fixture := []evalTool{
		{Name: "fs__file_get", Result: "hi\n", ByArgs: []evalArgsResult{{ArgsRegex: `a\.go`, Result: "package a"}}},
		{Name: "shell__command_run", Description: "hand-written", Results: []string{"exit 1\n", "exit 0\n"}},
	}
	got, err := mergeLiveTools(live, []string{"fs"}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(got))
	for _, tl := range got {
		names = append(names, tl.Name)
	}
	if want := []string{"fs__file_get", "fs__file_patch", "shell__command_run"}; !slices.Equal(names, want) {
		t.Fatalf("names=%v want %v", names, want)
	}
	for _, tl := range got {
		switch tl.Name {
		case "fs__file_get":
			if tl.Description != "read one file" || tl.Params == nil || tl.Result != "hi\n" || len(tl.ByArgs) != 1 {
				t.Fatalf("live def + canned result not merged: %+v", tl)
			}
		case "fs__file_patch":
			if tl.Result != "{}" {
				t.Fatalf("uncanned live tool should default to {}: %+v", tl)
			}
		case "shell__command_run":
			if tl.Description != "hand-written" || len(tl.Results) != 2 {
				t.Fatalf("other server should pass through: %+v", tl)
			}
		}
	}

	_, err = mergeLiveTools(live, []string{"fs"}, []evalTool{{Name: "fs__file_read", Result: "x"}})
	if err == nil || !strings.Contains(err.Error(), `"fs__file_read" is not in the live fs catalog`) || !strings.Contains(err.Error(), "fs__file_get") {
		t.Fatalf("renamed tool should fail with the live names: %v", err)
	}
	if _, err := mergeLiveTools(live, []string{"github"}, nil); err == nil || !strings.Contains(err.Error(), "no live tools") {
		t.Fatalf("unconnected server should fail: %v", err)
	}
}

// max_calls counts calls matching name (+ args).
func TestCheckEval_MaxCalls(t *testing.T) {
	two := 2
	one := 1
	out := evalOutcome{Calls: []evalCall{
		{Name: "fs__file_patch", Args: `{"path":"a.go"}`},
		{Name: "fs__file_patch", Args: `{"path":"b.go"}`},
		{Name: "fs__file_get", Args: `{}`},
	}}
	if fails := checkEval(context.Background(), out, evalExpect{ToolsCalled: []evalCallExpect{{Name: "fs__file_patch", MaxCalls: &two}}}); len(fails) != 0 {
		t.Fatalf("two calls within max 2: %v", fails)
	}
	fails := checkEval(context.Background(), out, evalExpect{ToolsCalled: []evalCallExpect{{Name: "fs__file_patch", MaxCalls: &one}}})
	if len(fails) != 1 || fails[0] != "tool fs__file_patch called 2 times, max 1" {
		t.Fatalf("fails=%v", fails)
	}
	// ArgsRegex narrows the count.
	if fails := checkEval(context.Background(), out, evalExpect{ToolsCalled: []evalCallExpect{{Name: "fs__file_patch", ArgsRegex: `a\.go`, MaxCalls: &one}}}); len(fails) != 0 {
		t.Fatalf("one a.go call within max 1: %v", fails)
	}
}

// A tools_from fixture outside the integration tag is a hard error, never a
// silently hand-written schema.
func TestEvalHarness_ToolsFromNeedsLiveCatalog(t *testing.T) {
	if evalLiveTools != nil {
		t.Skip("live catalog wired")
	}
	if _, err := mergeLiveTools(nil, []string{"fs"}, nil); err == nil {
		t.Fatal("empty live catalog must fail")
	}
}

// editFixture is edit_then_check with hand-written defs, so it runs without
// the live catalog.
func editFixture(t *testing.T) evalFixture {
	t.Helper()
	fx := loadEvalFixture(t, filepath.Join(evalFixtureDir, "27_edit_then_check.json"))
	fx.ToolsFrom = nil
	return fx
}

func editScript() []*provider.Result {
	return []*provider.Result{
		{ToolCalls: []provider.ToolCall{toolCall("c1", "fs__file_get", map[string]any{"path": "greet.txt"})}},
		{ToolCalls: []provider.ToolCall{toolCall("c2", "fs__file_patch", map[string]any{"path": "greet.txt", "old": "hi", "new": "hello"})}},
		{ToolCalls: []provider.ToolCall{toolCall("c3", "shell__command_run", map[string]any{"command": "wc -l greet.txt"})}},
		{ToolCalls: []provider.ToolCall{toolCall("c4", "git__status_get", map[string]any{})}},
		{Content: "Changed hi to hello in greet.txt. wc -l says 1 line; tree is clean."},
	}
}

// The coding fixture through the harness: the recorder saw every call with
// its round, the batch shape and budget notes come out right, and round 1
// published the forced prefixes and the builtins.
func TestEvalHarness_EditThenCheck(t *testing.T) {
	ctx := context.Background()
	fx := editFixture(t)
	sc := &scriptCompleter{res: editScript()}

	out := runEvalFixture(ctx, t, sc, fx)

	if fails := checkEval(ctx, out, fx.Expect); len(fails) > 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
	if got := describeBatches(out); got != "[fs__file_get] → [fs__file_patch] → [shell__command_run] → [git__status_get] → reply" {
		t.Fatalf("batches %q", got)
	}
	one := 1
	if fails := checkEval(ctx, out, evalExpect{RoundBudget: &one}); len(fails) != 0 {
		t.Fatalf("round_budget must never fail a run: %v", fails)
	}
	if got := overBudget(out, evalExpect{RoundBudget: &one}); got != "over budget: 5 rounds, budget 1" {
		t.Fatalf("over-budget note %q", got)
	}
	if got := overBudget(out, fx.Expect); got != "" {
		t.Fatalf("inside budget should be quiet, got %q", got)
	}
	req := sc.reqs[0]
	for _, name := range []string{"memory_store", "self_note", "mcp_enable", "fs__file_get", "shell__command_run", "git__status_get"} {
		if !hasToolDef(req.Tools, name) {
			t.Errorf("round 1 did not publish %s", name)
		}
	}
}

func TestEvalHarness_BareAckFails(t *testing.T) {
	ctx := context.Background()
	fx := editFixture(t)
	sc := &scriptCompleter{res: []*provider.Result{{Content: "Done."}}}

	out := runEvalFixture(ctx, t, sc, fx)

	fails := checkEval(ctx, out, fx.Expect)
	if len(fails) != 6 {
		t.Fatalf("want four missing tools, an order, and a reply failure, got %v", fails)
	}
	// Reversed order is caught on its own.
	swapped := out
	swapped.Calls = []evalCall{{Name: "fs__file_patch", Args: `hello`}, {Name: "fs__file_get", Args: `greet.txt`}}
	got := checkEval(ctx, swapped, evalExpect{Order: []string{"fs__file_get", "fs__file_patch"}})
	if len(got) != 1 || !strings.HasPrefix(got[0], "order:") {
		t.Fatalf("want one order failure, got %v", got)
	}
}

// Off prefix: round 1 hides the fs schema; after mcp_enable round 2
// publishes it.
func TestEvalHarness_OffPrefixEnableThenCall(t *testing.T) {
	ctx := context.Background()
	fx := evalFixture{
		Name:    "off_prefix",
		Inbound: "what does greet.txt say?",
		Tools:   []evalTool{{Name: "fs__file_get", Description: "read one file", Result: "hi\n"}},
	}
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{toolCall("c1", "mcp_enable", map[string]any{"prefixes": []string{"fs"}})}},
		{ToolCalls: []provider.ToolCall{toolCall("c2", "fs__file_get", map[string]any{"path": "greet.txt"})}},
		{Content: "It says hi."},
	}}

	out := runEvalFixture(ctx, t, sc, fx)

	if hasToolDef(sc.reqs[0].Tools, "fs__file_get") {
		t.Error("round 1 published an off prefix")
	}
	if !hasToolDef(sc.reqs[0].Tools, "mcp_enable") {
		t.Error("round 1 did not publish mcp_enable")
	}
	if !hasToolDef(sc.reqs[1].Tools, "fs__file_get") {
		t.Error("round 2 did not publish the enabled prefix")
	}
	if got := describeBatches(out); got != "[mcp_enable] → [fs__file_get] → reply" {
		t.Errorf("batches %q", got)
	}
}

// results: the nth call gets the nth result and the last one repeats, so a
// test can fail, then pass.
func TestEvalHarness_ResultsSequence(t *testing.T) {
	ctx := context.Background()
	fx := evalFixture{
		Name:    "results_sequence",
		Inbound: "run the tests until they pass",
		Force:   []string{"shell"},
		Tools:   []evalTool{{Name: "shell__command_run", Description: "run", Results: []string{"exit 1\nFAIL", "exit 0\nok"}}},
	}
	run := toolCall("c", "shell__command_run", map[string]any{"command": "go test ./..."})
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{run}},
		{ToolCalls: []provider.ToolCall{run}},
		{ToolCalls: []provider.ToolCall{run}},
		{Content: "Passing now."},
	}}

	out := runEvalFixture(ctx, t, sc, fx)

	var got []string
	for _, c := range out.Calls {
		got = append(got, c.Result)
	}
	if want := []string{"exit 1\nFAIL", "exit 0\nok", "exit 0\nok"}; !slices.Equal(got, want) {
		t.Fatalf("results %q want %q", got, want)
	}
}

// by_args: a parallel batch of one tool on different files gets each body,
// whatever order the goroutines run in.
func TestEvalHarness_ByArgs(t *testing.T) {
	ctx := context.Background()
	fx := evalFixture{
		Name:    "by_args",
		Inbound: "read a.go and b.go",
		Force:   []string{"fs"},
		Tools: []evalTool{{Name: "fs__file_get", Description: "read", Result: "missing", ByArgs: []evalArgsResult{
			{ArgsRegex: `\ba\.go`, Result: "package a"},
			{ArgsRegex: `\bb\.go`, Result: "package b"},
		}}},
	}
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{
			toolCall("1", "fs__file_get", map[string]any{"path": "b.go"}),
			toolCall("2", "fs__file_get", map[string]any{"path": "a.go"}),
			toolCall("3", "fs__file_get", map[string]any{"path": "c.go"}),
		}},
		{Content: "Read them."},
	}}

	out := runEvalFixture(ctx, t, sc, fx)

	got := map[string]string{}
	for _, c := range out.Calls {
		got[c.Args] = c.Result
	}
	want := map[string]string{`{"path":"a.go"}`: "package a", `{"path":"b.go"}`: "package b", `{"path":"c.go"}`: "missing"}
	if !maps.Equal(got, want) {
		t.Fatalf("results %q want %q", got, want)
	}
}

func TestCheckEval_SameRound(t *testing.T) {
	batch := [][]evalCallExpect{{{Name: "fs__file_get", ArgsRegex: `a\.go`}, {Name: "fs__file_get", ArgsRegex: `b\.go`}}}
	parallel := evalOutcome{Calls: []evalCall{
		{Name: "fs__file_get", Args: `{"path":"b.go"}`, Round: 1},
		{Name: "fs__file_get", Args: `{"path":"a.go"}`, Round: 1},
	}}
	if fails := checkEval(context.Background(), parallel, evalExpect{SameRound: batch}); len(fails) != 0 {
		t.Fatalf("parallel batch failed: %v", fails)
	}
	serial := evalOutcome{Calls: []evalCall{
		{Name: "fs__file_get", Args: `{"path":"a.go"}`, Round: 1},
		{Name: "fs__file_get", Args: `{"path":"b.go"}`, Round: 2},
	}}
	if fails := checkEval(context.Background(), serial, evalExpect{SameRound: batch}); len(fails) != 1 {
		t.Fatalf("serial reads should fail same_round once, got %v", fails)
	}
	// One call cannot stand in for two expectations.
	twice := evalOutcome{Calls: []evalCall{{Name: "fs__file_get", Args: `{"path":"a.go b.go"}`, Round: 1}}}
	if fails := checkEval(context.Background(), twice, evalExpect{SameRound: batch}); len(fails) != 1 {
		t.Fatalf("one call matched two expectations: %v", fails)
	}
}

// script: the first rounds are canned and the model sees them as its own;
// the model is first asked on the round after.
func TestEvalHarness_ScriptRounds(t *testing.T) {
	ctx := context.Background()
	fx := evalFixture{
		Name:    "script_rounds",
		Inbound: "what does greet.txt say?",
		Force:   []string{"fs"},
		Tools:   []evalTool{{Name: "fs__file_get", Description: "read one file", Result: "hi\n"}},
		Script:  []evalScriptRound{{ToolCalls: []evalScriptCall{{Name: "fs__file_get", Args: map[string]any{"path": "greet.txt"}}}}},
	}
	sc := &scriptCompleter{res: []*provider.Result{{Content: "It says hi."}}}

	out := runEvalFixture(ctx, t, sc, fx)

	if len(sc.reqs) != 1 {
		t.Fatalf("model asked %d times, want 1 (round 1 is scripted)", len(sc.reqs))
	}
	if out.Rounds != 2 || describeBatches(out) != "[fs__file_get] → reply" {
		t.Fatalf("rounds=%d batches %q", out.Rounds, describeBatches(out))
	}
	var sawResult bool
	for _, m := range sc.reqs[0].Messages {
		if m.Role == provider.RoleTool && m.Content == "hi\n" {
			sawResult = true
		}
	}
	if !sawResult {
		t.Fatal("the model did not see the scripted call's result")
	}
}

// A memory row the model stores is visible to the memory gate, under any
// kind when the fixture leaves kind empty; {{memory:N}} names seeded ids.
func TestEvalHarness_MemoryRows(t *testing.T) {
	ctx := context.Background()
	fx := evalFixture{
		Name:    "memory_rows",
		Inbound: "the tests here run with make test",
		Memory:  []evalMemory{{Kind: "fact", Subject: "convention/imports", Content: "goimports"}},
	}
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{toolCall("c1", "memory_store", map[string]any{
			"kind": "fact", "subject": "cmd/test", "content": "make test",
		})}},
		{Content: "Noted: make test."},
	}}

	out := runEvalFixture(ctx, t, sc, fx)

	if fails := checkEval(ctx, out, evalExpect{Memory: []evalMemoryExpect{{Subject: "cmd/test"}}}); len(fails) != 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
	if fails := checkEval(ctx, out, evalExpect{Memory: []evalMemoryExpect{{Subject: "cmd/build"}}}); len(fails) != 1 {
		t.Fatalf("missing row must fail: %v", fails)
	}
	if len(out.MemoryIDs) != 1 {
		t.Fatalf("memory ids %v", out.MemoryIDs)
	}
	if got, want := expandEvalIDs(`{{memory:0}} {{memory:9}}`, out.MemoryIDs), fmt.Sprintf("%d 0", out.MemoryIDs[0]); got != want {
		t.Fatalf("expand %q want %q", got, want)
	}
}
