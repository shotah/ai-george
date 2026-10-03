# Eval setup

> Fixtures: `internal/agent/testdata/eval/`

The live reading grades one local model: Qwen3-Coder-30B-A3B at Q4_K_M.
The id is `qwen3-coder:30b-a3b-q4_K_M`, served by Ollama on this machine.
The short tag `qwen3-coder:30b` and the family tag `qwen3-coder:latest`
name a moving target; this id names the size and the quant.

The eval replays each fixture against that model with the shipped persona
seed. Tools are canned, except a fixture's `live` servers (`fs`, `shell`):
those are the real binaries rooted at a temp dir seeded from the fixture's
`workspace`, and `expect.files` checks what the patch left on disk.
`make integration-test` sources `.env` and the harness reads:

| Value | Local | Harness |
| --- | --- | --- |
| `LLM_BASE_URL` | `.env` | Required. Empty skips the run. |
| `LLM_MODEL` | `.env` | Required. Printed on the first log line. |
| `LLM_API_KEY` | `.env` | Required. `ollama` is enough for a local server. |
| `LLM_REASONING_EFFORT` | `.env` | Sent as `reasoning_effort`. `none` on this model, so the completion budget is spent on tool calls. Empty omits the field. |
| `GITHUB_TOKEN` (optional) | `.env` | Lifts the GitHub API rate limit for the real-catalog fetch. Any token, no scopes needed. |

## 1. Local

1. Ollama listening on this machine, then the weights:

   ```sh
   ollama pull qwen3-coder:30b-a3b-q4_K_M
   ```

   About 19GB. The tag is the Q4_K_M build of Qwen3-Coder-30B-A3B
   (30.5B total, 3.3B active). The model advertises a 256k context.
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
   LLM_MODEL=qwen3-coder:30b-a3b-q4_K_M
   LLM_REASONING_EFFORT=none
   ```

   `.env` is gitignored. The eval is a reading of the seed on this id.

3. One fixture, one run, before the whole directory:

   ```sh
   make integration-test EVAL_ARGS='-eval.n=1 -eval.only=scoop_at_2'
   ```

   The target sources `.env`, then runs `go test -tags integration` on
   `TestEval_Live`. The first log line is the model id, the endpoint,
   and `reasoning_effort`. A turn here is prefill plus decode on the
   local GPU, so start with one fixture. Each turn is capped at 10
   minutes. The process cap is `-timeout $(EVAL_TIMEOUT)` (default
   `120m`); Go's own default of 10 m kills a long run mid-fixture with
   `panic: test timed out`. A full sweep at 3 runs can outlast 120 m —
   raise `EVAL_TIMEOUT`, or keep `-eval.only` narrow.

   Without the three values it does not fail, it skips:

   ```text
   --- SKIP: TestEval_Live (0.00s)
       eval_integration_test.go:61: LLM_BASE_URL, LLM_API_KEY, LLM_MODEL required; ...
   ```

4. Narrow it while you tune:

   ```sh
   make integration-test EVAL_ARGS='-eval.only=scoop_at_2'          # one fixture
   make integration-test EVAL_ARGS='-eval.n=10'                     # nervous
   make integration-test EVAL_ARGS='-eval.n=3 -eval.only=no_aims_one_question,planner_gym_no_workout'
   ```

   `-eval.only` takes fixture `name`s (the JSON field, not the file name),
   comma-separated. Two more flags: `-eval.v` prints every call with its
   args and the reply for passing runs too (the failure dump, on green);
   `-eval.persona=path` loads that file instead of the shipped seed — see
   the bake-off below.

   Every passing run prints its cost and shape:

   ```text
   run 1/3 ok: 2 rounds, 10.7k prompt / 259 completion tokens — [cron_schedule memory_store google__calendar_create_events] → reply
   scoop_at_2: 3 runs, mean 2.00 rounds, mean 10.7k prompt / 259 completion tokens per turn
   ...
   all fixtures: 21 runs, mean 2.17 rounds, mean 11.3k prompt / 149 completion tokens per turn
   ```

   Rounds are Completer calls for the turn; the last one is the reply, so
   one tool batch plus the answer is 2. Brackets are the calls that went
   out in one batch; each `→` is a serial round. Tokens are the
   provider's native usage summed over rounds (0 when it omits them).

   Windows: the target is POSIX shell. Set the four values in the
   session and run the `go test` line from the Makefile directly.

## 2. GitHub

The Actions job is a different runner. `eval.yml` on `ubuntu-latest` reads
`LLM_API_KEY` from a **secret** and `LLM_BASE_URL` plus `LLM_MODEL` from
**variables**. Variables show up in the run log. That job does not start
Ollama, does not pull this GGUF, and does not pass `LLM_REASONING_EFFORT`.
The reading in section 1 is the one on this machine.

Wire those three only when the runner can reach the endpoint you name:

**Settings → Secrets and variables → Actions**

- **Secrets** tab → *New repository secret*
  - Name `LLM_API_KEY`, value: the key.
- **Variables** tab → *New repository variable*
  - Name `LLM_BASE_URL`, value: the endpoint.
  - Name `LLM_MODEL`, value: the model id.

The hosted gate is OpenRouter, serving the same 30B-A3B model. With the `gh`
CLI from the repo (the key is from openrouter.ai → Keys):

```sh
gh secret set LLM_API_KEY
gh variable set LLM_BASE_URL --body 'https://openrouter.ai/api/v1'
gh variable set LLM_MODEL --body 'qwen/qwen3-coder-30b-a3b-instruct'
```

That is the same weights at a provider's quantization and chat template, not
Ollama's `q4_K_M` build and `qwen3-coder` renderer, so a CI green is not a
reading on this machine. At $0.07/M prompt tokens a default run (3 runs of
each fixture, about 24k prompt tokens a turn) costs a few cents.

`http://127.0.0.1:11434/v1` in that variable is the runner's loopback, so
the local build is graded only on a self-hosted runner that already has
Ollama and the pulled weights. Two workflows already read the three values:

- **`eval.yml`** — *Actions → eval → Run workflow*. Inputs: `runs` (default
  3) and `only` (a fixture name, blank = all). By hand:

  ```sh
  gh workflow run eval.yml -f runs=5 -f only=scoop_at_2
  ```

- **`release.yml`** — on a `v*` tag push the `eval` job runs first and
  GoReleaser `needs` it. Eval fails → no release. Fix the fixture or the
  persona, then re-run the failed job from the Actions tab (or push the
  next tag).

Without the secret the eval job skips and stays green, so a fork with no key
still releases. GitHub does not hand secrets to fork pull requests, and
`ci.yml` does not call the eval — the free goldens are the PR gate; this is
the release gate.

## 3. Reading a failure

Every failed run prints what the model did, then the reply:

```text
run 2/3 FAIL: expected tool cron_schedule; waiting_for_reply=false, want true
--- tool calls ---
google__calendar_list_events {}
--- waiting=false silent=false jobs=none ---
--- reply ---
Sprint's at 2:30 and the scoop is at 2 — I'll keep an eye on it.
```

| Failure line | Means |
| --- | --- |
| `expected tool X` | The recorder never saw `X`. A call the agent **blocked** (prefix off, never `mcp_enable`d) also never reaches the recorder — that is a real miss, not a harness gap. |
| `expected tool X with args /re/` | `X` was called, but no call's JSON args matched. |
| `tool X called N times, max M` | Spend discipline on a metered API. Two `offers_search` for two possible Fridays is fine; a third byte-identical to the first is the model re-asking a question it already had answered. |
| `invented price $N (in no tool result)` | A figure in the reply that no tool returned. Seen when two searches came back identical and the model made up a cheaper pair to tell the dates apart. |
| `fixture tool X is not in the live S catalog (have …)` | Drift: the sibling's latest release no longer publishes that name. Fails before any model call. |
| `order: X before …` | Both called, wrong order (e.g. the tool before `mcp_enable`). |
| `reply should match /re/` · `should not match` | Shape of the reply — a `?`, a bare “got it”. Never a sentence. |
| `N questions, max M` | Counted `?` in the reply. |
| `waiting_for_reply=false, want true` | The model asked but did not put `[wait]` on its own line — the kernel follow-up rule. |
| `silent=true, want false` | `[silent]` (or an empty reply with no reaction) where a nudge was owed. |
| `expected a reaction, got none` · `reaction X, want Y` | No `[react …]` in the reply, or the wrong emoji (`react: "*"` is any). |
| `want a reaction and no text` | `react_only`: the model reacted and also wrote — a “you're welcome” paragraph beside the 👍. |
| `N model rounds, want the kernel to settle it` | `no_model_call`: an idle 👍 reached the model — the kernel triage should have recorded it for free. |
| `expected memory row kind subject` | No live `memory_store` row with that subject after the turn. |
| `no cron job A–B min out (have …)` | Nothing on the board in the window; `followup`-kind wait pokes do not count, only the reminder. |
| `none of any_of held — alt 1: … \| alt 2: …` | Every alternative failed; each is listed with its own reason. |

A rule that holds two runs in three is a rule the persona is not carrying —
that is why every run must pass.

One line is **not** a failure. `run 2/3 ok (over budget: 4 rounds, budget
3)` means the turn passed and took more Completer rounds than the
fixture's `round_budget`. The gate is on missing work; a model that does
something extra — searches two dates because “next Friday” was ambiguous,
stores a fact it noticed — is not wrong, so the budget is reported beside
the pass and summed at the end (`… 1 over round budget`), never failed.
The batches on that line show where the extra round went. Read it as a
trend across runs: a fixture that is always over budget has a sentence in
the seed costing a round for nothing (see the bake-off below); one run in
five is the model being thorough.

## 4. Tuning

Two kinds of red:

- **The model is right and the fixture is wrong.** It scheduled the scoop
  at +118 minutes and the window was 120–140; it said “buzz you” and the
  regex only knew “ping”. Fix the JSON. Widen the window, add the word to
  the regex, add an `any_of` alternative. The fixture describes the rule,
  and rules have more than one right shape.
- **The persona lost a rule.** It said “nothing today” and stopped; it
  narrated a reminder and set no cron; it asked three questions. Do not
  loosen the fixture. Put the line back in
  `examples/persona/PERSONA.example.md`, run
  `-eval.only` on that fixture until it holds, then the whole set.

Regexes are Go `regexp` (RE2): `(?i)` for case-insensitive, no lookaround.
Fixtures are validated in `go test ./...` before any model call:

```sh
go test ./internal/agent/ -run 'TestEvalFixtures_WellFormed|TestEvalHarness'
```

### The bake-off

The same run is how a persona edit earns its place. Read the shapes first:
a `→` inside a run that the rule did not need is the thing to cut, and it
usually traces to one sentence — a pin the model can only know after a
store returns, a "check first" for something `[harness]` already stamps.
Then:

```sh
cp examples/persona/PERSONA.example.md /tmp/candidate.md
# edit /tmp/candidate.md
make integration-test EVAL_ARGS='-eval.n=5'                                   # seed: the number to beat
make integration-test EVAL_ARGS='-eval.n=5 -eval.persona=/tmp/candidate.md'   # candidate
```

Compare the two `all fixtures:` lines and the per-fixture means. A
candidate wins when every run still passes and rounds or tokens fall;
n=3 is too noisy to call a 0.2-round difference, use 5 or more. When it
wins, copy it over the seed (`examples/persona/PERSONA.example.md`), and
lower `round_budget` on the fixtures that moved so the next regression
shows up as a run of over-budget notes. A bake-off pass typically buys a
few tenths of a round per turn — a sentence that made the model wait for
an id it did not need, or re-read a line already stamped in `[harness]`.
The round count is a note, not a gate.

What a bake-off is not for: cutting the work. A round worth removing is
one in which the model did nothing new. A candidate that
passes with fewer rounds because it searches less, stores less, or stops
at “let me know when” is a loss, and the completion fixtures (the flight,
the gym check-in, the dinner planner) are there to fail it. Take the
reading at `LLM_REASONING_EFFORT=none`. A bake-off that also changes
that knob grades a different setting.

## 5. Adding a fixture

Copy the nearest file under `internal/agent/testdata/eval/` and edit.

| Field | What |
| --- | --- |
| `name`, `why` | `name` is what `-eval.only` matches; `why` is the rule in one sentence, printed at the top of the run. |
| `inbound` | The human's text. `{{+120m}}` becomes the local clock 120 minutes from now (“2:00PM”) — the eval runs on the real clock because `cron_schedule` does. |
| `planner` | Instead of `inbound`: send the daily planner wake (`DailyPlannerPrefix` + `DefaultDailyPlannerPrompt`). |
| `cron` | Instead of `inbound`: the prompt of a scheduled job the model set. Sent as `cron.JobUserPrefix` + prompt — the shape of a daily “check X” wake. |
| `history` | Prior `user` / `assistant` turns appended to the session first. |
| `waiting` | Arms `waiting_for_reply` before the turn — the last assistant line asked and put `[wait]` on it. For `[reaction] 👍 on: …` inbound: is the 👍 an answer. |
| `memory` | Seed rows: `kind`, `subject`, `content`. `pref/hours` here gives the model `[hours]`. |
| `self` | `SELF.md` body (`- ` bullets). Empty file when absent — that is the “empty SELF.md” scenario. |
| `tools` | Canned MCP tools: `name` (must be `server__name`), `description`, optional `params` schema, `result` returned every call — `{{+90m}}` in a result expands like `inbound`, so a stub dinner is still ahead whenever the eval runs. |
| `tools_from` | Servers whose **real** catalog replaces hand-written defs — see below. `tools` entries for those servers carry only `name` + `result`. |
| `force` | MCP prefixes published without `mcp_enable`. Leave empty to test the off → enable → call path. |
| `expect` | The shape contract — see the failure table above for each key. `round_budget` is the cost note, not a check: the rounds the rule needs (one batch + reply = 2; a prefix that must be `mcp_enable`d first = 3). Over it is printed beside the pass. `tools_called[].max_calls` caps a metered tool (one search, not five). `prices_from_tools` fails any `$N` in the reply that no tool returned. `react` / `react_only` / `no_model_call` are the reaction gates. |

### Real catalogs (`tools_from`)

A hand-written `params` block is the eval's guess at an MCP's interface,
and the guess drifts: the first Denver fixture searched `flights__search`
with `from/to/date`; the shipped tool is `flights__offers_search` with
`origin/destination/outbound_date` and a description that says when to
call `airports_search` first. A fixture proving the model can drive the
guess proves nothing about the binary.

`tools_from: ["rentals"]` publishes the server's real catalog instead.
At run time the harness reads `internal/agent/testdata/eval/mcp.toml`,
pulls each server's **latest GitHub release** with the same code as
`george tools-fetch`, boots it, and takes `tools/list` — names,
descriptions, schemas — as the tool defs. Nothing is checked in: the
manifest is name, binary, release URL, and a placeholder `env` for
binaries that refuse to start without a key present. Nothing is called:
results stay canned, so the placeholder never reaches a vendor. The
catalog is fetched once per run and cached under `EVAL_MCP_BIN` (default
`$TMPDIR/george-eval-mcp`). `GITHUB_TOKEN=` in `.env` (the target sources
it) lifts the API rate limit for the `latest` lookups — three per run
unauthenticated against a 60/hour cap is fine until a tuning afternoon.

Two things fail for free, before any model call: a fixture `tools` entry
whose name the live server does not publish (the release renamed or
dropped it — the message lists what it does have), and a `tools_from`
server missing from the manifest. Add a server by adding a `[[server]]`
to that manifest; Google is there with a two-tool allowlist because it
publishes about a hundred.

`denver_flight`, `rental_daily_cron`, and `edit_then_check` run on real catalogs. The rest are
hand-written where the binary needs a real account to list tools
(Garmin) or the schema is trivial; move them as the siblings allow.

Then:

```sh
go test ./internal/agent/ -run TestEvalFixtures_WellFormed     # parses, regexes compile
make integration-test EVAL_ARGS='-eval.n=1 -eval.only=<name>'   # first live read
```

Four kinds of fixture live in the directory. The first seven are
single-batch rules — one tool round and a reply — and their
`round_budget` is 2 or 3. `denver_flight`, `hows_gym_going`,
`planner_weight_dinner` are **completion** fixtures: the check is that the
legwork happened — the search called with real options back, Garmin read
before a progress answer, the nudge tied to the dinner on the calendar —
and the model may take the rounds it needs to get there. Write new
fixtures in that shape when the rule is “finish the task”: assert the
tool, the row, the question and `[wait]`, forbid “let me know when you
want me to look”, and leave the round count to the budget note.
`rental_daily_cron` (and Denver again) are **spend** fixtures on real
catalogs: a metered API, `max_calls` on the search, `tools_not_called` on
the per-item detail calls nobody asked for, `prices_from_tools` on the
reply. `12`–`15` are **reaction** fixtures: the agent's `[react 👍]` where
a sentence would be noise, and the human's 👍 / 👎 on the agent's message
— free when idle, an answer when the agent was waiting, never nothing when
negative. The one table row still without
a fixture (`self_note` on a landed joke) needs an `expect` that reads
`SELF.md` — a small addition to `eval_harness_test.go` and one JSON file.
