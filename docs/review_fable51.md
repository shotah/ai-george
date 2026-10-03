# Review

Reading of this tree on 2026-10-02 by a different model than
[review_grok47.md](review_grok47.md). I read the code, the docs, and the
fixtures, and I ran `go build`, `go vet`, `go test ./...`, and
`golangci-lint run` on the working copy. All four were green. I did not
run the live gate; the numbers quoted are the ones in [todo.md](todo.md).

One thing to say up front: the working copy is ahead of its own docs.
`git status` shows uncommitted edits to `run.go`, `stdio.go`,
`builtin.go`, `agent.go`, and `scope_test.go`, plus a new
`paste_test.go`. Those edits are the per-repo session id, the memory
`scope` column, bracketed paste, and Ctrl-C that cancels the turn. The
previous review lists all four as missing, and so do `design.md`,
`architecture.md`, and the readme. The code has them. I reviewed what is
on disk.

**Yes, with the same narrow brief, and a shorter list of blockers than
the last review had.** I would hand george to someone who already runs
one local coder model, reads logs, and wants to see every decision the
loop makes. I would not hand it to someone who wants to install it and
forget it: the graded tool surface floats on `latest`, the safety rules
are prose, and the docs will tell them a feature is missing when it is
not.

## What it is

A static Go binary that turns one local OpenAI-compatible model into a
terminal coding agent. It spawns four MCP children (`fs`, `git`,
`shell`, `github`) rooted at the git toplevel, keeps one SQLite file for
history and memory, and talks on stdin/stdout. The loop in
`internal/agent/agent.go` is about 1,350 lines, and most of the
interesting decisions are in it: parallel tool batches, name repair,
salvage of a printed tool call, collapse of old tool rounds, a cycle
guard, a budget warning at 70%, and a landing call with tools withheld.

The eval in `internal/agent/testdata/eval/` is six JSON fixtures that
replay a task against the live model with the real MCP binaries and
grade which tools ran, in which batch, in which order, and what the
files on disk say afterwards.

## Pros

**The loop reads like a lab notebook, and that is a compliment.** Nearly
every branch in `runLoop` carries a comment that says which model did
what to earn it: Qwen returning the answer in `thinking` with empty
`content`, a renderer dropping every system message after the first, a
nudge riding as a user turn because the fold would otherwise end the
transcript on the assistant's own prose. I could follow why each
heuristic exists without opening the git history (which I did not).
That is rarer than it should be.

**The cycle guard is the right shape.** `loopguard.go` hashes each
round's calls, arguments, and results, order-free, and lands the turn
only when the last k rounds repeat the k before them. A test-fix-retest
loop is not a cycle while the fix differs. A naive "same call twice"
guard would have broken the exact workflow a coding agent needs. The
unit tests cover it, and the live log in `todo.md` says it fired three
times in one gate run and shortened the worst `edit_then_check` run to
eight rounds.

**Name repair costs no model round.** `Host.resolve` tries the exact
name, a hyphenated prefix, a wrong separator (`fs.file_get`,
`git_status_get`), then a unique bare name, and only after all of those
does it hand back five candidates for a constrained retry. The two cases
it deliberately refuses to guess (a real prefix on the wrong server, two
servers with the same tool name) are written down in the comment. On a
local model a wasted round is the most expensive thing in a turn, so
this is where effort should go, and it did.

**The eval grades the right thing.** Fixtures check tools, arguments,
batch membership, order, a file regex on disk, and a reply regex. They
never check the sentence. `parallel_patches` seeds a real `go.mod`, runs
the real `fs` and `shell` binaries in a temp dir, and fails if `a_test.go`
does not contain `Greet("bob")` afterwards. A fixture that fails for a
fixture bug is written up as a fixture bug (the canned `shell` that
returned `{}` and caused a 24-round loop). The failure vocabulary in
[eval_setup.md](eval_setup.md) is good enough that I could read a red
run without the code.

**The cut is real and enforced.** `config.CheckRetired` refuses to boot
with a `TELEGRAM_*`, `CRON_*`, or `WATCH_*` variable set. One provider
client. No embeddings. No summarizer completion. `CGO_ENABLED=0`. Nine
direct dependencies, all mainstream. `golang.org/x/term` as the line
editor is the right "import over write" call for bracketed paste; it has
no history or multi-line editing, and that is fine for now.

**The repo publishes its own red runs.** `todo.md` records 0/5, 1/2,
2/5, and the sentence that caused each. Two abstract persona rewrites
"scored 0/10" and were reverted. The security section names what is not
guarded. This is the kind of project where I trust the green numbers
because the red ones are beside them.

**It tests clean today.** `go test ./...` passes on every package,
`golangci-lint` reports zero issues, `go vet` is quiet, and the static
build works. Coverage is 82.8% on `internal/agent`, 79% on
`internal/mcp`, and 89% on `internal/config`. The `cmd/george` number
(31%) is wiring, which is normal.

## Cons

**The harness enforces in prose what it could enforce in code.** The
project's thesis is that the model predicts tokens and the harness makes
the turn land. Then `contract.md` spends most of its Code section on
rules the host could check deterministically:

- "The patch waits on the read, even when they told you what the file
  says." The host sees every `fs__file_get` and `fs__file_patch` this
  turn and could refuse a patch on a path that was not read.
- "Never `git__commit_create` or `git__stage_update` unless they asked
  this turn." The host could refuse those two names unless the user
  line contains a commit verb, and return the refusal as the tool
  result.
- "Same error twice, stop and report." The cycle guard already has the
  signature for this.

Each of those rules cost gate runs to teach and is still carried by a
sentence the model may skip. A code guard would make the fixture pass
on any model and free the contract for the rules that really are
judgment. The "patch waits on the read" rule in particular is paid for
on every edit turn with an extra round by design, so it should be the
cheapest thing to make unconditional.

**The cue lists are the weakest code in the loop.** `promisesToolCall`,
`defersPendingWork`, and `claimsToolSuccess` are hard-coded English
phrase lists. One of them still contains `"query body battery"`, a
Garmin leftover. These lists decide when a nudge fires, and a nudge is a
whole extra round or, after tools, a forced give-up reply. A false
positive on a legitimate final answer ("I'll leave the rename for a
follow-up PR" contains `i'll`) costs a round; a miss ships "Let me
first…" as the reply. They are tested, but they are tested against the
phrases someone already thought of. I do not have a better mechanism to
offer that does not spend a completion, so this is a known cost, but it
should be named as one.

**The contract and the fixtures co-evolved, and it shows.** `todo.md`
says plainly that the goal example in the contract was once the
`edit_then_check` inbound word for word, and the model replayed it on a
different task. The fix was to make the example a "structural twin"
instead. That is still teaching the model the fixture's shape. Six
fixtures, all under ten lines of workspace, all graded on the same
four-tool chain. 28/30 is a reading on those six shapes. I would want a
fixture the contract was not written against before I believed the rate
transfers: a three-file change, a task where the check fails on the
first try, or a task phrased without naming the file.

**The graded surface is not pinned.** `tools-fetch` and the eval
manifest both resolve `download_tag = "latest"` through the GitHub API,
and `download.go` has no checksum or signature check. Two consequences.
The eval numbers in `todo.md` name `fs` 0.0.3 and 0.0.4 by hand, but
nothing in the repo holds those versions; the next `fs-mcp` release
changes the graded system and the stamp silently goes stale. And the
install path trusts GitHub TLS and the release author, which is a
normal choice, but the readme's own `checksums.txt` shows the project
knows how to do better for itself and does not do it for the four
binaries that run with shell access.

**Assistant-era machinery is still in the binary.** `budget.go` and
`budgetstore.go` meter per-server vendor quotas with a reset at the
human's midnight. `mcpenable` keeps `/brief`, `/short`, and `/off` holds
of 6 and 27 hours. `auth.go` and `auth_args` support a `george auth`
command that [coding-agent-plan.md](coding-agent-plan.md) says is gone.
`Host.CallRaw` exists for "the watch poller." None of this serves four
`force = true` coding servers, and the model still reads a line about
reviewing `[mcp prefixes]`. It is not wrong code; it is weight the
maintainer pays to keep lint-clean and the reader pays to understand.

**The docs disagree with each other on the same facts.** (As found. The
rows below, `examples/mcp.toml.example`, `auth.md`, and `mcp.md` were
reconciled to the code the same day; the table stays as the record of what
a reader would have hit.)

| Fact | [design.md](design.md) and readme | [todo.md](todo.md) | Code |
| --- | --- | --- | --- |
| Memory scope column | "planned and not in the schema yet" | Done, with test names | `ALTER TABLE memory ADD COLUMN scope` in `builtin.go` |
| Session id | "the constant `george`" | The normalized origin URL | `repoID()` in `root.go`, passed to `ch.SessionID` |
| Ctrl-C | "cancels the process" | Open item 4 | `turn()` in `stdio.go` cancels the turn only |
| Bracketed paste | "a paste is not one message" | Open item 4 | `readMessage` joins the paste |

Add to that: the readme's release links point at `shotah/ai-george`,
the CI badge and module path say `shotah/george`, and the checkout is
named `ai-skid`. `examples/mcp.toml.example` still opens with
`/etc/george/mcp.toml`, `CHANNEL=pendant`, Garmin, Strava, and `george
auth`. `docs/auth.md` and `docs/oauth-catch` describe a flow the binary
does not have. A new reader who starts anywhere but `todo.md` will get
at least one feature wrong.

**The context defaults are the wrong size for the window the eval
runs on.** `HISTORY_MAX_TOKENS=32000` is the whole `OLLAMA_CONTEXT_LENGTH`
the eval sets, before schemas, hydration, the contract, and the current
turn's tool results. The eval's own log says a turn is about 22k prompt
tokens. A long session will hit the model's window before the history
trim fires. The design doc admits this. It is a one-line change that
has been open for a while, and it is the kind of thing that only shows
up as a confusing truncation on the fifth task of the day.

**Safety is the contract plus git.** `shell__command_run` runs as the
user with the repo as cwd. Nothing stops `git push`, `curl | sh`, or an
`rm` outside the root through the shell. `fs` and `git` refuse to
escape `--root`; `shell` does not. A prompt injected through a file the
agent reads has the same tools the user has. The design doc says all of
this, and "git is the undo" is a defensible position for tracked files
in a repo you chose. It is not a position for `.env`, build output, or
anything in `$HOME`. I agree with the previous review here and have
nothing to add except that the `tools-fetch` point above makes the
trust boundary one hop wider than the doc says.

## Would I recommend it?

Yes, to the person who built it and people shaped like them: one local
Qwen, a terminal, a willingness to read a JSON log and a `todo.md`, and
a repo they are fine letting a model dirty. For that person the loop is
better than what they would write in a weekend, the eval is better than
what most agent projects ship at all, and the decisions are documented
well enough to disagree with.

I would also recommend reading `agent.go` and `host.go` even to someone
who never runs it. The name repair, the round collapse, the cycle guard,
and the landing call are each a small, correct answer to a real
local-model failure, and they are portable ideas.

I would not recommend it yet to:

- anyone who needs the graded behaviour to hold across time, until the
  four server tags are pinned and the eval manifest stops saying
  `latest`
- anyone who wants the safety rules to be more than a sentence the
  model usually reads
- anyone who will learn the tool from `design.md` or the readme instead
  of `todo.md`

If this tree were mine, the order would be: commit what is in the
working copy and update the three docs that contradict it; pin
`download_tag` and verify a checksum in `tools-fetch`; move
read-before-patch and no-unasked-commit into the host as refusals;
then write one fixture the contract has never seen and take the 5/5
reading. None of those need a model change.

The harness is the part worth keeping. The prose rules, the floating
tool versions, and the docs that describe last month's tree are the
part I would fix before telling anyone else to install it.
