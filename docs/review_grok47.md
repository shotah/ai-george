# Review

Second reading, 2026-10-03. The first reading of this file (2026-10-02)
listed a shared session, a line scanner, and a rename fixture that had not
been stamped at five runs. Those are no longer the state of the tree. The
other reading is [review_fable51.md](review_fable51.md). I re-read the code
and the docs. I did not re-run `go test` or the live gate. Gate numbers
below are the ones in [todo.md](todo.md).

**Yes, with a shorter wait list than the first reading.** I would use
george, and I would hand it to someone who already runs one local coder
model and is willing to let it edit a git repo. The two blockers I named
last time for day-one use — one memory for every repo, and a REPL that
cannot take a paste — are in the binary. What I would still wait on is a
pinned tool surface, a context budget that fits the window, and safety
rules the host enforces instead of the contract.

## What changed

| First reading | This tree |
| --- | --- |
| Session id is the constant `george`. One memory table for every repo. | `repoID` in `cmd/george/root.go` is the normalized `origin` URL, else the root. It is the stdio session id. `memory.scope` is `user` or that id. A `fact` stored in repo A is not read in repo B. A `preference` is. Old rows, from before the column, stay `user` and still show everywhere. |
| Stdio is a line scanner. Ctrl-C kills the process. | On a terminal, `golang.org/x/term` with bracketed paste makes a paste one message. `turn` in `internal/channel/stdio/stdio.go` cancels that turn on SIGINT and returns to the prompt. Ctrl-C at the prompt exits. Piped stdin is still one line per message. There is no history across messages. |
| `parallel_patches` was 3/3 and not stamped at `-eval.n=5`. The rename needed a tool this repo does not own. | `file_patch` `old`/`new` shipped in `fs-mcp`. [todo.md](todo.md) stamps that fixture 5/5 at `-eval.n=5` on `fs-mcp` 0.0.4, four rounds, no contract change. The last full six-fixture reading on the current model is 28 of 30. |
| [eval_setup.md](eval_setup.md) said a missing `LLM_*` skips, and named `scoop_at_2`. [auth.md](auth.md) described chat `/auth`. [coding-agent-plan.md](coding-agent-plan.md) still called the session fold a completion. | Those three were rewritten. `TestEval_Live` fails without `LLM_*`. Auth is a short note that the flow was removed. The plan says the trim deletes rows and spends no completion. |

## What it is

One static Go binary. You run it in a repo. It talks on stdin/stdout to one
OpenAI-compatible endpoint and to four MCP children (`fs`, `git`, `shell`,
`github`) rooted at that repo. The graded model is
`qwen3.6:35b-a3b-coding` on Ollama, about 22GB. History and memory are one
SQLite file. Git is the undo. There is no approval prompt and no shell jail.

The interesting work is still the turn loop in `internal/agent` and the MCP
host in `internal/mcp`: parallel batches, name repair, in-process collapse
of old tool rounds, a loop guard, and a landing call when the round budget
runs out. The edit tools live in other repositories. This process hosts
them.

## Pros

**The loop is aimed at a local model, and the tests know that.** Name
repair (wrong separator, unique bare name, five closest names, one
constrained retry), salvage of a tool call printed as JSON, collapse of
rounds older than the last two with no extra completion, and a landing
call at `TOOL_MAX_ITERATIONS` are in the binary and covered by unit tests.
`LLM_SYSTEM_FOLD` exists because a probe showed one renderer dropping every
system message after the first. On the current model the same probe reads
the later message, and the gate still runs folded. That is the right kind
of fix.

**Per-repo memory is the shape the plan asked for, and it is small.** Scope
follows the kind, not a new tool argument: `preference` and `person` are
`user`; `fact`, `insight`, and `episode` are the repo. Reads take `user`
plus this repo. Supersede-by-subject stays inside one scope. An existing
`george.db` gets the column on open. `/memory move <old repo id>` retags
rows after a remote rename. The live check in [todo.md](todo.md) is the one
I wanted: repo B answered that it had no memory of a command stored in
repo A, and repo A resumed its own conversation.

**The REPL is a terminal now.** Bracketed paste is tested in
`paste_test.go`. Ctrl-C during a turn is tested as cancelling that turn
only. `x/term` is the right import: maintained, and it does the one thing
the plan needed. History and arrow keys across messages are still open,
and [architecture.md](architecture.md) says so.

**Two contracts, and they measure different things.** `go test ./...`
pins the bytes the model sees. `make integration-test` replays six fixtures
against a live model and grades tools, arguments, batch shape, and the
files on disk. It does not grade the sentence. The last full reading at
`-eval.n=5` on `qwen3.6:35b-a3b-coding` is 28 of 30: four fixtures at 5/5,
`edit_then_check` 4/5, `parallel_patches` 4/5 in that sweep. A later stamp
of the rename fixture alone, on `fs-mcp` 0.0.4, is 5/5. A project that
publishes both numbers is easier to trust than one that publishes a family
tag.

**The cut from the assistant is real.** Chat channels, cron, the planner,
watches, aims, and the consolidator are gone. `config.CheckRetired` fails
boot if an old env var is still set. One provider client. Pure-Go SQLite,
FTS5, no embeddings. `CGO_ENABLED=0`. Config is an env file and a few
markdown files under `~/.config/george/`.

**The docs that matter caught up.** [readme.md](../readme.md),
[architecture.md](architecture.md), [design.md](design.md), and
[coding-agent-plan.md](coding-agent-plan.md) describe the scope column, the
repo session id, bracketed paste, and Ctrl-C the way the code does.
[auth.md](auth.md) says the OAuth flow was removed.

## Cons

**The rules that cost gate runs are still sentences.** The contract says a
patch waits on a read of that path, and that `git__commit_create` and
`git__stage_update` do not run unless the user asked this turn. The host
sees every call and could refuse those. It does not. Each rule was taught
with live runs, and `edit_then_check` still misses one run in five by
calling `git__status_get` in the same round as the check. A refusal in the
host would hold on any model and leave the contract for judgment. I agree
with [review_fable51.md](review_fable51.md) on this.

**The cue lists are assistant leftovers that still steer a turn.**
`promisesToolCall` and `defersPendingWork` in `agent.go` are hard-coded
English phrases. One cue is still `"query body battery"`. A hit is an extra
round, or a forced give-up after tools have already run. A miss ships
"Let me first…" as the reply. They are tested against the phrases someone
already wrote down.

**The graded binaries float.** `examples/mcp.toml.example` and the eval
manifest set `download_tag = "latest"`. `internal/mcp/download.go` resolves
that through the GitHub API and checks no checksum. The 5/5 rename stamp
names `fs-mcp` 0.0.4 by hand. The next release of that binary changes the
system the gate is grading, and the stamp goes stale without a diff in
this repo. The readme publishes `checksums.txt` for george itself.

**The context budget is still the cloud size.** `HISTORY_MAX_TOKENS=32000`
is the whole `OLLAMA_CONTEXT_LENGTH` the eval sets, before schemas, the
contract, hydration, and this turn's tool results. [todo.md](todo.md) item
5 is open. A long session hits the window before the history trim does.
This is the open item I would do next.

**The shell is still the security boundary.** `fs` and `git` refuse a path
that escapes `--root`. `shell` sets the working directory and then runs as
you. `git-mcp` has no push; the shell can still run `git push`, `curl`, and
`gh`. Untracked files, including `.env`, have no history to revert. I would
run it in a repo I am willing to let a model dirty, with secrets kept out
of that tree.

**Two work-list lines are behind the code, and one plan section is behind
the tree.**

| Place | What it still says | What the tree does |
| --- | --- | --- |
| [todo.md](todo.md) item 4 | Unchecked. Import a line editor, bracketed paste, Ctrl-C cancels the turn, delete `coalesce.go`. | Done in `stdio.go`. [coding-agent-plan.md](coding-agent-plan.md) marks paste, Ctrl-C, and the coalesce deletion done. |
| [todo.md](todo.md) item 1, last sentence | The memory move is not done. | `Builtin.Move` and `/memory move` are in the binary. The readme lists the command. |
| [todo.md](todo.md) section 7 | The four servers are in `deploy/mcp.toml`. | `deploy/` is gone. `george init` writes `examples/mcp.toml.example`. |
| [coding-agent-plan.md](coding-agent-plan.md), Goldens first | The only full-request golden is `TestPendantInbound_CompleterPayload`. | No pendant testdata. The stdio golden is `internal/agent/testdata/stdio/`. |
| `.github/workflows/ci.yml` | Publishes `docs/oauth-catch` as the chat `/auth` catch page. | [auth.md](auth.md) says nothing in the binary points at that page. |

`budget.go` still meters a per-server vendor quota at the human's midnight,
and `/brief` `/short` `/off` still hold a prefix for 6 or 27 hours.
`Host.CallRaw`'s comment still names the watch poller. Four `force = true`
coding servers do not need that machinery. It is weight, not a bug.

## Would I recommend it?

Yes. The job is a terminal coding agent on one local model, where a
mangled tool name still runs, old tool results do not cost another
completion, a fact learned in one repo stays there, and a regression is a
wrong tool call rather than a wrong sentence. For that job the loop is
ahead of what I would trust from a weekend project, and the eval is ahead
of what most agent projects ship.

The setup I would accept is the one the readme describes. `ollama pull
qwen3.6:35b-a3b-coding`, `george init`, `george tools-fetch`, then `cd`
into a git repo and type. Keep `OLLAMA_CONTEXT_LENGTH` modest. Treat
`~/.config/george/env` and `george.db` as secrets. Read `git status` before
you trust a reply. After `tools-fetch`, know that the next `fs-mcp` release
is what you will be running, not the 0.0.4 the stamp names.

I would wait, and I would say so, if the person needs any of these on day
one:

- the four server tags pinned, and a checksum on what `tools-fetch` installs
- a history cap that leaves room for schemas and tool results inside 32k
- a full six-fixture stamp at 5/5 after the `fs-mcp` 0.0.4 rename reading,
  including the mangled-name fixture, which is still open
- read-before-patch and no-unasked-commit as host refusals, not contract lines
- an approval prompt or a shell that cannot leave the root

I would point them at a different tool if they want a GUI, many providers,
or a model they do not already have running locally. The non-goals in
[design.md](design.md) are the right list.

The harness is the part I would keep, and more of the product around it is
real than it was yesterday. The prose rules, the floating tool versions,
and the context default are the part I would fix before calling the work
list done.
