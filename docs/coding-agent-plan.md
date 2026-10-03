# george as a coding agent

This is the plan for turning the forked assistant into a local coding agent. [todo.md](todo.md) is still the list for the fork itself: pin the model, keep the contracts, stay away from goose. This file covers the assistant features that come out, the parts of the prompt that change, and the evals that have to exist before the job is done. The tools are in [coding-mcp.md](coding-mcp.md).

## The product

The product is a CLI. You run `george` in a repo, it works in that tree, and it exits when you quit. It is not a daemon and not a container. It uses one local model, `qwen3.6:35b-a3b-coding` from [eval_setup.md](eval_setup.md), and four MCP children rooted at the repo: `fs`, `git`, `shell`, and `github`. The human types a task, and the agent reads, patches, runs the check, and reports. Nothing happens unless the human starts it. There are no wakes and nothing runs on a clock.

It remembers two things across runs: the human (taste, habits) and each repo (commands, conventions, goals). See [Memory](#memory-you-and-the-repo).

The loop core stays. It is the reason for the fork ([fork-cli-agent.md](fork-cli-agent.md)):

- name repair: prefix alias, five closest names, one constrained retry, printed-call salvage, a landing call
- round collapse: the last two rounds stay whole and older rounds become a one-line marker, with no completion spent
- `[mcp prefixes]` plus `mcp_enable`, with `force = true` on the four coding servers
- the session fold, which is one completion
- `/toolstats`, `/tokens`, `/perf`, `/tools`, `/new`, `/cancel`
- prompt goldens and `make integration-test`

## What comes out

Each row comes out whole: the package, its wiring in `cmd/george/run.go`, its config, its prompt text, its tests, and its fixtures. A flag that defaults to off is not the same as removing it. The prompt text and the goldens would still carry it.

| Feature | Code | Prompt surface | Why it goes |
| --- | --- | --- | --- |
| Daily planner | `internal/cron/planner*.go`, `plannerctl.go`, `agent/plannercmd.go`, `ensurePlannerJobs` | `plannerToolFirstNote`, `[progress]`, `/planner` | Not a coding task. |
| Cron | `internal/cron/` (whole), `cron.Composite` | `cron_*` tools, `cronToolFirstNote`, `[wakes]` | A coding turn is started by the human. |
| Follow-up `[wait]` | `cron/wait*.go`, `agent/wait.go`, `persona/wait.go` | `waitReplyNote`, the `## Follow-up` kernel section, `[conversation]` | Runs on cron. A question just ends the turn. |
| Capability examples | `internal/examples/`, `cron/examples.go`, `store_examples.go`, `agent/examplescmd.go` | `/examples` | Proactive pings. |
| Watches | `internal/watch/`, `watch.Composite` | `watch_*` tools | Polling a feed. |
| Aims ledger | `internal/aims/`, `agent/aimscmd.go`, `aims.Composite`, `ForgetAim` | `[aims]`, `/aims`, `aim/` rows | A household ledger. |
| Todo board | `memory/todo_board.go`, `agent/todocmd.go`, `todoBoard` | `[todo]`, `/todo`, `todo/` rows | Same. A coding todo is a file in the repo. |
| Horizon stamps | `memory/horizon.go`, `memory/hours.go`, `agent/harness.go` | `[hours]`, `[loops]`, `[last contact]` | No human schedule to read. |
| Telegram, Discord, Slack, pendant | `internal/channel/{telegram,discord,slack,pendant}`, `newChannel` | `[surface]`, `[input]`, `[room]` | stdio is the product. |
| Error forwarding | `internal/logfwd/` | none | Telegram only. |
| Reactions | `agent/react.go`, `channel/reaction.go`, `cron/react.go`, `persona/react.go` | `## Reactions` kernel section, `[react …]` | Chat-app emoji. |
| Location | `internal/here/`, `channel` geo, `## Location pins` | `[location …]` | GPS from a phone. |
| Photos | `channel/images.go`, `photos.go`, per-mouth `photo.go` | `[photo]` | No image input on stdio. |
| Consolidator | `memory/consolidate.go`, `MEMORY_CONSOLIDATE_MINUTES` | none | It runs one pass on boot plus one every 30 minutes, and each pass is a completion. A CLI boots on every launch, so you would pay for a completion before your first word. Rows come from the model storing on purpose and from the fold's `Facts:` episode. Nothing needs promoting. |
| Coalescing and steering | `agent/coalesce.go`, `COALESCE_SETTLE_MS` | steer messages | Built for chat bubbles. Dead on stdio. See [Input on stdio](#input-on-stdio). |
| Container and daemon | `Dockerfile`, `compose.yml`, `deploy/`, `docker.yml`, `dockerhub-description.yml`, `docs/deploy-*.md`, `docs/dockerhub.md` | none | george is a CLI. See [Where it lives](#where-it-lives-no-docker). |
| Health and status | `internal/heartbeat/`, `internal/doctor/`, `george status` | none | They exist for a Docker healthcheck and `docker exec`. Nothing is left running to check. |
| SIGTERM drain, SIGHUP reload | `internal/drain/`, `reload_unix.go`, `reload_stub.go` | none | Long-lived-process signals. Ctrl-C replaces them (see below). |
| `george auth` | `cmd/george/auth.go`, `auth_*` in the manifest | none | OAuth subprocesses for the assistant plugins. `github-mcp` reads `GITHUB_TOKEN`. |

Once the four mouths are gone, `go mod tidy` should drop `discordgo`, `go-telegram/bot`, `slack-go/slack`, `gorilla/websocket`, and `goldmark`. Check the result. Do not assume it.

## What stays, changed

| Feature | Change |
| --- | --- |
| Builtin memory | Gains a scope: the human, or this repo. See [Memory](#memory-you-and-the-repo). |
| `SELF.md` and `self_note` | Become the human's coding taste. They move to the user config dir and carry over to every repo. |
| `web_search` (Brave) | Unchanged. Off without `BRAVE_SEARCH_API_KEY`, which is already how it behaves. |
| Session | One conversation per repo, not one per process. Today the id is the constant `channel.AgentSession = "george"`. |

## What changes

### Persona

The prompt has three layers, and each has one owner:

| Layer | Owner | Holds | Evaluated |
| --- | --- | --- | --- |
| Contract | george, embedded in the binary (`internal/persona/contract.md`) | How the work is done: the edit chain, batching, the green check, commit and push rules, memory and `self_note` rules | Yes. The eval grades exactly this text |
| `PERSONA.md` | You | Name, voice, tone. Empty by default | No. We cannot vouch for what you add |
| `SELF.md` | The agent | Your coding taste, noted when you say it | No |

The prompt is contract, then `PERSONA.md`, then `SELF.md`. george never writes `PERSONA.md`. `persona.SyncKernel`, which stamped kernel sections into that file on every boot, goes away. A heading george used to own (`## Self-notes`, and the retired assistant sections) is dropped from an old `PERSONA.md` at load time, without touching the file.

The eval runs the contract with an empty `PERSONA.md`, which is the shipped default, so what passed is what you get. `-eval.persona=<file>` runs your persona on top of the contract, so you can find out whether your personality breaks the edit chain.

A persona can still contradict the contract, and a small model will not always let the contract win. That is why the persona is disclaimed, not guaranteed.

The contract text has to be earned. The first contract is the old seed's working rules, moved almost word for word, because the two rewrites tried so far scored 0 of 10 against 9 of 10 for the seed. Then the assistant-only parts come out one cut at a time, each cut held to `-eval.n=5` on every fixture, and a cut that drops the pass rate goes back in. The target is the coding list:

- identity in two lines, with answers first
- the chain: read before patch, patch before the check, and status only when asked
- parallel reads when the files are independent, and one `file_patch` per file in one batch
- never claim a check passed unless `command_run` returned exit 0
- commit only when asked, and never push (`git` has no push)
- stop after the same error twice, and report it
- through `shell`: no `git push`, `reset --hard`, or `clean`, no deletes outside the tree, and no publishes

Examples in the contract must not be the fixture's own words. A small model copies an example sentence into memory and into its replies.

### Harness stamp

`[harness]` drops down to what a coding turn can use: the clock (for commit dates and "what changed today") and the workspace root. A cheap `[workspace]` line (root path, plus the branch when the root is a git repo) would save a `git__status_get` call on most turns. The harness would have to call the tool to get the branch. That costs no completion, but it is still a tool call on every turn. Measure it before you ship it.

### Defaults that fight a 32k window

The eval sets `OLLAMA_CONTEXT_LENGTH=32768`. These defaults were sized for Gemini:

| Env | Now | Problem |
| --- | --- | --- |
| `HISTORY_MAX_TOKENS` | `32000` | History alone can fill the window before the persona, the schemas, and the tool results are added. |
| `TOOL_RESULT_MAX_CHARS` | `6000` | About 1.5k tokens. That's fine for one file. A parallel read of four files is about 6k. |
| `TOOL_MAX_ITERATIONS` | `25` (was `10`) | Done. Local tools cost nothing, and three parallel rounds (read, patch, test) plus fix-ups fit well under it. The cap only catches loops; per-fixture `round_budget` still flags waste in the eval. |
| `STREAM_REPLIES`, `SHOW_THINKING`, `TOOL_TRACE`, `SPINUP_NOTICE_MS`, `COALESCE_SETTLE_MS` | chat-bubble tuning | Keep the ones that mean something on stdio, and delete the rest. |

Set the new defaults from a measured reading: the `/tokens` output across one long coding session on the pinned model. Do not guess them.

### Config

`internal/config` loses every `TELEGRAM_*`, `DISCORD_*`, `SLACK_*`, `PENDANT_*`, `CRON_*`, `DAILY_PLANNER_AT`, `WATCH_*`, `EXAMPLES_*`, `MEMORY_CONSOLIDATE_MINUTES`, and `COALESCE_SETTLE_MS` value. `CHANNEL` goes away. Boot should fail when a removed variable is set (`CRON_ENABLED=true` on a coding build). If it is quietly ignored, an old env file will hide what was removed.

## Memory: you and the repo

There is no reason the agent can't remember a repo. The assistant memory is about the human because the assistant served one human. A coding agent has two subjects, and they live for different lengths of time:

| Scope | What lands there | Example rows |
| --- | --- | --- |
| **you** (every repo) | taste, habits, the way you like a change shaped | `pref/commits`: small commits, imperative subject. `pref/style`: no comments that narrate. `pref/workflow`: run lint before tests. |
| **this repo** | commands, conventions, order of operations, goals, gotchas | `cmd/test`: `make test`, never bare `go test` (needs tags). `order/release`: bump `VERSION`, then tag, then push. `goal/coding-agent`: cut the assistant features (see `docs/coding-agent-plan.md`). `gotcha/eval`: `make integration-test` sources `.env`. |

`SELF.md` sits next to the "you" scope. It holds the voice and taste lines that should shape every reply, not facts the agent looks up.

How it lands in the code, with the least change:

- `memory` gets one column: `scope TEXT NOT NULL DEFAULT 'user'`. `user` means you. A repo id means that repo. Hydration reads `scope = 'user' OR scope = <this repo>`. FTS, kinds, supersede-by-subject, and the `memory_*` tools stay as they are. `memory_store` gains `scope: "repo" | "user"`, defaulting to `repo`, because most coding facts are about the tree.
- **Repo id.** Use the normalized `origin` URL when there is one (it survives a re-clone and is the same in every worktree). Otherwise use the absolute path of the git toplevel. Otherwise use the cwd.
- **Goals** are `goal/<slug>` rows in the repo scope. They reach the prompt through hydration (FTS on the turn's text, at most about 30 rows), not through a standing stamp. This is the piece of the aims ledger worth keeping. It has no scoring, no ladder, and no planner. A finished goal is `memory_forget`.
- **Files in the repo win.** `AGENTS.md`, `CONTRIBUTING.md`, and the `Makefile` are the team's memory, and the agent reads them with `fs__file_get`. A memory row is what the agent learned that is not written there. The seed says so in one line. Memory never writes into the repo.
- **Session per repo.** The session id becomes the repo id, so `george` in repo A resumes repo A's conversation and `/new` resets only that one.

## Where it lives (no Docker)

george stops being a container. It runs the way `git` runs: from the directory you are in.

| What | Where | Why |
| --- | --- | --- |
| Model and keys (`LLM_*`, `BRAVE_SEARCH_API_KEY`, `GITHUB_TOKEN`) | `~/.config/george/env` | Every repo uses the same model and keys. Keys never sit in a repo. |
| MCP manifest | `~/.config/george/mcp.toml` | The same four servers in every repo. |
| `PERSONA.md`, `SELF.md` | `~/.config/george/` | Your taste goes with you. |
| `george.db` (sessions, memory, `mcp_enable` grants) | `~/.local/share/george/george.db` | One file, scoped by repo id. You can open it with `sqlite3`. |
| MCP binaries | `~/.local/share/george/bin`, filled by `george tools-fetch` | The host puts that directory first on `PATH` for its children. |
| Root | git toplevel of the cwd, or else the cwd | `cd repo && george` is the whole UX. |

Mechanics, picked so that most of it is imported or already in the tree:

- The env file is loaded with [`github.com/joho/godotenv`](https://github.com/joho/godotenv) before `env.Parse`. `godotenv.Load` does not override variables that are already set, so the real environment still wins and the eval's `.env` keeps working. `GEORGE_CONFIG_DIR` overrides the path.
- The XDG paths come from [`github.com/adrg/xdg`](https://github.com/adrg/xdg). The stdlib has `os.UserConfigDir` but no data dir.
- The root reaches the children as `GEORGE_ROOT`. Manifest `args` and `env` go through `os.Expand`, the same expansion `auth.go` and `skip.go` already do. The manifest says `args = ["--root", "${GEORGE_ROOT}"]` instead of `/workspace`.
- `george init` writes `~/.config/george/` once (env template, manifest, persona) and refuses to overwrite.
- A per-repo override file is out of v1. When one is needed, it may carry settings and must never carry keys.

**No sandbox.** [coding-mcp.md](coding-mcp.md) says "the container is the sandbox" for `shell`. Without the container, `shell__command_run` runs as you, on your machine, in `--root`. Nothing replaces the container: git is the undo. See the decisions at the end.

## Input on stdio

"Chat-shaped" means the input model of Telegram or Slack. People send short bubbles ("hey", "can you", "check the build"), so the agent waits 2 seconds for the burst to settle and merges it. A bubble that arrives during a turn steers that turn. None of this happens on stdio today. `stdio.Run` calls `handle` and blocks until the reply is done before it reads the next line. Nothing is read during a turn, so nothing steers. And a pasted 40-line stack trace becomes 40 turns, one after another.

The coding REPL needs these instead:

- **A paste is one message.** Read bracketed paste (the terminal wraps a paste in `ESC[200~` … `ESC[201~`), so a paste of any length arrives as one message and Enter sends it. Import a line editor that already handles this, such as [`github.com/chzyer/readline`](https://github.com/chzyer/readline) or [`github.com/reeflective/readline`](https://github.com/reeflective/readline). Do not write a terminal parser. A piped stdin (not a TTY) is read to EOF as one message.
- **Ctrl-C cancels the turn, not the process.** The first Ctrl-C does what `/cancel` does. A second one at an idle prompt exits. Today `signal.NotifyContext` kills the whole process on the first one.
- **No steering.** Delete `coalesce.go`. To change direction, press Ctrl-C and type the new instruction.
- **History and arrow keys** come from the same line editor.

## Goldens first

The only free golden of the full request is `TestPendantInbound_CompleterPayload`, and it starts from pendant inbound JSON (`testdata/pendant/`). Before pendant is deleted, add a stdio-inbound golden that covers the same path: stdin line, `Handle`, the Completer request, and the HTTP body through `prompt_wire_test.go`. Then delete the pendant goldens. Each removal phase below ends with `go test -update` and a reviewed golden diff. Read the diff. That is the check that the prompt lost only what you meant to remove.

## Evals

26 of the 27 fixtures under `internal/agent/testdata/eval/` grade the assistant: the planner, the calendar, Garmin, rentals, reactions, and todos. They go away along with the features they grade. `edit_then_check` stays.

The coding set. Each fixture is a rule the pinned model has to pass on every run:

| Fixture | Rule | Needs |
| --- | --- | --- |
| `edit_then_check` | get, then patch, then run, then status, in that order | exists |
| `mangled_name` | after a near-miss is refused, the model picks the right tool from the candidate list on the next round | **harness change**: a scripted first round (see below) |
| `search_then_patch` | rename a symbol: `file_search`, then one `file_patch` per hit in one batch | none |
| `fix_failing_test` | `command_run` fails, then get, then patch, then `command_run` passes, and the reply says it passed | **harness change**: tool results are one string per tool today, so the second run cannot return a different result. Add `results: [...]` (the nth call gets the nth result, and the last one repeats). |
| `no_false_green` | `command_run` returns exit 1, and the reply does not claim it passed | `reply_not_regex` |
| `commit_only_when_asked` | "fix the typo" leads to no `commit_create`, and "fix it and commit" leads to `stage_update` then `commit_create` | `tools_not_called`, two fixtures |
| `create_not_patch` | a new file goes through `file_create`, and an existing one through `file_patch` | none |
| `long_session` | a session of 8 or more rounds still answers after older rounds have been collapsed, and the round count equals the completion count | the `todo.md` done-when item |

### Mangled names

A bad call already produces a list of possible tools. `mcp.Host.suggest` returns a hint plus up to five candidates, and the agent retries once with the tool choice constrained to those names. What a fixture cannot do is make the pinned model emit a bad name, because a good model mostly calls the right one. The test splits in two:

- **Repairs that need no model** are free unit tests in `internal/mcp`, run against the real `fs-mcp` child. A hyphenated or underscored prefix (`hyphenatePrefix`) and a missing or invented prefix (`resolveByBaseNameLocked`) already land without a round. **A dot separator does not.** `fs.file_get` has no `__`, so it falls through to `suggest` and costs a whole model round on a local GPU. Add `.` → `__` to `resolve` as one more alias, with a test.
- **The repair the model does** is a live fixture with no history. Add a fixture field `script`: the first N completions are canned tool calls, not the live model. Script round 1 as `fs__file_read` with `{"path":"greet.txt"}`. The host refuses it, and the real hint and candidates go back into the transcript. Round 2 and later are the live model, which has to call `fs__file_get` and finish the task. Assert the order and that `/toolstats` counts one unknown tool and one constrained retry. The eval code is the only place that seeds the round. It never touches the session or the persona.

Fixture fields that only the assistant used come out of the harness (`planner`, `cron`, `surface`, `input`, `waiting`, `ledger`, `react*`, `no_model_call`, `cron_within`, `prices_from_tools`). Their checks come out of `eval_harness_test.go` in the same change.

`eval_setup.md` sections 4 and 5 get rewritten around these fixtures. The bake-off method stays.

## Order of work

Every phase ends the same way: `make check` is green, the golden diff has been read, and `edit_then_check` passes live on the pinned model (`-eval.n=3`). A phase that breaks the live fixture gets fixed before the next phase starts.

1. **Baseline.** Record `go test ./...`, the coverage number, and three live runs of `edit_then_check` with their round and token means. Every later phase is compared against this.
2. **stdio golden.** Add it, then delete the pendant goldens.
3. **Mouths.** Delete telegram, discord, slack, pendant, logfwd, reactions, location, photos, and the `[surface]`, `[input]`, and `[room]` stamps. `run.go` gets `stdio.New()` with no switch.
4. **Clock.** Delete cron, the planner, examples, `[wait]`, watches, the slash commands they owned, and their notes and kernel sections. `channel.Pusher` goes if nothing else uses it.
5. **Household.** Delete aims, the todo board, the horizon and hours stamps, and the consolidator, and drop those subjects from the `memory` docs and tests.
6. **Off Docker.** Delete the container files, heartbeat, doctor, `status`, drain, SIGHUP reload, and `auth`. Add the env file, the XDG paths, the root from the cwd, and `${GEORGE_ROOT}` in the manifest. Gate: `cd /tmp/scratch && george` boots against `~/.config/george/` with no `.env` in that directory. Done: the container files, `deploy/`, heartbeat, doctor, `status`, and `auth` are gone. The env file, the XDG defaults, the bin dir on `PATH`, and the root from the cwd are in, and the gate passed in a fresh `HOME` ([todo.md](todo.md#8-work-list)).
7. **Memory scope.** Add the `scope` column, the repo id, and the session per repo. Gate: a row stored in repo A does not hydrate in repo B, and a `user` row hydrates in both.
8. **stdio input.** Add the line editor, bracketed paste, and Ctrl-C to cancel, and delete coalescing. Gate: a 40-line paste is one turn.
9. **Contract.** Move the seed's rules into the embedded contract, drop `SyncKernel`, and ship an empty `PERSONA.md` template. Gate: the eval passes at the same rate as before the move. Then trim the assistant parts one cut at a time under `-eval.n=5`, and add the memory lines (repo versus you, and repo files first).
10. **Context budget.** Read `/tokens` on a long session, then set the history, result, and iteration defaults.
11. **Evals.** Add `results: [...]` and `script` to the harness and the dot alias to the host, then the coding fixtures, one at a time, each green at `-eval.n=5` before the next one starts. Add a memory fixture: "tests here run with `make test`" lands a repo-scope `cmd/test` row.
12. **Docs.** `architecture.md`, `design.md`, `readme.md`, `eval_setup.md`, `coding-mcp.md` (sandbox), `examples/`. They should describe the CLI and nothing that was removed.

## Decisions

- **Memory:** kept, scoped to you and to the repo, with goals as repo rows. Repo files win over rows.
- **Contract and persona:** how the work is done is george's contract, embedded and evaluated. `PERSONA.md` is yours, for personality only, empty by default, and not evaluated. See [Persona](#persona).
- **SELF.md:** kept, as your coding taste, under `~/.config/george/`.
- **Consolidator:** removed. It is a completion on every boot, and the fold plus deliberate stores already fill the table. If rows ever pile up, bring it back as an explicit `george memory consolidate`, not a timer.
- **web_search:** kept as it is.
- **Steering:** removed. Ctrl-C to cancel and bracketed paste replace it.
- **One-shot mode:** not planned. There is no use case yet.
- **Mangled names:** unit tests for the host repairs, plus a dot alias, plus one live fixture with a scripted first round.
- **Root and config:** the root is the cwd's git toplevel, keys and settings live in `~/.config/george/env`, and data lives in `~/.local/share/george/`.
- **Shell sandbox:** none. No approval prompt, no `bwrap`. The agent edits and runs freely in `--root`, and git is the undo. Git does not cover these, and the contract's last bullet (see [Persona](#persona)) is the only guard on them:
  - untracked and ignored files (`.env`, build output) have no history to revert to
  - anything outside `--root`; `shell-mcp` sets the cwd, not a jail
  - git commands run through `shell`, such as `git push`, `git reset --hard`, and `git clean`, which skip the no-remote rule on `git-mcp`
  - network side effects (`gh`, `curl`, package publishes)

## Still open

- **Repo id after a remote rename.** An `origin` URL change orphans the repo's rows. Either live with it, or add `/memory move <old>` later.

## Done when

- `rg -il 'telegram|discord|slack|pendant|planner|aims|cron_|watch_' internal cmd` finds nothing outside history notes.
- `go.mod` has no chat SDKs.
- The default `[harness]` block on a coding turn shows only the clock and the workspace.
- Every coding fixture passes every run at `-eval.n=5` on `qwen3.6:35b-a3b-coding`, and the log prints that id.
- A long session on the pinned model stays under the 32k window, as measured by `/tokens`.
- The repo has no `Dockerfile`, no `compose.yml`, and no `deploy/`. `cd any-repo && george` works with keys from `~/.config/george/env` only.
- `sqlite3 ~/.local/share/george/george.db 'select scope, subject from memory'` shows `user` rows and per-repo rows side by side.
