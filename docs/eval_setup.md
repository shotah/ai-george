# Eval setup

> Fixtures: `internal/agent/testdata/eval/`

The live reading grades one local model: the Qwen3.6-35B-A3B coding build at
Q4_K_M. The id is `qwen3.6:35b-a3b-coding`, served by Ollama on this machine.
The family tag `qwen3.6:latest` names a moving target; this id names the size
and the coding build. It replaced `qwen3-coder:30b-a3b-q4_K_M`, which never
passed `parallel_patches` (0/5); this one passed it 3/3.

The eval replays each fixture against that model with the shipped persona
seed. Tools are canned, except a fixture's `live` servers (`fs`, `shell`):
those are the real binaries rooted at a temp dir seeded from the fixture's
`workspace`, and `expect.files` checks what the patch left on disk.
`make integration-test` sources `.env` and the harness reads:

| Value | Local | Harness |
| --- | --- | --- |
| `LLM_BASE_URL` | `.env` | Required. Empty fails the run. |
| `LLM_MODEL` | `.env` | Required. Printed on the first log line. |
| `LLM_API_KEY` | `.env` | Required. `ollama` is enough for a local server. |
| `LLM_REASONING_EFFORT` | `.env` | Sent as `reasoning_effort`. `none` on this model, so the completion budget is spent on tool calls. Empty omits the field. |
| `GITHUB_TOKEN` (optional) | `.env` | Lifts the GitHub API rate limit for the real-catalog fetch. Any token, no scopes needed. |

## 1. Local

1. Ollama listening on this machine, then the weights:

   ```sh
   ollama pull qwen3.6:35b-a3b-coding
   ```

   About 22GB. The tag is the Q4_K_M coding build of Qwen3.6-35B-A3B
   (35.5B total, 3B active), with Ollama's `qwen3.5` renderer and parser.
   It thinks by default; `LLM_REASONING_EFFORT=none` turns that off, and
   the gate runs with it off. The model advertises a 256k context.
   On a machine with a lot of shared memory Ollama will reserve that
   whole window (tens of GB of cache) and the first turn sits in
   warmup. Set `OLLAMA_CONTEXT_LENGTH=32768` on the Ollama service
   before the run.

2. Copy the example. The lines at the top are already this model:

   ```sh
   cp .env.example .env
   ```

   ```sh
   LLM_BASE_URL=http://127.0.0.1:11434/v1
   LLM_API_KEY=ollama
   LLM_MODEL=qwen3.6:35b-a3b-coding
   LLM_REASONING_EFFORT=none
   ```

   `.env` is gitignored. The eval is a reading of the seed on this id.

3. One fixture, one run, before the whole directory:

   ```sh
   make integration-test EVAL_ARGS='-eval.n=1 -eval.only=edit_then_check'
   ```

   The target sources `.env`, then runs `go test -tags integration` on
   `TestEval_Live`. The first log lines are the model id, the endpoint,
   `reasoning_effort`, and the MCP releases it fetched. A turn is prefill
   plus decode on the local GPU, about 20s on this model. Each turn is
   capped at 10 minutes. The process cap is `-timeout $(EVAL_TIMEOUT)`
   (default `120m`); Go's own default of 10m kills a long run mid-fixture
   with `panic: test timed out`.

   Without the three values it fails. The live eval is the release gate, so
   it does not skip:

   ```text
   --- FAIL: TestEval_Live (0.00s)
       eval_integration_test.go:63: LLM_BASE_URL, LLM_API_KEY, LLM_MODEL required; ...
   ```

4. Iterate narrow, stamp wide. Iterate at `-eval.n=1` to `3`; `-eval.n=5`
   stamps an item done:

   ```sh
   make integration-test EVAL_ARGS='-eval.n=3 -eval.only=parallel_patches'
   make integration-test EVAL_ARGS='-eval.n=3 -eval.only=edit_then_check,stop_when_done'
   make integration-test EVAL_ARGS='-eval.n=5'                      # the stamp
   ```

   `-eval.only` takes fixture `name`s (the JSON field, not the file name),
   comma-separated. Two more flags: `-eval.v` prints every call with its
   args and the reply for passing runs too (the failure dump, on green);
   `-eval.persona=path` loads that file instead of the shipped seed.

   Every passing run prints its cost and shape:

   ```text
   run 2/3 ok: 4 rounds, 18.2k prompt / 362 completion tokens — [fs__file_get fs__file_get] → [fs__file_patch fs__file_patch] → [shell__command_run] → reply
   parallel_patches: 3 runs, mean 4.67 rounds, mean 21.3k prompt / 398 completion tokens per turn, 1 over round budget
   ...
   all fixtures: 12 runs, mean 4.83 rounds, mean 22.5k prompt / 269 completion tokens per turn, 2 over round budget
   ```

   Rounds are Completer calls for the turn; the last one is the reply, so
   one tool batch plus the answer is 2. Brackets are the calls that went
   out in one batch; each `→` is a serial round. Tokens are the
   provider's native usage summed over rounds (0 when it omits them).

   The eval caches MCP binaries in `EVAL_MCP_BIN` (default
   `$TMPDIR/george-eval-mcp`) and skips a server that is already there.
   After a server release, `rm -rf /tmp/george-eval-mcp` so the next run
   fetches `latest`.

   Windows: the target is POSIX shell. Set the four values in the
   session and run the `go test` line from the Makefile directly.

## 2. GitHub

The Actions job is a different runner. `eval.yml` on `ubuntu-latest` reads
`LLM_API_KEY` from a **secret** and `LLM_BASE_URL` plus `LLM_MODEL` from
**variables**. Variables show up in the run log. That job does not start
Ollama, does not pull these weights, and does not pass
`LLM_REASONING_EFFORT`. The reading in section 1 is the one on this machine.

Wire those three only when the runner can reach the endpoint you name:

**Settings → Secrets and variables → Actions**

- **Secrets** tab → *New repository secret*
  - Name `LLM_API_KEY`, value: the key.
- **Variables** tab → *New repository variable*
  - Name `LLM_BASE_URL`, value: the endpoint.
  - Name `LLM_MODEL`, value: the model id.

The hosted gate is OpenRouter, serving Qwen3.6-35B-A3B. With the `gh`
CLI from the repo (the key is from openrouter.ai → Keys):

```sh
gh secret set LLM_API_KEY
gh variable set LLM_BASE_URL --body 'https://openrouter.ai/api/v1'
gh variable set LLM_MODEL --body 'qwen/qwen3.6-35b-a3b'
```

That is Qwen3.6-35B-A3B at a provider's quantization, chat template, and
sampling, not Ollama's coding tag and `qwen3.5` renderer, so a CI green is not a
reading on this machine. At about $0.15/M prompt tokens a default run (3 runs
of each fixture, about 24k prompt tokens a turn) costs a few cents.

`http://127.0.0.1:11434/v1` in that variable is the runner's loopback, so
the local build is graded only on a self-hosted runner that already has
Ollama and the pulled weights. Two workflows already read the three values:

- **`eval.yml`** — *Actions → eval → Run workflow*. Inputs: `runs` (default
  3) and `only` (a fixture name, blank = all). By hand:

  ```sh
  gh workflow run eval.yml -f runs=5 -f only=parallel_patches
  ```

- **`release.yml`** — on a `v*` tag push the `eval` job runs first and
  GoReleaser `needs` it. Eval fails → no release. Fix the fixture or the
  persona, then re-run the failed job from the Actions tab (or push the
  next tag).

Without the secret the eval job fails with `::error::LLM_API_KEY not set`,
so a fork with no key cannot release. GitHub does not hand secrets to fork
pull requests, and `ci.yml` does not call the eval — the free goldens are
the PR gate; this is the release gate.

## 3. Reading a failure

Every failed run prints what the model did, then the reply:

```text
run 1/2 FAIL: reply should match /Greet/
--- tool calls ---
r1 fs__file_get {"path":"a_test.go"}
r1 fs__file_get {"path":"a.go"}
r2 fs__file_patch {"count":1,"new":"\tif Greet(\"bob\") ...","old":"\tif Hello(\"bob\") ...","path":"a_test.go"}
...
--- reply ---
Files look correct. Want me to run the tests? (`go test`)
```

| Failure line | Means |
| --- | --- |
| `turn errored: …` | The turn returned an error instead of a reply. |
| `expected tool X` | The recorder never saw `X`. A call the agent **blocked** (prefix off, never `mcp_enable`d) also never reaches the recorder — that is a real miss, not a harness gap. |
| `expected tool X with args /re/` | `X` was called, but no call's JSON args matched. |
| `tool X called N times, max M` | `tools_called[].max_calls`: the same call again for nothing. |
| `unexpected tool X` | `tools_not_called`: e.g. `git__commit_create` on a turn that never asked for a commit. |
| `N tool calls, max M` | `max_tool_calls`: past the answer, any further call fails (`stop_when_done`). |
| `order: X before …` · `order: X never called` | Wrong order (a patch before the read), or a link in the chain missing. |
| `same_round: … not in one batch` | Independent calls went out in separate rounds instead of one parallel batch. |
| `file P should match /re/, is "…"` · `file P: …` | `expect.files`: what the patch left on disk, whatever the reply said. |
| `reply should match /re/` · `should not match` | Shape of the reply — it names what changed. Never a sentence. |
| `N questions, max M` | Counted `?` in the reply. |
| `expected memory row kind subject` | No live `memory_store` row with that subject after the turn. |
| `fixture tool X is not in the live S catalog (have …)` | Drift: the server's latest release no longer publishes that name. Fails before any model call. |
| `none of any_of held — alt 1: … \| alt 2: …` | Every alternative failed; each is listed with its own reason. |

A rule that holds two runs in three is a rule the seed is not carrying —
that is why every run must pass.

One line is **not** a failure. `run 1/3 ok (over budget: 6 rounds, budget
4)` means the turn passed and took more Completer rounds than the
fixture's `round_budget`. The gate is on missing work; a model that does
something extra and useful is not wrong, so the budget is reported beside
the pass and summed at the end (`… 1 over round budget`), never failed.
The batches on that line show where the extra round went. Read it as a
trend across runs: a fixture that is always over budget has a sentence in
the seed or a tool error costing a round for nothing; one run in five is
the model being thorough.

## 4. Tuning

Three kinds of red:

- **The fixture is wrong.** The model ran `go test`, as the contract says,
  but `shell` was canned and answered `{}`, so it retried for 24 rounds.
  Fix the JSON: make the server `live`, seed the file the check needs
  (`go.mod`), set `round_budget` to the rounds the rule really takes.
- **A tool is wrong.** A patch is rejected with an error that does not say
  why, and the model re-reads and retries. That belongs to the server's
  repo; the requests and their checks are [mcp_todo.md](mcp_todo.md).
- **The seed lost a rule.** It committed unasked, or stopped on "let me
  first…". Do not loosen the fixture. Change one line in
  `internal/persona/contract.md`, run `-eval.only` on that fixture until it
  holds, then the whole set. Abstract rules ("least change", "KISS") do
  nothing on this model; concrete lines with a tool name do.

Regexes are Go `regexp` (RE2): `(?i)` for case-insensitive, no lookaround.
Fixtures are validated in `go test ./...` before any model call:

```sh
go test ./internal/agent/ -run 'TestEvalFixtures_WellFormed|TestEvalHarness'
```

### The bake-off

The same run is how a persona edit earns its place:

```sh
cp examples/persona/PERSONA.example.md /tmp/candidate.md
# edit /tmp/candidate.md
make integration-test EVAL_ARGS='-eval.n=5'                                   # seed: the number to beat
make integration-test EVAL_ARGS='-eval.n=5 -eval.persona=/tmp/candidate.md'   # candidate
```

Compare the two `all fixtures:` lines and the per-fixture means. A
candidate wins when every run still passes and rounds or tokens fall;
n=3 is too noisy to call a 0.2-round difference, use 5 or more. A
candidate that passes with fewer rounds because it skips the check is a
loss. Take the reading at `LLM_REASONING_EFFORT=none`.

## 5. Adding a fixture

Copy the nearest file under `internal/agent/testdata/eval/` and edit.

| Field | What |
| --- | --- |
| `name`, `why` | `name` is what `-eval.only` matches; `why` is the rule in one sentence, printed at the top of the run. |
| `inbound` | The human's text. |
| `history` | Prior `user` / `assistant` turns appended to the session first. |
| `memory` | Seed rows: `kind`, `subject`, `content`. |
| `self` | `SELF.md` body (`- ` bullets). Empty file when absent. |
| `tools` | Canned MCP tools: `name` (must be `server__name`), `description`, optional `params` schema, and `result`, `results` (the nth call gets the nth entry), or `by_args` (answer by an args regex, for a parallel batch). |
| `tools_from` | Servers whose **real** catalog replaces hand-written defs — see below. `tools` entries for those servers carry only `name` + `result`. |
| `live` | `tools_from` servers whose calls go to the real binary, rooted at a temp dir seeded from `workspace`. Use it whenever the model will read back what it wrote or run a check. |
| `workspace` | Path → file body, written to that temp dir before the turn. Seed what the check needs (`go.mod` for `go test`). |
| `force` | MCP prefixes published without `mcp_enable`. Leave empty to test the off → enable → call path. |
| `script` | Canned first completions, to seed a round the model would not produce (a mangled tool name). The live model takes the rounds after. |
| `now` | Freeze the turn clock (RFC3339). |
| `expect` | The shape contract — see the failure table above for each key. `round_budget` is the cost note, not a check: the rounds the rule needs, reply included (read, patch, test, reply = 4). |

### Real catalogs (`tools_from`)

A hand-written `params` block is the eval's guess at an MCP's interface,
and the guess drifts. `tools_from: ["fs"]` publishes the server's real
catalog instead. At run time the harness reads
`internal/agent/testdata/eval/mcp.toml`, pulls each server's **latest
GitHub release** with the same code as `george tools-fetch`, boots it, and
takes `tools/list` — names, descriptions, schemas — as the tool defs.
Canned results stay canned unless the server is also `live`.

Two things fail for free, before any model call: a fixture `tools` entry
whose name the live server does not publish (the release renamed or
dropped it — the message lists what it does have), and a `tools_from`
server missing from the manifest.

Then:

```sh
go test ./internal/agent/ -run TestEvalFixtures_WellFormed     # parses, regexes compile
make integration-test EVAL_ARGS='-eval.n=1 -eval.only=<name>'   # first live read
```

The six fixtures on disk (`27`–`32`) are the coding loop: one edit with a
check (`edit_then_check`), three batch shapes (`parallel_reads`,
`parallel_checks`, `parallel_patches`), and two stopping rules
(`commit_only_when_asked`, `stop_when_done`). A new fixture is worth
adding when it measures a rule the contract wants to teach, for example a
task where expanding an existing function passes and a near-copy beside it
fails.
