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
- [x] Config in `~/.config/george/`, data in `~/.local/share/george/`. `george init` writes `env`, `mcp.toml`, `PERSONA.md`, and `SELF.md` there; `tools-fetch` installs into `~/.local/share/george/bin`, which george puts first on `PATH`. `deploy/` is gone. See [gap 1](#8-gaps).

## 2. Pin the model the eval grades

Write one id, in one place, that the live eval reads. Include the size and the quant. Example shape, not a decision: `qwen2.5-coder:32b` at `Q4_K_M`, or a GGUF repo plus quant such as `bartowski/Llama-3.2-1B-Instruct-GGUF:Q4_K_M`. Pick the model you will actually run.

- [x] Record that id in the eval env (the same three values gantry already uses: base URL, model, key). The model string is the full id. `qwen3-coder:30b-a3b-q4_K_M` in `.env.example`.
- [x] ~~Refuse to start the live eval when the model string is a family tag.~~ Dropped: george can't know every model name, and picking one is the user's call. Instead, a model the server doesn't have fails with `model "bad_name" was not found at LLM_BASE_URL; set LLM_MODEL to one the server has (Ollama: ollama list)`. See [gap 2](#8-gaps).
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
- [ ] `make integration-test` stays the paid gate: tools, arguments, call counts, memory rows. It checks shape, not the sentence. Every run passes, or the rule is not carried. The gate exists; not every run passes yet. See [gap 3](#8-gaps).
- [ ] Add one fixture whose only job is a mangled MCP name on the pinned model: dot instead of the separator, a hyphen in the server prefix, and a near-miss. The child that runs is the real tool. A pasted XML string in a unit test does not satisfy this. The host side is done and unit-tested; the live fixture is still open. See [gap 4](#8-gaps).
- [x] Keep the repair on the session model: prefix alias, at most five closest names, one grammar-constrained retry, salvage a call printed as JSON, a landing call when the iteration budget runs out. Count them. `/toolstats` prints them.
- [ ] Confirm `LLM_SYSTEM_FOLD` against this model’s chat template before you trust a red or a green. The open risk is a template that renders system text only at position 0. If the model cannot see the harness block, tool-call failures are a prompt-placement bug. See [gap 5](#8-gaps).

## 4. Keep the loop small enough for that model

- [x] Last two tool rounds stay whole, including a parallel batch. Older rounds become a one-line marker in-process. No completion spent to describe a tool pair.
- [x] Schemas stay off until that prefix is enabled. The prefix list itself stays a stable line.
- [x] Fold stays one completion. No second “store everything now” turn.
- [x] Quiet watches poll a tool and do not call the model. Moot: the clock and its watches are gone.
- [x] Memory stays SQLite you can open with `sqlite3`. FTS is fine. No embedding call, no vector column, no vector database.
- [x] Auto-save of every turn stays off. A row lands because the model stored it, or because a consolidator promoted an episode that was stored on purpose. The consolidator is gone too.

## 5. Cut what grew up on Gemini

Do this after the fork boots on stdio, before adding features. The planner and the mouths are why a local model stopped being the daily driver. The full cut list, phase order, and coding eval set are [coding-agent-plan.md](coding-agent-plan.md).

- [x] Remove Telegram, Discord, and Slack from the default path. Delete them once stdio is the product, or leave them behind a build tag you do not ship. Deleted. `config.CheckRetired` refuses their env vars. A few comments still name Telegram; see [gap 6](#8-gaps).
- [x] Remove the daily planner note (the long policy string, measured near 460 words). Small models drop the middle. Goldens will stay green while the model ignores a clause. That is the failure mode. The session is a short pull-and-schedule note. The aims ladder, the todo essay, and the room redress are out of `DefaultDailyPlannerPrompt` and `plannerToolFirstNote`.
- [ ] Remove the aims ledger, the todo-as-memory rows, and the pendant room frames (`[room]`, avatar, backdrop, theme) from the default prompt. They are the crane’s household. They are not the CLI runner. Still open in `internal/persona/contract.md` (`[aims]`, `[todo]`, `todo/dentist`), `internal/session/summary.go` (aims), and `internal/mcp/host.go` (`avatar_get`). See [gap 7](#8-gaps).
- [ ] Drop Gemini thought-signature handling unless that provider is still on the socket. One socket, the local one. Still in `internal/provider`, and `.env.example` still offers a Gemini model. See [gap 8](#8-gaps).
- [x] Re-read every fixture that was green on Flash. Retarget it at the pinned model. A miss is a sentence to shorten, not an expectation to loosen. The Flash fixtures were deleted with the assistant; the coding set (`27`–`30`) was written against the pinned model.

## 6. Do not bring these over from goose

Each one is an extra completion, a second model, or a vague model id.

- [x] No tool shim and no interpreter model (`mistral-nemo`, `qwen2.5:3b`, or a local second GGUF).
- [x] No session-title completion on the first turns.
- [ ] No auto-compaction that asks the model to summarize the history. `session.LLMSummarizer` does this when history is trimmed. See [gap 9](#8-gaps).
- [x] No per-tool-pair summarizer.
- [x] No Smart Approve judge call, and no adversary-inspector completion.
- [x] No skills directory, no subagents, no inbound port, no control UI.
- [x] No name repair that only accepts an exact rewrite of `extension__tool` and then pastes the full tool list back into the next prompt.
- [ ] No live test that skips unless a host env var is set, and no CI that never sets it. The golden stays on every push. The live eval is the release gate, and it names the model. Still open: the eval skips without `LLM_*`, and `eval.yml` goes green without a key. See [gap 10](#8-gaps).

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

## 8. Gaps

Each one comes from the code or a live run on `qwen3-coder:30b-a3b-q4_K_M`. A gap is closed when its check passes. Writing the code is not enough.

Run the live gate like this: `make integration-test EVAL_ARGS='-eval.n=5'`. Last reading: `edit_then_check` 5/5, `parallel_reads` 5/5, `parallel_checks` 2/5, `parallel_patches` 2/5.

1. **Closed: george runs in any repo.** Done in two steps:
   - **Docker out:** `Dockerfile`, `compose.yml`, `.dockerignore`, the `docker-*` targets, `docs/deploy-docker.md`, and `docs/dockerhub.md` are deleted.
   - **Root from the cwd:** `cmd/george/root.go` sets `GEORGE_ROOT` to the git toplevel of the cwd (or the cwd outside a repo) unless it's already set, and server `args` expand `${VAR}`.
   - **XDG paths** (`internal/config/paths.go`, using `adrg/xdg`): the config dir is `GEORGE_CONFIG_DIR` or `~/.config/george`. `PERSONA_DIR` defaults to it, `MCP_MANIFEST` to its `mcp.toml`, and `DATA_DIR` to `~/.local/share/george`.
   - **Env file:** `config.Load` reads `~/.config/george/env` with `godotenv` first. The process env wins, and a retired variable in the file still fails boot.
   - **init and tools-fetch:** `george init` writes `env` (0600), `mcp.toml`, `PERSONA.md`, and `SELF.md` there. `george tools-fetch` defaults to `~/.local/share/george/bin`, and `run` puts that dir first on `PATH`.
   - **Removed:** `deploy/` and the Makefile's `PERSONA_DIR` override. `make run` is now the same as `george`.
   - **Checked:** unit tests for the root, `PATH`, defaults, and env file (fills unset, loses to the process env, refuses `CHANNEL`, names a bad file), and init's mode and targets. Then a real run with `env -i PATH=/usr/bin:/bin HOME=<fresh>`: `george init`, MCP binaries placed in the bin dir, then `george` in a non-repo directory read `greet.txt` there, and `george` in `internal/config` of this repo rooted at the toplevel and answered from a real `git__status_get`.
   - **Docs:** `docs/architecture.md`, `docs/design.md`, and `docs/deploy-native.md` describe the CLI. Your own `.env` (the eval's) still sets `CHANNEL`. Delete that line.
2. **Closed: a wrong model name says so.** Refusing family tags was dropped; which model to run is the user's call. A 404 that names the model now reads `model "bad_name" was not found at LLM_BASE_URL; set LLM_MODEL to one the server has (Ollama: ollama list)`, followed by the raw response. A 404 that doesn't name the model (a wrong base URL path) stays raw. Checked by `TestClient_ModelNotFound`, `TestClient_NotFoundWithoutModelStaysRaw`, and a live `LLM_MODEL=bad_name make run` against Ollama.
3. **The model doesn't stop when the task is done.** This causes most live failures: it commits without being asked (`git__commit_create`), writes self-notes and memory nobody asked for, runs a patch nobody asked for, and repeats a finished chain up to the round cap. There are three causes, and each needs its own fix:
   - **The contract never says what "done" is.** Add a definition of done to `contract.md` and keep it concrete. "Least change, DRY, KISS, TDD" are good taste but abstract, and this model ignores abstract rules or copies them back as text (two abstract persona rewrites scored 0/10). Turn each one into an action it can check:
     - Least change: patch only the lines the task needs, and don't reformat or rename around them.
     - KISS and DRY: reuse what the repo already has before writing new code.
     - TDD: when the task adds behaviour, add or change a test first and run it.
     - Done: the asked change is made, and the repo's test and lint commands pass. Then reply in one or two lines and stop.
     - Never: commit, push, `memory_store`, or `self_note` unless the human asked for it this turn.
   - **The contract still pulls toward memory.** The assistant-era Memory hygiene and todo-capture text tells it to store things (gap 7). Cut that before adding the done block, or the two fight.
   - **The harness lets a loop run.** The fake tools don't show the patch (gap 11), and nothing stops a repeated call (gap 12).
   - Order: add the `commit_only_when_asked` and `stop_when_done` fixtures first, so there's a number to move (`stop_when_done` is a task answered in one round where any further call fails the run). Then cut gap 7. Then add the done block one bullet at a time under `-eval.n=5`, and keep a bullet only if the rate goes up.
   - Check: every fixture at 5/5.
4. **Mangled names: host done, live fixture open.** `Host.resolve` now also repairs a real prefix joined by the wrong separator (`fs.file_get`, `git_status_get`, `shell-command_run`), and counts it as `prefix_alias`. `TestHost_MangledCodingToolNames` covers that plus a bare name (`file_patch`), an invented prefix (`workspace__file_list`), an OpenAI-style `functions.fs__file_get`, and a near miss (`fs__get_file` gets back `fs__file_get` as the closest name). Still open: the live fixture. The eval's canned tools don't go through `Host.resolve`, so the fixture waits on gap 11 (a real host and real `fs` in the eval). Then give it an inbound that names a tool the wrong way ("run fs.file_get on a.go") and expect the real `fs__file_get` to run. Check: 5/5, and `/toolstats` shows the repair.
5. **`LLM_SYSTEM_FOLD` not checked against the qwen template.** `auto` only folds for `gemini*`. Render one request through Ollama's `/api/chat` template for this model (or read the Modelfile `TEMPLATE`) and confirm that system text placed after position 0 reaches the model. If it doesn't, set `fold` for this model. Check: a short note here naming the template line that was read.
6. **Stale assistant comments.** Telegram is named in comments in `agent.go`, `spinup_notes.go`, `provider/stream.go`, and `channel/stream.go`, and `readme.md`'s note that the paths are still repo-relative goes away when gap 1 lands. Reword them when gaps 1 and 7 land. Check: `rg -i telegram --type go -g '!*_test.go'` shows only `config.CheckRetired`.
7. **Household still in the contract.** `contract.md` teaches `[aims]`, `[todo]`, `[hours]`, `todo/`, `follow/`, and a dentist example; `summary.go` mentions aims; `host.go` has the `avatar_get` path. Cut each one as its own change under the live gate. Then add the coding memory lines: repo versus you, and repo files first. Check: the gate rate holds or rises after every cut.
8. **Gemini still on the socket.** Thought-signature echo and stream synthesis live in `internal/provider`, and `.env.example` offers `gemini-3.5-flash`. Decide whether Gemini stays as a dev option. If it doesn't, delete the code, its tests, and the `.env.example` lines. Check: `rg thought_signature` comes back empty.
9. **History is still compacted by the model.** `session.LLMSummarizer` spends a completion to fold trimmed turns into a summary, which section 6 forbids. Either drop it (trimmed turns are simply gone, and the `bound.go` markers already cover tool rounds) or write down why this one completion is allowed and strike the section 6 line. Plan phase 10 (context budget) is the natural place. Check: a long session in the process log shows no summary completion, or the decision is recorded here.
10. **The release gate can be skipped.** `TestEval_Live` skips without `LLM_*`, and `eval.yml` goes green on a fork with no key. Make the release workflow fail when the eval skips. A local `make integration-test` with no `.env` should also fail with a message, not skip. Check: `release.yml` on a repo with no key goes red.
11. **Canned tools don't react.** `fs__file_get` returns its script whatever was patched, so the model re-reads, sees no change, and patches again (19 and 26 round loops). Run the patch fixtures against the real `fs` server on a temporary git repo seeded from the fixture. Check: `parallel_patches` stops re-reading after the patch, and a malformed diff fails the run.
12. **No loop guard under the higher cap.** `TOOL_MAX_ITERATIONS` is 25, so a loop now costs about 130k tokens before it's stopped. End the turn with an honest reply when the model makes the same call (same name, same arguments) a second time after the result came back. Check: a unit test, plus the 26-round `edit_then_check` loop ends by round 9.
13. **An empty final reply after a tool.** One live run ended with an empty reply after an unasked `memory_store`. Fall back to the last real narration, or to a short "done; nothing to add" line. Check: a unit test where the last completion is empty.
14. **Rest of the plan.** Memory scope plus repo id plus memory move (phase 7). Readline, bracketed paste, and Ctrl-C cancelling the turn (phase 8). Context budget (phase 10). `docs/architecture.md` still describes the assistant (phase 12). Each phase has its own gate in [coding-agent-plan.md](coding-agent-plan.md).
15. **The last two "done when" lines.** No long live session has been run to show the older-round markers working with no extra completion. Nobody has opened the memory file with `sqlite3` on this fork. Run both by hand once gaps 9 and 14 (phase 7) land, and tick them.
