# Review

Reading of this tree on 2026-10-02. The live gate was not re-run. Numbers
below are the ones already written in [todo.md](todo.md) and
[eval_setup.md](eval_setup.md).

**Yes, with a narrow brief.** I would use george, and I would hand it to
someone who already runs a local coder model and wants a terminal agent
whose loop was built for that model. I would not hand it to someone who
wants a finished coding CLI, a sandbox, or a second project that must not
see the first project's memory.

## What it is

One static Go binary. You run it in a repo. It talks on stdin/stdout to one
OpenAI-compatible endpoint and to four MCP children (`fs`, `git`, `shell`,
`github`) rooted at that repo. The graded model is
`qwen3.6:35b-a3b-coding` on Ollama, about 22GB. Git is the undo. There is
no approval prompt and no shell jail.

The interesting work is the turn loop in `internal/agent` and the MCP host
in `internal/mcp`: parallel batches, name repair, in-process collapse of
old tool rounds, a loop guard, and a landing call when the round budget
runs out. The edit tools live in other repositories. This process hosts
them.

## Pros

**The loop is aimed at a local model, and the tests know that.** Name
repair (wrong separator, unique bare name, five closest names, one
constrained retry), salvage of a tool call printed as JSON, collapse of
rounds older than the last two with no extra completion, and a landing
call at `TOOL_MAX_ITERATIONS` are all in the binary and covered by unit
tests. `LLM_SYSTEM_FOLD` exists because a probe showed a renderer dropping
every system message after the first. That is the right kind of fix.

**Two contracts, and they measure different things.** `go test ./...`
pins the bytes the model sees. `make integration-test` replays fixtures
against a live model and grades tools, arguments, batch shape, and the
files on disk. It does not grade the sentence. Six fixtures sit in
`internal/agent/testdata/eval/`. A miss is written down with a fraction
(`parallel_patches` 0/5 on the previous model, then 3/3 on the current one
after the fixture itself was fixed). A project that publishes its red
runs is easier to trust than one that publishes a family tag.

**The cut from the assistant is real.** Chat channels, cron, the planner,
watches, aims, and the consolidator are gone. `config.CheckRetired` fails
boot if an old env var is still set. One provider client. Pure-Go SQLite,
FTS5, no embeddings. `CGO_ENABLED=0`. Config is an env file and a few
markdown files under `~/.config/george/`. Memory is a file you can open
with `sqlite3`.

**The docs say what was rejected.** [design.md](design.md) and
[fork-cli-agent.md](fork-cli-agent.md) record why there is no second
model, no tool-pair summarizer, no skills directory, and no sandbox. The
security section names the residual: `shell__command_run` runs as you, a
file can inject a prompt, and the contract is the only guard on `git push`
through the shell. That is a usable threat model.

**Dependencies are few and ordinary.** The module directly requires the
MCP Go SDK, the OpenAI Go SDK, `modernc.org/sqlite`, godotenv, env parsing,
XDG paths, and a TOML parser. Go 1.26. CI runs vet, tests, golangci-lint,
and a static build.

## Cons

**The product around the loop is still the old shape.** These are open in
[todo.md](todo.md) and called out at the bottom of
[architecture.md](architecture.md):

- One session id, the constant `george`, and one memory table for every
  repo. A fact stored in repo A is hydrated in repo B.
- Stdio is a line scanner. A paste is not one message. The first Ctrl-C
  cancels the process, not the turn.
- The context defaults (`HISTORY_MAX_TOKENS=32000`,
  `TOOL_RESULT_MAX_CHARS=6000`) were sized for a cloud window. The eval
  sets `OLLAMA_CONTEXT_LENGTH=32768` so Ollama does not reserve the model's
  advertised 256k.
- "Teach it what done is" is unchecked. Least-change, expand-don't-copy,
  and test-first were tried in the contract and taken back out because the
  gate rate fell. The current model has a 3/3 on `parallel_patches` and
  has not been stamped at `-eval.n=5`.

**Edit quality depends on binaries this repo does not own.** A 30B model
miscounted hunk headers; the fix landed in `fs-mcp`. Renames still miss a
call site when the model leaves it as diff context. The request for an
exact-string replace mode is [mcp_todo.md](mcp_todo.md), and the check is
"no change in this repo." Until that ships, a rename of two files is the
fixture this harness is worst at, and george cannot fix it alone.

**The shell is the security boundary.** `fs` and `git` refuse a path that
escapes `--root`. `shell` sets the working directory and then runs as you.
`git-mcp` has no push; the shell can still run `git push`, `curl`, and
`gh`. Untracked files, including `.env`, have no history to revert. I would
run it in a repo I am willing to let a model dirty, with secrets kept out
of that tree.

**The prompt still carries assistant habits.** [contract.md](../internal/persona/contract.md)
teaches voice, jokes, nicknames, and storing a preference the same turn it
is learned. That fights a coding turn. The household cut already showed the
failure mode: a goal example copied from a fixture made the model replay
that example on a different task. The contract is load-bearing and still
easy to overfit.

**Some docs describe a process that is no longer here.**

| Place | What it says | What the tree does |
| --- | --- | --- |
| [eval_setup.md](eval_setup.md) | Missing `LLM_*` skips `TestEval_Live`, and the sample fixture is `scoop_at_2` | `TestEval_Live` calls `t.Fatal`. The fixtures on disk are `27`–`32`. `scoop_at_2` is gone. |
| [auth.md](auth.md) and `.github/workflows/ci.yml` | Chat `/auth`, OAuth catch page, `george auth` | [coding-agent-plan.md](coding-agent-plan.md) lists `george auth` as removed. CI still publishes `docs/oauth-catch`. |
| [eval.yml](.github/workflows/eval.yml) | Points at `docs/evaluation.md`. Comments assume ~15s a turn on `ubuntu-latest` | That file does not exist. A local Qwen turn is prefill plus decode; [eval_setup.md](eval_setup.md) caps one turn at 10 minutes. |
| [coding-agent-plan.md](coding-agent-plan.md) | "The session fold, which is one completion" stays | The summarizer was deleted. A trim drops the oldest rows. |

The design docs (`architecture.md`, `design.md`, `todo.md`) are ahead of
those leftovers. A new reader who starts at `eval_setup.md` or `auth.md`
will follow instructions the code will not honor.

## Would I recommend it?

Yes, for this job: a terminal coding agent on one local model, where you
care that a mangled tool name still runs, that old tool results do not
cost another completion, and that a regression is a wrong tool call rather
than a wrong sentence.

The setup I would accept is the one the readme already describes.
`ollama pull qwen3.6:35b-a3b-coding`, `george init`, `george tools-fetch`,
then `cd` into a git repo and type. Keep `OLLAMA_CONTEXT_LENGTH` modest.
Treat `~/.config/george/env` and `george.db` as secrets. Read `git status`
before you trust a reply. Use `/new` when the transcript gets long, and
expect the next repo to still see the memory rows from this one until
phase 7 lands.

I would wait, and I would say so, if the person needs any of these on day
one:

- a conversation and a memory that belong to one repo
- a line editor, bracketed paste, and Ctrl-C that cancels the turn
- a gate stamped 5/5 on the current model, including the rename fixture
- an approval prompt or a shell that cannot leave the root

I would point them at a different tool if they want a GUI, a sandbox, many
providers, or a model they do not already have running locally. george is
not trying to be that, and the non-goals in [design.md](design.md) are the
right list.

The harness is the part I would keep. The REPL, the shared database, and
the unfinished "done" rules are the part I would not pretend is finished.
