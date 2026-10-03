//go:build integration

package agent_test

// Live behavioral eval — docs/eval_setup.md. Never in `go test ./...`:
//
//	make integration-test              # sources .env, 3 runs per fixture
//	make integration-test EVAL_ARGS='-eval.n=10 -eval.only=edit_then_check'
//
// Each fixture under testdata/eval runs N times against the configured model
// with the shipped persona seed and canned tools; every run must pass. A rule
// that holds two times in three is a rule the persona is not carrying.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/george/internal/agent"
	"github.com/shotah/george/internal/mcp"
	"github.com/shotah/george/internal/provider"
)

var (
	evalN           = flag.Int("eval.n", 3, "runs per fixture; every run must pass")
	evalOnly        = flag.String("eval.only", "", "comma-separated fixture names to run (default all)")
	evalPersonaFlag = flag.String("eval.persona", "", "persona file to test instead of the shipped seed (bake-off)")
	evalVerbose     = flag.Bool("eval.v", false, "dump calls with args and the reply for passing runs too")
)

// evalSelected parses -eval.only; nil means every fixture.
func evalSelected() []string {
	if *evalOnly == "" {
		return nil
	}
	var names []string
	for _, n := range strings.Split(*evalOnly, ",") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// evalTurnTimeout bounds one turn. A local Qwen turn is prefill plus a
// few completer rounds, so the cap sits above the old Flash budget.
const evalTurnTimeout = 10 * time.Minute

func TestEval_Live(t *testing.T) {
	baseURL, apiKey, model := os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	if apiKey == "" || baseURL == "" || model == "" {
		t.Fatal("LLM_BASE_URL, LLM_API_KEY, LLM_MODEL required; put them in .env and run make integration-test. The live eval is the release gate, so it fails instead of skipping")
	}
	effort := os.Getenv("LLM_REASONING_EFFORT")
	completer := provider.New(baseURL, apiKey, model).WithReasoningEffort(effort)
	if *evalPersonaFlag != "" {
		evalPersonaPath = *evalPersonaFlag
	}
	evalLiveTools = evalLiveCatalog
	evalLiveHost = func(t *testing.T, root string, servers []string) agent.Tools {
		return startCodingHost(t, codingManifest(root, slices.Contains(servers, "shell")))
	}
	t.Logf("model %s at %s; reasoning_effort %q; %d run(s) per fixture; persona %s", model, baseURL, effort, *evalN, evalPersonaPath)

	assertCodingCatalog(t, evalLiveCatalog(t))
	assertCodingHostFailSoft(t)
	assertBadDiffFailsRun(t)
	if t.Failed() {
		t.FailNow()
	}

	selected := evalSelected()
	var total evalTotals
	for _, fx := range loadEvalFixtures(t, evalFixtureDir) {
		if selected != nil && !slices.Contains(selected, fx.Name) {
			continue
		}
		t.Run(fx.Name, func(t *testing.T) {
			t.Log(fx.Why)
			var sub evalTotals
			for i := 1; i <= *evalN; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), evalTurnTimeout)
				out := runEvalFixture(ctx, t, completer, fx)
				fails := checkEval(ctx, out, fx.Expect)
				cancel()
				over := overBudget(out, fx.Expect)
				sub.add(out, over != "")
				if len(fails) > 0 {
					t.Errorf("run %d/%d FAIL: %s\n%s", i, *evalN, strings.Join(fails, "; "), describeEval(out))
					continue
				}
				if over != "" {
					over = " (" + over + ")"
				}
				t.Logf("run %d/%d ok%s: %s — %s", i, *evalN, over, describeCost(out), describeBatches(out))
				if *evalVerbose {
					t.Log(describeEval(out))
				}
			}
			t.Logf("%s: %s", fx.Name, sub.String())
			total.merge(sub)
		})
	}
	t.Logf("all fixtures: %s", total.String())
}

// evalLiveCatalog fetches the latest release of every server in
// testdata/eval/mcp.toml, boots them, and returns their published tool
// defs. Once per process; the first tools_from fixture pays for it. The
// binaries are closed as soon as tools/list is in hand — the defs are
// data. A fixture's live servers are booted again per run by evalLiveHost,
// rooted at that run's workspace.
var (
	liveCatalogOnce sync.Once
	liveCatalogDefs []provider.ToolDef
	liveCatalogErr  error
)

func evalLiveCatalog(t *testing.T) []provider.ToolDef {
	t.Helper()
	liveCatalogOnce.Do(func() { liveCatalogDefs, liveCatalogErr = fetchLiveCatalog(t) })
	if liveCatalogErr != nil {
		t.Fatalf("live MCP catalog: %v", liveCatalogErr)
	}
	return liveCatalogDefs
}

func fetchLiveCatalog(t *testing.T) ([]provider.ToolDef, error) {
	manifestPath := filepath.Join(evalFixtureDir, "mcp.toml")
	m, err := mcp.LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	binDir := os.Getenv("EVAL_MCP_BIN")
	if binDir == "" {
		binDir = filepath.Join(os.TempDir(), "george-eval-mcp")
	}
	res, err := mcp.FetchDownloads(m, mcp.FetchOptions{
		OutDir: binDir,
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
		Logf:   func(format string, args ...any) { t.Logf("tools-fetch: "+format, args...) },
	})
	if err != nil {
		return nil, fmt.Errorf("tools-fetch: %w", err)
	}
	t.Logf("live catalog: binaries in %s (installed %d, cached %d)", binDir, len(res.Installed), len(res.Skipped))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	host, err := mcp.Start(ctx, mcp.Options{
		ManifestPath: manifestPath,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = host.Close() }()
	for _, s := range host.ServerHealth() {
		if s.State == mcp.ServerSkipped {
			t.Logf("live catalog: %s skipped (%s): %s", s.Name, s.Reason, s.Note)
		}
	}
	defs := host.Tools()
	t.Logf("live catalog: %d tools from %s", len(defs), manifestPath)
	return defs, nil
}

// codingPrefixes are the workspace servers. Their live tools/list is the
// registration the fixture grades; a renamed release fails here, before a
// model call.
var codingWant = map[string][]string{
	"fs":    {"file_create", "file_get", "file_list", "file_patch", "file_search"},
	"git":   {"commit_create", "commits_list", "diff_get", "stage_update", "status_get"},
	"shell": {"command_run"},
}

// Names the model may invent. They must not be registered.
var codingForbidden = []string{"file_update", "file_grep", "grep", "git_status", "run_command"}

func assertCodingCatalog(t *testing.T, defs []provider.ToolDef) {
	t.Helper()
	byPrefix := map[string][]string{}
	for _, d := range defs {
		prefix, base, ok := strings.Cut(d.Name, "__")
		if !ok {
			continue
		}
		if _, want := codingWant[prefix]; want || prefix == "github" {
			byPrefix[prefix] = append(byPrefix[prefix], base)
		}
		if slices.Contains(codingForbidden, base) {
			t.Errorf("live catalog registered %s", d.Name)
		}
	}
	for prefix, want := range codingWant {
		got := append([]string(nil), byPrefix[prefix]...)
		slices.Sort(got)
		wantSorted := append([]string(nil), want...)
		slices.Sort(wantSorted)
		t.Logf("live catalog %s: %s", prefix, strings.Join(got, ", "))
		if !slices.Equal(got, wantSorted) {
			t.Errorf("live catalog %s = %q, want %q", prefix, strings.Join(got, ", "), strings.Join(wantSorted, ", "))
		}
	}
	if gh := byPrefix["github"]; len(gh) > 0 {
		slices.Sort(gh)
		t.Logf("live catalog github: %s", strings.Join(gh, ", "))
	}
}

// assertCodingHostFailSoft boots the fetched binaries twice: a non-git
// directory must still list and patch, with git the server that skips; a
// manifest without shell must drop command_run and leave fs and git up.
func assertCodingHostFailSoft(t *testing.T) {
	t.Helper()
	assertNonGitSkipsGit(t)
	assertOmitShell(t)
}

func assertNonGitSkipsGit(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	host := startCodingHost(t, codingManifest(root, true))
	logCodingHealth(t, "non-git", host)
	if !serverSkipped(host, "git") {
		t.Errorf("git should skip when --root is not a git repo")
	}
	for _, name := range []string{"fs", "shell"} {
		if serverSkipped(host, name) {
			t.Errorf("%s skipped on a non-git directory", name)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	listed, err := host.Call(ctx, "fs__file_list", json.RawMessage(`{"path":"."}`))
	if err != nil {
		t.Fatalf("fs__file_list: %v", err)
	}
	if !strings.Contains(listed, "greet.txt") {
		t.Fatalf("fs__file_list = %q, want greet.txt", listed)
	}
	got, err := host.Call(ctx, "fs__file_get", json.RawMessage(`{"path":"greet.txt"}`))
	if err != nil {
		t.Fatalf("fs__file_get: %v", err)
	}
	if !strings.Contains(got, "hi") {
		t.Fatalf("fs__file_get = %q, want hi", got)
	}
	diff := "--- greet.txt\n+++ greet.txt\n@@ -1 +1 @@\n-hi\n+hello\n"
	patched, err := host.Call(ctx, "fs__file_patch", json.RawMessage(mustJSON(map[string]string{
		"path": "greet.txt",
		"diff": diff,
	})))
	if err != nil {
		t.Fatalf("fs__file_patch: %v", err)
	}
	t.Logf("fs__file_patch: %s", patched)
	body, err := os.ReadFile(filepath.Join(root, "greet.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "hello") {
		t.Fatalf("greet.txt = %q after patch, want hello", body)
	}
}

func assertOmitShell(t *testing.T) {
	t.Helper()
	root := moduleRoot(t)
	host := startCodingHost(t, codingManifest(root, false))
	logCodingHealth(t, "no-shell", host)
	for _, name := range []string{"fs", "git"} {
		if serverSkipped(host, name) {
			t.Errorf("%s skipped when shell is omitted", name)
		}
	}
	names := toolNames(host)
	for _, want := range []string{"fs__file_get", "fs__file_patch", "git__status_get"} {
		if !slices.Contains(names, want) {
			t.Errorf("omit shell: missing %s (have %s)", want, strings.Join(names, ", "))
		}
	}
	if slices.Contains(names, "shell__command_run") {
		t.Errorf("omit shell: shell__command_run still registered")
	}
}

// assertBadDiffFailsRun scripts parallel_patches against the real fs: a
// malformed diff and a reply that claims success must still fail the run,
// because the files expectation reads what landed.
func assertBadDiffFailsRun(t *testing.T) {
	t.Helper()
	fx := loadEvalFixture(t, filepath.Join(evalFixtureDir, "30_parallel_patches.json"))
	bad := "@@ nonsense @@\n-func Hello(\n+func Greet(\n"
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{
			toolCall("c1", "fs__file_patch", map[string]any{"path": "a.go", "diff": bad}),
			toolCall("c2", "fs__file_patch", map[string]any{"path": "a_test.go", "diff": bad}),
		}},
		{Content: "Renamed Hello to Greet in both files."},
	}}
	out := runEvalFixture(context.Background(), t, sc, fx)
	for _, c := range out.Calls {
		t.Logf("bad diff: %s → %q", c.Name, c.Result)
	}
	fails := checkEval(context.Background(), out, evalExpect{Files: fx.Expect.Files})
	if len(fails) == 0 {
		t.Errorf("a malformed diff passed the files check")
	}
	t.Logf("bad diff fails the run: %s", strings.Join(fails, "; "))
}

func codingManifest(root string, withShell bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `
[[server]]
name = "fs"
command = "fs-mcp"
args = ["--root", %q, "--tool-tier", "core"]

[[server]]
name = "git"
command = "git-mcp"
args = ["--root", %q, "--tool-tier", "core"]
`, root, root)
	if withShell {
		fmt.Fprintf(&b, `
[[server]]
name = "shell"
command = "shell-mcp"
args = ["--root", %q]
`, root)
	}
	return b.String()
}

func startCodingHost(t *testing.T, manifest string) *mcp.Host {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.toml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	host, err := mcp.Start(ctx, mcp.Options{
		ManifestPath: path,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("mcp start: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })
	return host
}

func logCodingHealth(t *testing.T, label string, host *mcp.Host) {
	t.Helper()
	for _, s := range host.ServerHealth() {
		t.Logf("%s %s state=%s reason=%s note=%s", label, s.Name, s.State, s.Reason, s.Note)
	}
}

func serverSkipped(host *mcp.Host, name string) bool {
	for _, s := range host.ServerHealth() {
		if s.Name == name {
			return s.State == mcp.ServerSkipped
		}
	}
	return true
}

func toolNames(host *mcp.Host) []string {
	var names []string
	for _, d := range host.Tools() {
		names = append(names, d.Name)
	}
	slices.Sort(names)
	return names
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// evalTotals is the cost roll-up the bake-off compares: mean rounds, mean
// prompt tokens per turn, and how many runs went over their round_budget.
type evalTotals struct {
	runs, rounds, prompt, completion, over int
}

func (e *evalTotals) add(out evalOutcome, overBudget bool) {
	e.runs++
	e.rounds += out.Rounds
	e.prompt += out.PromptTokens
	e.completion += out.CompletionTokens
	if overBudget {
		e.over++
	}
}

func (e *evalTotals) merge(o evalTotals) {
	e.runs += o.runs
	e.rounds += o.rounds
	e.prompt += o.prompt
	e.completion += o.completion
	e.over += o.over
}

func (e evalTotals) String() string {
	if e.runs == 0 {
		return "no runs"
	}
	n := float64(e.runs)
	s := fmt.Sprintf("%d runs, mean %.2f rounds, mean %.1fk prompt / %.0f completion tokens per turn",
		e.runs, float64(e.rounds)/n, float64(e.prompt)/n/1000, float64(e.completion)/n)
	if e.over > 0 {
		s += fmt.Sprintf(", %d over round budget", e.over)
	}
	return s
}
