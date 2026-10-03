package agent_test

// Behavioral eval harness (docs/eval_setup.md).
//
// A fixture is one turn against the shipped persona seed with a real session,
// memory, mcp_enable, and self-note store — the same composition as
// cmd/george/run.go — and canned MCP tools, or the real fs/shell binaries on
// a seeded workspace for a fixture's live servers. Only the Completer varies: the
// live eval (eval_integration_test.go, build tag `integration`) plugs in the
// real provider; the plumbing test plugs in a scripted fake so the harness
// itself is covered by `go test ./...`.
//
// Expectations are about shape, not prose: which tools were called, with
// what, in what order, and which memory rows landed. The sentence is the part
// that changes run to run.

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/george/internal/agent"
	"github.com/shotah/george/internal/channel"
	"github.com/shotah/george/internal/mcp"
	"github.com/shotah/george/internal/mcpenable"
	"github.com/shotah/george/internal/memory"
	"github.com/shotah/george/internal/persona"
	"github.com/shotah/george/internal/provider"
	"github.com/shotah/george/internal/selfnote"
	"github.com/shotah/george/internal/session"
)

// evalFixtureDir holds one JSON file per scenario.
const evalFixtureDir = "testdata/eval"

// evalPersonaSeed is the PERSONA.md `george init` ships. It is all comment, so
// the default eval grades the contract alone: what a fresh install runs.
const evalPersonaSeed = "../../examples/persona/PERSONA.example.md"

// evalPersonaPath is the PERSONA.md actually loaded; -eval.persona swaps in
// a human's file to check it against the contract.
var evalPersonaPath = evalPersonaSeed

const evalTZ = "America/Los_Angeles"

// evalFixture is one scenario: one human line, one turn.
type evalFixture struct {
	Name string `json:"name"`
	Why  string `json:"why"`
	// Inbound is the human's text.
	Inbound string `json:"inbound"`
	// History is appended to the session before the turn (role user|assistant).
	History []evalHistory `json:"history,omitempty"`
	Memory  []evalMemory  `json:"memory,omitempty"`
	// Self is SELF.md body bullets ("- ..." lines). Empty file when absent.
	Self string `json:"self,omitempty"`
	// Tools are canned MCP tools.
	Tools []evalTool `json:"tools,omitempty"`
	// ToolsFrom names servers in testdata/eval/mcp.toml whose real catalog —
	// names, descriptions, schemas from the latest release binary's
	// tools/list — replaces hand-written defs. Every tool the server
	// publishes is published here; fixture Tools for those servers carry
	// only name + canned result, and a name the live catalog lacks fails
	// before any model call. Needs the integration tag (network).
	ToolsFrom []string `json:"tools_from,omitempty"`
	// Workspace seeds a fresh directory for the turn: path → file body.
	Workspace map[string]string `json:"workspace,omitempty"`
	// Live names tools_from servers whose calls go to the real binary rooted
	// at Workspace instead of the canned result, so a re-read sees the
	// patch and a bad diff stays bad. Canned results for those servers are
	// the stand-in only where no live host is wired (plain `go test`).
	Live []string `json:"live,omitempty"`
	// Force lists MCP prefixes published without mcp_enable (like MCP_ENABLE_FORCE).
	Force []string `json:"force,omitempty"`
	// Script is the first completions of the turn, canned instead of the
	// model: the way to seed a round the pinned model would not produce on
	// its own (a mangled tool name). The live model takes the rounds after.
	Script []evalScriptRound `json:"script,omitempty"`
	// Now freezes the turn clock (RFC3339). Empty uses time.Now.
	Now    string     `json:"now,omitempty"`
	Expect evalExpect `json:"expect"`
}

type evalHistory struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type evalMemory struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Content string `json:"content"`
}

type evalTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Params      map[string]any `json:"params,omitempty"`
	// Result is returned on every call. Results, when set, wins: the nth
	// call gets the nth entry and the last one repeats (a test that fails,
	// then passes).
	Result  string   `json:"result"`
	Results []string `json:"results,omitempty"`
	// ByArgs answers by the call's arguments, first match wins, before
	// Result/Results. A parallel batch runs in any order, so one tool read
	// on three files needs each file's body keyed by its args.
	ByArgs []evalArgsResult `json:"by_args,omitempty"`
}

type evalArgsResult struct {
	ArgsRegex string `json:"args_regex"`
	Result    string `json:"result"`
}

// evalScriptRound is one canned completion: tool calls, or a reply.
type evalScriptRound struct {
	ToolCalls []evalScriptCall `json:"tool_calls,omitempty"`
	Content   string           `json:"content,omitempty"`
}

type evalScriptCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// evalExpect is the shape contract. Every set field must hold. AnyOf holds
// when at least one alternative holds in full.
type evalExpect struct {
	ToolsCalled    []evalCallExpect `json:"tools_called,omitempty"`
	ToolsNotCalled []string         `json:"tools_not_called,omitempty"`
	// Order lists tool names whose first calls must appear in this order.
	Order []string `json:"order,omitempty"`
	// SameRound lists batches: every call in one entry must land in a single
	// completer round, each matched by a distinct call. Args regexes inside
	// an entry must not overlap; matching is greedy.
	SameRound  [][]evalCallExpect `json:"same_round,omitempty"`
	ReplyRegex string             `json:"reply_regex,omitempty"`
	ReplyNot   string             `json:"reply_not_regex,omitempty"`
	// ToolStatsRegex must match `/toolstats` after the turn (a repair count).
	ToolStatsRegex string `json:"toolstats_regex,omitempty"`
	MaxQuestions   *int   `json:"max_questions,omitempty"`
	// MaxToolCalls caps every call in the turn, builtins included: past the
	// answer, any further call fails the run.
	MaxToolCalls *int `json:"max_tool_calls,omitempty"`
	// RoundBudget is the completer rounds the rule needs (tool rounds + the
	// reply). Going over is reported, never failed: the gate is on missing
	// work, and a model that does something extra and useful is not wrong.
	RoundBudget *int               `json:"round_budget,omitempty"`
	Memory      []evalMemoryExpect `json:"memory,omitempty"`
	// Files maps a workspace path to a regex its body must match after the
	// turn: the patch landed, whatever the model said about it.
	Files map[string]string `json:"files,omitempty"`
	// FilesNot maps a workspace path to a regex its body must not match:
	// the shape that should not land (a near-copy beside the original).
	FilesNot map[string]string `json:"files_not,omitempty"`
	AnyOf    []evalExpect      `json:"any_of,omitempty"`
}

type evalCallExpect struct {
	Name      string `json:"name"`
	ArgsRegex string `json:"args_regex,omitempty"`
	// MaxCalls caps how often this tool may be called in the turn (matching
	// ArgsRegex when set).
	MaxCalls *int `json:"max_calls,omitempty"`
}

type evalMemoryExpect struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
}

// evalCall is one recorded tool call, builtin or MCP. Round is the completer
// round that requested it (1-based), so a batch is the calls sharing a round.
type evalCall struct {
	Name   string
	Args   string
	Round  int
	Result string // what the tool returned (empty on error)
}

// evalOutcome is what one turn produced, gathered from the recorder, the
// completer counter, and the stores.
type evalOutcome struct {
	Calls []evalCall
	Reply string
	// ToolStats is the `/toolstats` reply after the turn.
	ToolStats string
	// Err is Handle's error. A turn that errors is a failed run, not a
	// stopped fixture, so the other runs still report.
	Err error
	// Rounds is completer calls for the turn, scripted ones included; the
	// last one is the reply.
	Rounds           int
	PromptTokens     int // native usage summed over rounds; 0 when the provider omits it
	CompletionTokens int
	// MemoryIDs are the seeded memory row ids, in fixture order:
	// {{memory:0}} in an expect regex.
	MemoryIDs []int64
	mem       *memory.Builtin
	// Workspace is the seeded directory; Files expectations read it.
	Workspace string
}

func loadEvalFixtures(t *testing.T, dir string) []evalFixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var out []evalFixture
	for _, p := range paths {
		out = append(out, loadEvalFixture(t, p))
	}
	if len(out) == 0 {
		t.Fatalf("no fixtures under %s", dir)
	}
	return out
}

func loadEvalFixture(t *testing.T, path string) evalFixture {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fx evalFixture
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fx); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if fx.Name == "" {
		fx.Name = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	return fx
}

// evalLiveTools returns the real catalogs of testdata/eval/mcp.toml servers.
// Set by the integration test (network: tools-fetch + boot); nil in plain
// `go test`, where a tools_from fixture is a hard error rather than a
// silently faked schema.
var evalLiveTools func(t *testing.T) []provider.ToolDef

// evalLiveHost boots the named servers rooted at a fixture's workspace. Set
// by the integration test; nil in plain `go test`, where Live servers fall
// back to their canned results.
var evalLiveHost func(t *testing.T, root string, servers []string) agent.Tools

// liveRouted sends calls for Live server prefixes to the real host, and a
// name no canned tool owns there too, so a mangled name meets Host.resolve
// as it would in production. Every other call goes to the canned tools. The
// defs stay the canned (live-catalog) set, so what the model sees does not
// change with the routing.
type liveRouted struct {
	*cannedTools
	live    agent.Tools
	servers []string
}

func (r *liveRouted) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	prefix, _, ok := strings.Cut(name, "__")
	_, canned := r.results[name]
	if (ok && slices.Contains(r.servers, prefix)) || !canned {
		return r.live.Call(ctx, name, args)
	}
	return r.cannedTools.Call(ctx, name, args)
}

// CallStats is the live host's, so /toolstats shows its repairs.
func (r *liveRouted) CallStats() mcp.CallStats {
	if s, ok := r.live.(interface{ CallStats() mcp.CallStats }); ok {
		return s.CallStats()
	}
	return mcp.CallStats{}
}

// seedWorkspace writes a fixture's files under dir.
func seedWorkspace(dir string, files map[string]string) error {
	for path, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// mergeLiveTools builds the canned set for a tools_from fixture: every tool
// the live servers publish, with the fixture's canned result where it gave
// one and "{}" where it did not. A fixture result for a tool the live
// catalog does not have is the drift signal — a sibling release renamed
// or dropped it — and fails here, before a single model call. Fixture
// tools on other servers pass through hand-written.
func mergeLiveTools(live []provider.ToolDef, servers []string, fixture []evalTool) ([]evalTool, error) {
	byName := map[string]evalTool{}
	for _, tl := range fixture {
		byName[tl.Name] = tl
	}
	var out []evalTool
	covered := map[string]bool{}
	for _, server := range servers {
		prefix := server + "__"
		n := 0
		for _, def := range live {
			if !strings.HasPrefix(def.Name, prefix) {
				continue
			}
			n++
			merged := evalTool{Name: def.Name, Description: def.Description, Params: def.Parameters, Result: "{}"}
			if tl, ok := byName[def.Name]; ok {
				merged.Result, merged.Results, merged.ByArgs = tl.Result, tl.Results, tl.ByArgs
			}
			covered[def.Name] = true
			out = append(out, merged)
		}
		if n == 0 {
			return nil, fmt.Errorf("tools_from %q: no live tools (server not connected or not in testdata/eval/mcp.toml)", server)
		}
		for _, tl := range fixture {
			if strings.HasPrefix(tl.Name, prefix) && !covered[tl.Name] {
				return nil, fmt.Errorf("fixture tool %q is not in the live %s catalog (have %s)", tl.Name, server, strings.Join(liveNames(live, prefix), ", "))
			}
		}
	}
	for _, tl := range fixture {
		if !covered[tl.Name] {
			out = append(out, tl)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func liveNames(live []provider.ToolDef, prefix string) []string {
	var names []string
	for _, def := range live {
		if strings.HasPrefix(def.Name, prefix) {
			names = append(names, def.Name)
		}
	}
	sort.Strings(names)
	return names
}

// cannedTools is the innermost Tools: MCP stand-ins that answer verbatim.
type cannedTools struct {
	defs    []provider.ToolDef
	results map[string][]string
	byArgs  map[string][]cannedArgsResult
	mu      sync.Mutex
	calls   map[string]int
}

type cannedArgsResult struct {
	re     *regexp.Regexp
	result string
}

func newCannedTools(tools []evalTool) *cannedTools {
	c := &cannedTools{results: map[string][]string{}, byArgs: map[string][]cannedArgsResult{}, calls: map[string]int{}}
	for _, tl := range tools {
		params := tl.Params
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		c.defs = append(c.defs, provider.ToolDef{Name: tl.Name, Description: tl.Description, Parameters: params})
		results := tl.Results
		if len(results) == 0 {
			results = []string{tl.Result}
		}
		c.results[tl.Name] = results
		for _, ba := range tl.ByArgs {
			c.byArgs[tl.Name] = append(c.byArgs[tl.Name], cannedArgsResult{re: regexp.MustCompile(ba.ArgsRegex), result: ba.Result})
		}
	}
	return c
}

func (c *cannedTools) Tools() []provider.ToolDef { return c.defs }

func (c *cannedTools) ToolCount() int { return len(c.defs) }

func (c *cannedTools) Call(_ context.Context, name string, args json.RawMessage) (string, error) {
	results, ok := c.results[name]
	if !ok {
		return "", fmt.Errorf("eval: no canned tool %q", name)
	}
	for _, ba := range c.byArgs[name] {
		if ba.re.Match(args) {
			return ba.result, nil
		}
	}
	c.mu.Lock()
	i := c.calls[name]
	c.calls[name]++
	c.mu.Unlock()
	return results[min(i, len(results)-1)], nil
}

// countingCompleter answers the first len(script) rounds from the script,
// then hands off to inner. It counts rounds and sums native usage; the
// recorder reads the round so each tool call is tagged with its batch.
type countingCompleter struct {
	inner  provider.Completer
	script []evalScriptRound
	mu     sync.Mutex
	rounds int
	usage  provider.Usage
}

func (c *countingCompleter) Complete(ctx context.Context, req provider.Request) (*provider.Result, error) {
	c.mu.Lock()
	c.rounds++
	n := c.rounds
	c.mu.Unlock()
	if n <= len(c.script) {
		return scriptResult(n, c.script[n-1]), nil
	}
	res, err := c.inner.Complete(ctx, req)
	if res != nil {
		c.mu.Lock()
		c.usage.PromptTokens += res.Usage.PromptTokens
		c.usage.CompletionTokens += res.Usage.CompletionTokens
		c.mu.Unlock()
	}
	return res, err
}

func scriptResult(round int, s evalScriptRound) *provider.Result {
	res := &provider.Result{Content: s.Content}
	for i, call := range s.ToolCalls {
		args := call.Args
		if args == nil {
			args = map[string]any{}
		}
		raw, err := json.Marshal(args)
		if err != nil {
			panic(err)
		}
		res.ToolCalls = append(res.ToolCalls, provider.ToolCall{
			ID: fmt.Sprintf("script-%d-%d", round, i), Name: call.Name, Arguments: string(raw),
		})
	}
	return res
}

func (c *countingCompleter) round() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rounds
}

// recordingTools wraps the outermost composite so builtins and MCP calls are
// both seen, then delegates.
type recordingTools struct {
	inner agent.Tools
	round func() int
	mu    sync.Mutex
	calls []evalCall
}

func (r *recordingTools) Tools() []provider.ToolDef { return r.inner.Tools() }

func (r *recordingTools) ToolCount() int { return r.inner.ToolCount() }

func (r *recordingTools) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, evalCall{Name: name, Args: string(args), Round: r.round()})
	i := len(r.calls) - 1
	r.mu.Unlock()
	out, err := r.inner.Call(ctx, name, args)
	if err == nil {
		r.mu.Lock()
		r.calls[i].Result = out
		r.mu.Unlock()
	}
	return out, err
}

func (r *recordingTools) CallStats() mcp.CallStats {
	if s, ok := r.inner.(interface{ CallStats() mcp.CallStats }); ok {
		return s.CallStats()
	}
	return mcp.CallStats{}
}

func (r *recordingTools) snapshot() []evalCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]evalCall(nil), r.calls...)
}

// evalPersona copies the PERSONA.md under test into a fresh persona dir and
// loads it the way boot does: contract first.
func evalPersona(t *testing.T, dir string) string {
	t.Helper()
	seed, err := os.ReadFile(evalPersonaPath)
	if err != nil {
		t.Fatalf("read seed %s: %v", evalPersonaPath, err)
	}
	if err := os.WriteFile(filepath.Join(dir, persona.FilePersona), seed, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := persona.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// runEvalFixture builds a fresh world, runs the one turn, and gathers the
// outcome. The stores are closed when the test ends.
func runEvalFixture(ctx context.Context, t *testing.T, completer provider.Completer, fx evalFixture) evalOutcome {
	t.Helper()
	loc, err := time.LoadLocation(evalTZ)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	personaDir := filepath.Join(root, "persona")
	dataDir := filepath.Join(root, "data")
	workDir := filepath.Join(root, "work")
	for _, d := range []string{personaDir, dataDir, workDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := seedWorkspace(workDir, fx.Workspace); err != nil {
		t.Fatalf("%s: workspace: %v", fx.Name, err)
	}
	if fx.Self != "" {
		if err := os.WriteFile(filepath.Join(personaDir, selfnote.FileName), []byte(fx.Self+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	personaText := evalPersona(t, personaDir)

	// One george.db handle for every store, as run.go does. A second handle
	// on the same file (memory.Open) makes a parallel tool batch fight over
	// the write lock — SQLITE_BUSY that production never sees.
	sessions, err := session.Open(dataDir, 50, 100000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessions.Close() })
	mem, err := memory.OpenDB(sessions.DB())
	if err != nil {
		t.Fatal(err)
	}
	var memoryIDs []int64
	for _, row := range fx.Memory {
		e, err := mem.Store(ctx, row.Kind, row.Subject, row.Content)
		if err != nil {
			t.Fatalf("seed memory %s/%s: %v", row.Kind, row.Subject, err)
		}
		memoryIDs = append(memoryIDs, e.ID)
	}
	enable, err := mcpenable.OpenDB(sessions.DB())
	if err != nil {
		t.Fatal(err)
	}
	self, err := selfnote.Open(personaDir)
	if err != nil {
		t.Fatal(err)
	}

	sessionID := "eval-" + fx.Name
	for _, h := range fx.History {
		role := session.RoleUser
		if h.Role == "assistant" {
			role = session.RoleAssistant
		}
		if err := sessions.Append(ctx, sessionID, session.Message{Role: role, Content: h.Text}); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().In(loc)
	if fx.Now != "" {
		parsed, perr := time.Parse(time.RFC3339, fx.Now)
		if perr != nil {
			t.Fatalf("%s: now: %v", fx.Name, perr)
		}
		now = parsed.In(loc)
	}
	canned := fx.Tools
	if len(fx.ToolsFrom) > 0 {
		if evalLiveTools == nil {
			t.Fatalf("%s: tools_from needs the live MCP catalog; run under the integration tag", fx.Name)
		}
		canned, err = mergeLiveTools(evalLiveTools(t), fx.ToolsFrom, fx.Tools)
		if err != nil {
			t.Fatalf("%s: %v", fx.Name, err)
		}
	}

	// Same stack as cmd/george/run.go, canned MCP host at the bottom,
	// recorder on top.
	cannedSet := newCannedTools(canned)
	var tools agent.Tools = cannedSet
	if len(fx.Live) > 0 && evalLiveHost != nil {
		tools = &liveRouted{cannedTools: cannedSet, live: evalLiveHost(t, workDir, fx.Live), servers: fx.Live}
	}
	tools = memory.Composite{Memory: memory.Tools{Backend: mem}, Other: tools}
	tools = selfnote.Composite{Self: selfnote.Tools{Store: self}, Other: tools}
	base := tools
	tools = mcpenable.Composite{
		Enable: mcpenable.Tools{Store: enable, Index: func() []string { return mcpenable.Index(base.Tools()) }},
		Other:  base,
	}
	counter := &countingCompleter{inner: completer, script: fx.Script}
	rec := &recordingTools{inner: tools, round: counter.round}

	a, err := agent.New(agent.Options{
		Persona:     personaText,
		Completer:   counter,
		Sessions:    sessions,
		Memory:      mem,
		Tools:       rec,
		SelfNotes:   self,
		Enable:      enable,
		EnableForce: mcpenable.Force{Prefixes: fx.Force},
		Model:       "eval",
		Location:    loc,
		TZName:      evalTZ,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := a.Handle(ctx, channel.Message{SessionID: sessionID, UserID: "eval", Text: fx.Inbound})
	calls := rec.snapshot()
	toolStats, _ := a.Handle(ctx, channel.Message{SessionID: sessionID, UserID: "eval", Text: "/toolstats"})
	return evalOutcome{
		Calls:            calls,
		Reply:            reply,
		ToolStats:        toolStats,
		Err:              err,
		Rounds:           counter.round(),
		PromptTokens:     counter.usage.PromptTokens,
		CompletionTokens: counter.usage.CompletionTokens,
		MemoryIDs:        memoryIDs,
		mem:              mem,
		Workspace:        workDir,
	}
}

var evalIDRef = regexp.MustCompile(`\{\{memory:(\d+)\}\}`)

func expandEvalExpect(want evalExpect, ids []int64) evalExpect {
	for i := range want.ToolsCalled {
		want.ToolsCalled[i].ArgsRegex = expandEvalIDs(want.ToolsCalled[i].ArgsRegex, ids)
	}
	for i := range want.AnyOf {
		want.AnyOf[i] = expandEvalExpect(want.AnyOf[i], ids)
	}
	if want.ReplyRegex != "" {
		want.ReplyRegex = expandEvalIDs(want.ReplyRegex, ids)
	}
	if want.ReplyNot != "" {
		want.ReplyNot = expandEvalIDs(want.ReplyNot, ids)
	}
	return want
}

// expandEvalIDs replaces {{memory:N}} with the seeded row id. An index off
// the end is "0", which matches nothing real.
func expandEvalIDs(s string, ids []int64) string {
	return evalIDRef.ReplaceAllStringFunc(s, func(m string) string {
		var n int
		if _, err := fmt.Sscanf(evalIDRef.FindStringSubmatch(m)[1], "%d", &n); err != nil || n >= len(ids) {
			return "0"
		}
		return fmt.Sprintf("%d", ids[n])
	})
}

// checkEval returns one line per failed expectation. Empty means pass.
func checkEval(ctx context.Context, out evalOutcome, want evalExpect) []string {
	want = expandEvalExpect(want, out.MemoryIDs)
	var fails []string
	if out.Err != nil {
		fails = append(fails, "turn errored: "+out.Err.Error())
	}
	reply := out.Reply

	for _, c := range want.ToolsCalled {
		if !calledMatching(out.Calls, c) {
			fails = append(fails, fmt.Sprintf("expected tool %s%s", c.Name, argsNote(c.ArgsRegex)))
			continue
		}
		if c.MaxCalls != nil {
			if n := countMatching(out.Calls, c); n > *c.MaxCalls {
				fails = append(fails, fmt.Sprintf("tool %s called %d times, max %d", c.Name, n, *c.MaxCalls))
			}
		}
	}
	for _, name := range want.ToolsNotCalled {
		if calledMatching(out.Calls, evalCallExpect{Name: name}) {
			fails = append(fails, "unexpected tool "+name)
		}
	}
	if want.MaxToolCalls != nil && len(out.Calls) > *want.MaxToolCalls {
		fails = append(fails, fmt.Sprintf("%d tool calls, max %d", len(out.Calls), *want.MaxToolCalls))
	}
	if len(want.Order) > 1 {
		last := -1
		for _, name := range want.Order {
			i := firstCall(out.Calls, name)
			if i < 0 {
				fails = append(fails, "order: "+name+" never called")
				break
			}
			if i <= last {
				fails = append(fails, "order: "+name+" before "+strings.Join(want.Order, " → "))
				break
			}
			last = i
		}
	}
	for _, batch := range want.SameRound {
		if !sameRound(out.Calls, batch) {
			fails = append(fails, "same_round: "+describeCallExpects(batch)+" not in one batch")
		}
	}
	if want.ReplyRegex != "" && !regexp.MustCompile(want.ReplyRegex).MatchString(reply) {
		fails = append(fails, "reply should match /"+want.ReplyRegex+"/")
	}
	if want.ReplyNot != "" && regexp.MustCompile(want.ReplyNot).MatchString(reply) {
		fails = append(fails, "reply should not match /"+want.ReplyNot+"/")
	}
	if want.ToolStatsRegex != "" && !regexp.MustCompile(want.ToolStatsRegex).MatchString(out.ToolStats) {
		fails = append(fails, "toolstats should match /"+want.ToolStatsRegex+"/, got "+strings.ReplaceAll(out.ToolStats, "\n", " | "))
	}
	if want.MaxQuestions != nil {
		if n := strings.Count(reply, "?") + strings.Count(reply, "？"); n > *want.MaxQuestions {
			fails = append(fails, fmt.Sprintf("%d questions, max %d", n, *want.MaxQuestions))
		}
	}
	for _, m := range want.Memory {
		if !memoryRowExists(ctx, out.mem, m) {
			fails = append(fails, "expected memory row "+m.Kind+" "+m.Subject)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(want.Files)) {
		body, err := os.ReadFile(filepath.Join(out.Workspace, filepath.FromSlash(path)))
		if err != nil {
			fails = append(fails, "file "+path+": "+err.Error())
			continue
		}
		if re := want.Files[path]; !regexp.MustCompile(re).Match(body) {
			fails = append(fails, fmt.Sprintf("file %s should match /%s/, is %q", path, re, body))
		}
	}
	for _, path := range slices.Sorted(maps.Keys(want.FilesNot)) {
		body, err := os.ReadFile(filepath.Join(out.Workspace, filepath.FromSlash(path)))
		if err != nil {
			fails = append(fails, "file "+path+": "+err.Error())
			continue
		}
		if re := want.FilesNot[path]; regexp.MustCompile(re).Match(body) {
			fails = append(fails, fmt.Sprintf("file %s should not match /%s/, is %q", path, re, body))
		}
	}
	if len(want.AnyOf) > 0 {
		var alts []string
		for i, alt := range want.AnyOf {
			sub := checkEval(ctx, out, alt)
			if len(sub) == 0 {
				alts = nil
				break
			}
			alts = append(alts, fmt.Sprintf("alt %d: %s", i+1, strings.Join(sub, "; ")))
		}
		if len(alts) > 0 {
			fails = append(fails, "none of any_of held — "+strings.Join(alts, " | "))
		}
	}
	return fails
}

// memoryRowExists checks a live row by subject; empty Kind accepts any of the
// kinds a model plausibly picks for it.
func memoryRowExists(ctx context.Context, mem *memory.Builtin, m evalMemoryExpect) bool {
	if mem == nil {
		return false
	}
	kinds := []string{m.Kind}
	if m.Kind == "" {
		kinds = []string{memory.KindPreference, memory.KindFact, memory.KindInsight}
	}
	for _, kind := range kinds {
		if _, ok, err := mem.ActiveByKindSubject(ctx, kind, m.Subject); err == nil && ok {
			return true
		}
	}
	return false
}

func calledMatching(calls []evalCall, c evalCallExpect) bool {
	return countMatching(calls, c) > 0
}

func countMatching(calls []evalCall, c evalCallExpect) int {
	var re *regexp.Regexp
	if c.ArgsRegex != "" {
		re = regexp.MustCompile(c.ArgsRegex)
	}
	n := 0
	for _, call := range calls {
		if call.Name != c.Name {
			continue
		}
		if re == nil || re.MatchString(call.Args) {
			n++
		}
	}
	return n
}

// sameRound reports whether some single round holds a distinct call for
// every expectation in batch.
func sameRound(calls []evalCall, batch []evalCallExpect) bool {
	rounds := map[int][]evalCall{}
	for _, c := range calls {
		rounds[c.Round] = append(rounds[c.Round], c)
	}
	for _, inRound := range rounds {
		used := make([]bool, len(inRound))
		matched := 0
		for _, want := range batch {
			for i, c := range inRound {
				if !used[i] && countMatching([]evalCall{c}, want) == 1 {
					used[i] = true
					matched++
					break
				}
			}
		}
		if matched == len(batch) {
			return true
		}
	}
	return false
}

func describeCallExpects(batch []evalCallExpect) string {
	parts := make([]string, len(batch))
	for i, c := range batch {
		parts[i] = c.Name + argsNote(c.ArgsRegex)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func firstCall(calls []evalCall, name string) int {
	for i, c := range calls {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func argsNote(re string) string {
	if re == "" {
		return ""
	}
	return " with args /" + re + "/"
}

// overBudget is the cost note for a run that took more rounds than the
// fixture's round_budget — printed beside a passing run, never a failure.
// Empty when there is no budget or the run stayed inside it.
func overBudget(out evalOutcome, want evalExpect) string {
	if want.RoundBudget == nil || out.Rounds <= *want.RoundBudget {
		return ""
	}
	return fmt.Sprintf("over budget: %d rounds, budget %d", out.Rounds, *want.RoundBudget)
}

// describeBatches is the turn's shape: tool calls grouped by completer
// round, then the reply. "[a b] → [c] → reply" is three rounds; the
// arrows are the serial cost.
func describeBatches(out evalOutcome) string {
	var parts []string
	for r := 1; r <= out.Rounds; r++ {
		var names []string
		for _, c := range out.Calls {
			if c.Round == r {
				names = append(names, c.Name)
			}
		}
		if len(names) > 0 {
			parts = append(parts, "["+strings.Join(names, " ")+"]")
		}
	}
	parts = append(parts, "reply")
	return strings.Join(parts, " → ")
}

// describeCost is the per-run cost line: rounds and native tokens when the
// provider reports them.
func describeCost(out evalOutcome) string {
	if out.PromptTokens == 0 {
		return fmt.Sprintf("%d rounds", out.Rounds)
	}
	return fmt.Sprintf("%d rounds, %.1fk prompt / %d completion tokens", out.Rounds, float64(out.PromptTokens)/1000, out.CompletionTokens)
}

// describeEval is the failure dump: calls, then the reply.
func describeEval(out evalOutcome) string {
	var b strings.Builder
	b.WriteString("--- tool calls ---\n")
	if len(out.Calls) == 0 {
		b.WriteString("(none)\n")
	}
	for _, c := range out.Calls {
		fmt.Fprintf(&b, "r%d %s %s\n", c.Round, c.Name, c.Args)
	}
	fmt.Fprintf(&b, "--- %s: %s ---\n", describeCost(out), describeBatches(out))
	b.WriteString("--- reply ---\n")
	b.WriteString(out.Reply)
	b.WriteString("\n")
	return b.String()
}
