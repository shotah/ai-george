# george — work after the fork

george is the CLI cut of [ai-gantry](https://github.com/shotah/ai-gantry). One process, one local model, MCP tools, stdin/stdout. The binary is `george`.

The job is a local agent you can grade. A green run on Gemini does not count. A family tag (`qwen3`, `qwen2.5`, `qwen3-coder:latest`) does not count. The eval names one model with a size and a quant, and every run on that model has to pass.

This file is the work list. The other notes beside it are the goose-side evidence (`fork-cli-agent.md`, `ollama-and-llama.md`, `extra-model-calls.md`, `storage.md`). They are not the design of george.

goose stays where it is. Do not port its provider matrix, recipes, desktop, or second agent loop.

## 1. Fork and rename

- [x] Fork `ai-gantry` into the new repo. Module, binary, module path, and user-facing strings become `george`.
- [x] Leave the gantry name on the crane. Pendant, Cab, and gantree stay their own repos. george does not import them.
- [x] Default channel is stdio. `george` with no `CHANNEL` set talks on stdin/stdout. A missing channel must not come up as Telegram.
- [x] Not Docker-based. george is a CLI you run in a repo, not a long-lived container. The container files, the `docker-*` targets, and the Docker docs are gone. The root comes from the cwd: george sets `GEORGE_ROOT` to the git toplevel, and `mcp.toml` passes `--root ${GEORGE_ROOT}`. Detail: [coding-agent-plan.md](coding-agent-plan.md#where-it-lives-no-docker).
- [x] Config in `~/.config/george/`, data in `~/.local/share/george/`. `george init` writes `env`, `mcp.toml`, `PERSONA.md`, and `SELF.md` there; `tools-fetch` installs into `~/.local/share/george/bin`, which george puts first on `PATH`. `deploy/` is gone. See [the work list](#8-work-list).

## 2. Pin the model the eval grades

Write one id, in one place, that the live eval reads. Include the size and the quant. Example shape, not a decision: `qwen2.5-coder:32b` at `Q4_K_M`, or a GGUF repo plus quant such as `bartowski/Llama-3.2-1B-Instruct-GGUF:Q4_K_M`. Pick the model you will actually run.

- [x] Record that id in the eval env (the same three values gantry already uses: base URL, model, key). The model string is the full id. `qwen3-coder:30b-a3b-q4_K_M` in `.env.example`.
- [x] ~~Refuse to start the live eval when the model string is a family tag.~~ Dropped: george can't know every model name, and picking one is the user's call. Instead, a model the server doesn't have fails with `model "bad_name" was not found at LLM_BASE_URL; set LLM_MODEL to one the server has (Ollama: ollama list)`. See [the work list](#8-work-list).
- [x] Print the id at the top of every live run, next to the pass line, so a green log says which weights were graded.
- [x] First live reading is that local id. A Flash log is unread until this one passes.

goose’s published local claims, for the record you are walking away from:

| Place | String they wrote |
| --- | --- |
| Known issues, “works with extensions” | `qwen2.5` |
| Ollama setup example | `ollama run qwen2.5` |
| LM Studio example | `qwen2.5-7b-instruct` |
| Custom distro default | `qwen3-coder:latest` |
| Code default `OLLAMA_DEFAULT_MODEL` | `qwen3` |
| Known-name list | `qwen3`, `qwen3-vl`, `qwen3-coder:30b`, `qwen3-coder:480b-cloud` |
| Skipped live test | `qwen3`, `qwen3-vl` |
| XML-parser comment, models that drop native tool calls | `qwen3-coder`, `qwen3-coder-32b` |

Those strings do not name one model. The March 2025 goose benchmark did (`qwen2.5-coder:32b` Q4_K_M and three siblings). That post is not a test that still runs.

## 3. Grade tool calls on that model

Keep gantry’s two contracts.

- [x] Goldens stay free and on every push: the bytes from stdin JSON through the completer request to the HTTP body.
- [ ] `make integration-test` stays the paid gate: tools, arguments, call counts, memory rows. It checks shape, not the sentence. Every run passes, or the rule is not carried. The gate exists; not every run passes yet. See *Teach it what "done" is* in [the work list](#8-work-list).
- [ ] Add one fixture whose only job is a mangled MCP name on the pinned model: dot instead of the separator, a hyphen in the server prefix, and a near-miss. The child that runs is the real tool. A pasted XML string in a unit test does not satisfy this. The host side is done and unit-tested; the live fixture is still open. See *Mangled-name live fixture* in [the work list](#8-work-list).
- [x] Keep the repair on the session model: prefix alias, at most five closest names, one grammar-constrained retry, salvage a call printed as JSON, a landing call when the iteration budget runs out. Count them. `/toolstats` prints them.
- [x] Confirm `LLM_SYSTEM_FOLD` against this model’s chat template before you trust a red or a green. The open risk is a template that renders system text only at position 0. If the model cannot see the harness block, tool-call failures are a prompt-placement bug. It was: the renderer drops later system messages, so `auto` now folds. See *Fold vs. the qwen template* in [the work list](#8-work-list).

## 4. Keep the loop small enough for that model

- [x] Last two tool rounds stay whole, including a parallel batch. Older rounds become a one-line marker in-process. No completion spent to describe a tool pair.
- [x] Schemas stay off until that prefix is enabled. The prefix list itself stays a stable line.
- [x] Fold stays one completion. No second “store everything now” turn.
- [x] Quiet watches poll a tool and do not call the model. Moot: the clock and its watches are gone.
- [x] Memory stays SQLite you can open with `sqlite3`. FTS is fine. No embedding call, no vector column, no vector database.
- [x] Auto-save of every turn stays off. A row lands because the model stored it, or because a consolidator promoted an episode that was stored on purpose. The consolidator is gone too.

## 5. Cut what grew up on Gemini

Do this after the fork boots on stdio, before adding features. The planner and the mouths are why a local model stopped being the daily driver. The full cut list, phase order, and coding eval set are [coding-agent-plan.md](coding-agent-plan.md).

- [x] Remove Telegram, Discord, and Slack from the default path. Delete them once stdio is the product, or leave them behind a build tag you do not ship. Deleted. `config.CheckRetired` refuses their env vars. A few comments still name Telegram; see *Stale Telegram comments* in [the work list](#8-work-list).
- [x] Remove the daily planner note (the long policy string, measured near 460 words). Small models drop the middle. Goldens will stay green while the model ignores a clause. That is the failure mode. The session is a short pull-and-schedule note. The aims ladder, the todo essay, and the room redress are out of `DefaultDailyPlannerPrompt` and `plannerToolFirstNote`.
- [ ] Remove the aims ledger, the todo-as-memory rows, and the pendant room frames (`[room]`, avatar, backdrop, theme) from the default prompt. They are the crane’s household. They are not the CLI runner. Still open in `internal/persona/contract.md` (`[aims]`, `[todo]`, `todo/dentist`), `internal/session/summary.go` (aims), and `internal/mcp/host.go` (`avatar_get`). See *Cut the household from the contract* in [the work list](#8-work-list).
- [x] Drop Gemini thought-signature handling unless that provider is still on the socket. One socket, the local one. Dropped: Gemini isn't for this repo.
- [x] Re-read every fixture that was green on Flash. Retarget it at the pinned model. A miss is a sentence to shorten, not an expectation to loosen. The Flash fixtures were deleted with the assistant; the coding set (`27`–`30`) was written against the pinned model.

## 6. Do not bring these over from goose

Each one is an extra completion, a second model, or a vague model id.

- [x] No tool shim and no interpreter model (`mistral-nemo`, `qwen2.5:3b`, or a local second GGUF).
- [x] No session-title completion on the first turns.
- [ ] No auto-compaction that asks the model to summarize the history. `session.LLMSummarizer` does this when history is trimmed. See *The summarizer decision* in [the work list](#8-work-list).
- [x] No per-tool-pair summarizer.
- [x] No Smart Approve judge call, and no adversary-inspector completion.
- [x] No skills directory, no subagents, no inbound port, no control UI.
- [x] No name repair that only accepts an exact rewrite of `extension__tool` and then pastes the full tool list back into the next prompt.
- [x] No live test that skips unless a host env var is set, and no CI that never sets it. The golden stays on every push. The live eval is the release gate, and it names the model. `TestEval_Live` fails without `LLM_*`, so `eval.yml` and the release go red without a model.

## Done when

- [x] `george` on a fresh tree talks stdio and calls the pinned local model.
- [x] The live log prints that full model id.
- [ ] The mangled-name fixture passes on that model, every run.
- [ ] A long tool session still answers after older rounds have been marked down, and the process log shows no extra completion for those markers.
- [ ] `sqlite3` on the memory file shows rows, and the repo has no embedding dependency.

## 7. Coding tools

Workspace read, patch, git, and shell are MCP binaries. The plan, the server ids, and the tool names are [coding-mcp.md](coding-mcp.md). Nouns are reserved in [mcp-naming.md](mcp-naming.md).

New repos (`fs-mcp`, `git-mcp`, `shell-mcp`, `github-mcp`). They are not features of this process, and they are not patches to the other plugins under `repos/`.

Done: `fs`, `git`, `shell`, and `github` are in `deploy/mcp.toml`. The live eval fetches the real releases and checks the coding catalog before any fixture runs.

## 8. Work list

Each item comes from the code or a live run on `qwen3-coder:30b-a3b-q4_K_M`. An item is done when its check passes. Writing the code is not enough. The quick ones are first; the big ones are at the bottom, in priority order.

Live gate: `make integration-test EVAL_ARGS='-eval.n=5'`. Last full reading, after the fold and the loop guard: `edit_then_check` 4/5, `parallel_reads` 4/5, `parallel_checks` 5/5, `parallel_patches` 3/5. The one before it, with later system messages dropped by the renderer: 5/5, 5/5, 2/5, 2/5.

### Quick wins

- [x] **Stale Telegram comments.** Comments in `agent.go`, `spinup_notes.go`, `provider/stream.go`, and `channel/stream.go` still name Telegram, and `websearch` still special-cases the retired `mcp-gemini-google-search` server. Reword or delete them. Check: `rg -i 'telegram|google-search' --type go -g '!*_test.go'` shows only `config.CheckRetired`.
- [x] **Empty final reply after a tool.** One live run ended with an empty reply after an unasked `memory_store`. Fall back to the last real narration, or to a short "Done." line. Check: a unit test where the last completion is empty. `TestAgent_EmptyFinalAfterTools` covers blank and tag-only finals; a stray `<tool_call>` is stripped inside the loop, so it falls back too.
- [x] **Repeat-call loop guard.** `TOOL_MAX_ITERATIONS` is 25, so a loop costs about 130k tokens before it's stopped. End the turn with an honest reply when the model makes the same call (same name, same arguments) a second time after the result came back. Check: a unit test, plus the 26-round `edit_then_check` loop ends by round 9. A single repeated call is too strict: test, fix, re-test repeats `shell__command_run`. The guard (`loopguard.go`) fires when the last k rounds repeat the k before them, with the same calls, arguments, and results; the next call is then the landing call with a loop note. A re-run whose result changed is progress. Live: it fired three times in one gate run; the longest `edit_then_check` run was 8 rounds. Not caught: a loop whose arguments keep changing (one `parallel_patches` run searched with new queries until round 24).
- [x] **Fold vs. the qwen template.** `LLM_SYSTEM_FOLD=auto` kept the agent layout, with system text after the user turn. The template is only `{{ .Prompt }}`; the modelfile says `RENDERER qwen3-coder` (Ollama 0.35.0), so it's rendered in code. A probe settled it: a codeword in a second system message after the user turn changed neither the prompt token count (47 both ways) nor the reply (`NONE`), while the same line in the first system message was read. Only the first system message renders. `auto` now folds like `one`; `many` keeps the old layout. Every gate reading before this ran without the budget warning and the landing note. The wire golden is `testdata/stdio/wire.txt`.
- [x] **The release gate can't be skipped.** `TestEval_Live` skipped without `LLM_*`, and `eval.yml` went green on a fork with no key. It now fails with a message, so the eval job and `release.yml` go red. Checked: `make integration-test` in a copy with no `.env` and no `LLM_*` exits non-zero with that message. Not checked on GitHub.

### Medium

- [ ] **Fixtures for stopping.** Add `commit_only_when_asked` (the task edits a file; `git__commit_create` fails the run) and `stop_when_done` (the task is answered in one round; any further call fails the run). These give the next two items a number to move. Check: both run at `-eval.n=5` and a baseline is recorded here.
- [ ] **Cut the household from the contract.** `contract.md` teaches `[aims]`, `[todo]`, `[hours]`, `todo/`, `follow/`, a dentist example, and Memory hygiene that tells it to store things. `summary.go` mentions aims, and `host.go` has the `avatar_get` path. Cut one piece per change, each under the live gate, then add the coding memory lines (repo versus you, repo files first). Check: the gate rate holds or rises after every cut.
- [ ] **Teach it what "done" is.** This model ignores abstract rules ("least change, DRY, KISS, TDD") or copies them back as text; two abstract persona rewrites scored 0/10. Add concrete lines to `contract.md`, one at a time, keeping each only if the rate goes up:
  - Least change: patch only the lines the task needs, and don't reformat or rename around them.
  - KISS and DRY: reuse what the repo already has before writing new code.
  - TDD: when the task adds behaviour, add or change a test first and run it.
  - Done: the asked change is made, and the repo's test and lint commands pass. Then reply in one or two lines and stop.
  - Never: commit, push, `memory_store`, or `self_note` unless the human asked for it this turn.
  - Check: every fixture at 5/5.
- [ ] **The summarizer decision.** `session.LLMSummarizer` spends a completion to fold trimmed turns into a summary, which section 6 forbids. Either drop it (trimmed turns are gone, and the `bound.go` markers already cover tool rounds) or record why this one completion is allowed and strike the section 6 line. Check: a long session's log shows no summary completion, or the decision is written here.

### Big, in priority order

- [ ] **1. One session and one memory per repo (plan phase 7).** george now runs in any repo, but every repo still shares one session and one memory pool, so repo A's conversation and facts leak into repo B. Add the repo id (from the root), make it the session id, add a scope column to memory (repo or you), and add the memory move. Check: `george` in two repos resumes two different conversations, and a repo fact stored in A is not recalled in B.
- [ ] **2. Real `fs` in the eval.** The canned `fs__file_get` returns its script whatever was patched, so the model re-reads, sees no change, and patches again (19- and 26-round loops). Run the patch fixtures against the real `fs` server and host on a temporary repo seeded from the fixture. Check: `parallel_patches` stops re-reading after the patch, and a malformed diff fails the run.
- [ ] **3. Mangled-name live fixture.** Needs item 2, because the canned tools don't go through `Host.resolve`. The host side is done (see below). Give the fixture an inbound that names a tool the wrong way ("run fs.file_get on a.go") and expect the real `fs__file_get` to run. Check: 5/5, and `/toolstats` shows the repair.
- [ ] **4. Input on stdio (plan phase 8).** A line editor (import one, such as `chzyer/readline`) with bracketed paste, so a paste is one message. Ctrl-C cancels the turn, not the process. Delete `coalesce.go`. Check: a 40-line paste is one turn, and Ctrl-C mid-turn returns to the prompt.
- [ ] **5. Context budget (plan phase 10).** Fit history, contract, schemas, and tool results into the model's window, together with the summarizer decision above. Check: a long session still answers after older rounds are marked down, with no extra completion in the log.
- [ ] **6. Docs (plan phase 12).** Finish the CLI rewrite of `docs/architecture.md`, `docs/design.md`, and `docs/deploy-native.md`, and drop the assistant-era docs that no longer describe george. Check: no doc tells you to run a daemon, a container, or a chat channel.
- [ ] **7. The last two "done when" lines.** Run a long live session to show the older-round markers working with no extra completion, and open the memory file with `sqlite3`. Run both by hand after items 1 and 5, then tick them above.

### Done

- [x] **george runs in any repo.** The container files and `deploy/` are gone. `cmd/george/root.go` sets `GEORGE_ROOT` to the git toplevel of the cwd (or the cwd) unless set, and server `args` expand `${VAR}`. `internal/config/paths.go` (`adrg/xdg`) puts config in `GEORGE_CONFIG_DIR` or `~/.config/george` and data in `~/.local/share/george`. `config.Load` reads `~/.config/george/env` with `godotenv`; the process env wins and a retired variable still fails boot. `george init` writes `env` (0600), `mcp.toml`, `PERSONA.md`, and `SELF.md`; `tools-fetch` installs into `~/.local/share/george/bin`, which george puts first on `PATH`. Checked by unit tests and a real run in a fresh `HOME` from a non-repo dir and from a repo subdirectory.
- [x] **A wrong model name says so.** Refusing family tags was dropped; which model to run is the user's call. A 404 that names the model reads `model "bad_name" was not found at LLM_BASE_URL; set LLM_MODEL to one the server has (Ollama: ollama list)`. Checked by `TestClient_ModelNotFound`, `TestClient_NotFoundWithoutModelStaysRaw`, and a live run.
- [x] **Mangled names in the host.** `Host.resolve` repairs a wrong separator (`fs.file_get`, `git_status_get`, `shell-command_run`) and counts it as `prefix_alias`. `TestHost_MangledCodingToolNames` covers that plus a bare name, an invented prefix, `functions.fs__file_get`, and a near miss.
- [x] **Gemini is gone.** Not for this repo. `ToolCall.Raw`, the thought-signature echo, and the skip token are deleted; tool calls are re-encoded from id, name, and arguments. `LLM_SYSTEM_FOLD=auto` no longer looks at the model name (`one` stays for chat templates that need it). The env templates no longer offer Gemini. Checked: `rg thought_signature` is empty outside the docs, the request goldens are unchanged, and a live run completed multi-round tool turns.
