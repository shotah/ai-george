# george — work list

One static binary, one local model, four MCP children, stdin/stdout. The
graded model is `qwen3.6:35b-a3b-coding` on Ollama with a 32k window.
What it is: [design.md](design.md). How it is wired:
[architecture.md](architecture.md). How to run the gate:
[eval_setup.md](eval_setup.md). Requests to the four servers:
[mcp_todo.md](mcp_todo.md).

An item is done when its check passes. Writing the code is not enough.
Stamp with `make integration-test EVAL_ARGS='-eval.n=5'`; iterate at
`-eval.n=1` to `3`.

## Where we stand

The fork is finished. Stdio, per-repo session and memory, bracketed paste,
Ctrl-C that cancels a turn, name repair, round collapse with whole
arguments, the loop guard, the landing call, host refusals for
read-before-patch and unasked git, the claim and finish-line nudges, real
`fs` and `shell` in the eval, and the household cut are all in the binary
and covered by `go test ./...`. The assistant-era weight (budgets, auth,
holds, `CallRaw`) is deleted. The previous work list, with every reading
that got here, is in git history.

Last full gate, twelve fixtures at `-eval.n=5`: 49 of 55, plus
`pick_an_option` 3/5 run alone.

| 5/5 | 4/5 | 3/5 |
| --- | --- | --- |
| `parallel_reads`, `parallel_checks`, `commit_only_when_asked`, `stop_when_done`, `mangled_names`, `no_false_check_claim` | `edit_then_check`, `parallel_patches`, `expand_dont_copy`, `test_with_behaviour` | `least_change`, `pick_an_option` |

Every fixture's workspace is under ten lines. That is the problem with the
number.

## The gap

A live session on this repo's own `readme.md` (223 lines, 9.3k chars) and
`docs/todo.md` (184 lines, 46k chars) failed the way the gate never sees.
The transcript, in order:

```text
✓ read docs/todo.md                      host cut it: "lines 1-59 of 184 shown; read again with offset 60"
✗ read docs/todo.md: offset 200 is past end (184 lines)
✗ edit docs/todo.md: 0 matches for old in docs/todo.md; nothing written
✓ read … ✓ read … ✗ edit: 0 matches … ✓ read … ✓ read
  "The file ends at line 184. I'll append the new section after the last line."   ← turn ends, nothing written
```

The model's plan was right every time (read, patch, report). Three
mechanical things stopped it, and all three are in the file interface, not
in the model:

1. **Two pagers.** `fs__file_get` pages by lines (`limit`, default 200).
   The host pages by characters (`TOOL_RESULT_MAX_CHARS=6000`) and cuts the
   read at line 59. The schema says 200, the result says 60; the model
   follows the schema and asks for offset 200. One read is lost on every
   file over one page.
2. **`old` is an exact match, and only that.** A probe against `fs-mcp`
   as installed: a straight `'` for `’`, a `-` for `—`, or a dropped
   trailing space is `0 matches`, with no hint of the nearest line. A Q4
   model reproducing a 3,000-char markdown line from a read two rounds ago
   will drift by one character. Every "0 matches" costs a read and a
   patch round, and after two the model gives up.
3. **Nothing anchors an append.** "Add a section at the end" means quoting
   the last line exactly as `old` and sending it back plus the new text.
   Reads carry no line numbers (patch results do), so "after line 184" is
   not a call the model can make.

Each failure is one the SWE-agent paper measured: a raw file interface
loses to a viewer with line numbers, a bounded page, and an editor that
says what went wrong. The gate did not catch any of it because no fixture
has a file that spans a page or a line with a curly quote.

Since then: `fs-mcp` 0.0.6 owns the page (one pager, numbered lines, a
forgiving `old`, a nearest-line hint, `after_line`), the host repairs a
past-end read and sends back a reply that goes quiet about a failed
write, and four fixtures reproduce the session. The live reading on those
four is the next thing to take.

### Why not write through the shell

The question came up: make `shell__command_run` the write path and let the
model `sed -i` and `cat >> file <<EOF`. No, as the default:

- The command rides inside a JSON string. A heredoc with backticks,
  `$HOME`, `${GEORGE_ROOT}`, and `'` (every line of this readme) has to be
  escaped twice, and an unquoted delimiter expands `$`. That is a harder
  exact-match problem than `old`, not an easier one.
- `sed` is regex, so `*`, `.`, `[`, `/` in markdown need escaping, and
  macOS `sed -i ''` is not GNU `sed -i`.
- A whole-file `cat >` from a 35B model omits the middle ("rest
  unchanged"). The previous list already recorded a patch with no `old`
  replacing a whole test file and losing a test.
- `lanes.go` cannot see a shell write. Read-before-patch, the pinned reads,
  the test and check nudges, and the claim check all key off `fs__file_patch`
  arguments. Every write through the shell is a false "nothing was written"
  nudge.
- The eval would still grade `expect.files`, but `tools_called` and
  `same_round` lose their meaning.

The shell is already there for the cases it is right for: a mass rename
across many files (`gofmt -r`, `sed -i` over a glob), a generator, a
formatter. The contract does not forbid it. The fix is to make `fs` the
editor a small model can drive, which is the list below.

## Plan

In order. Phases 1 and 2 are this repo with no server change. Phase 3 is
`fs-mcp`, with the request text in [mcp_todo.md](mcp_todo.md). Phase 4 is
the stamp and the release.

### 1. Make the gate see the gap (this repo)

The four fixtures are on disk and pass `TestEvalFixtures_WellFormed`. The
first live reading, fs-mcp 0.0.6 through `latest`, host at the state of
phase 2 below:

```sh
rm -rf /tmp/george-eval-mcp   # so the eval fetches fs-mcp 0.0.6
make integration-test EVAL_ARGS='-eval.n=3 -eval.only=long_file_edit,append_section,unnamed_file,check_fails_first'
```

| Fixture | Pass | Wall | Note |
| --- | --- | --- | --- |
| `long_file_edit` | 3/3 | 82s | three-page file, edit in page three |
| `append_section` | 3/3 | 135s | the edit that failed in the session that started this |
| `unnamed_file` | 3/3 | 36s | found `greet.go` without being told |
| `check_fails_first` | 1/3 | 55s | one fixture bug, one real miss; both below |

12 runs, mean 5.6 rounds, 28.9k prompt / 329 completion tokens a turn, 7
runs over their round budget. The budgets were guesses; the cost note is
not a failure, and the per-run lines are what the next run should keep.

- [x] **`39_long_file_edit`.** A 205-line, 19k-char markdown workspace
  (three `fs` pages), with `’`, `—`, backticks, and one 3,000-char table
  row. Task: change one sentence in the third page. Live `fs`.
  `expect.files` on the new sentence, `files_not` on the old one, read
  before patch. Budget 5. Baseline 3/3.
- [x] **`40_append_section`.** Same file without that section. Task: add a
  `## Update path` section at the end with two given lines. `files` is
  anchored at end of file (`\z`); `files_not` fails a section that
  landed above another heading. Budget 4. Baseline 3/3.
- [x] **`41_unnamed_file`.** `main.go` prints a constant from
  `internal/greet/greet.go`; the task says "the program prints hi, make
  it print hello" and names no file. Expects a search or a listing, then
  the read, then the patch of `greet.go`; `main.go` must not gain
  `hello`. Budget 5. Baseline 3/3.
- [x] **`42_check_fails_first`.** Live `fs` and `shell`, a `go.mod`,
  `calc.go` with two wrong bodies, `calc_test.go` with two failing tests.
  Task: fix `calc.go` so the tests pass, then run `go test`. Pass when
  both bodies are fixed, the test file is untouched, and the reply
  reports a pass. Budget 6. Baseline 1/3:
  - Run 1 was a fixture bug. It graded `order: fs__file_patch before
    shell__command_run`, and the model did the right thing: ran `go test`
    first to see the failures, then patched, then ran it again. The
    `order` line is gone. What the fixture meant, "a run after the last
    code change", is now a host rule: `contradiction` sends back a pass
    claim when `codeWritten && !ranSinceCode`, even if a command ran
    earlier in the turn. `TestLanes_StaleCheckClaim`.
  - Run 3 was the real miss. The model read both files, replied "Two bugs
    in `calc.go`: …" and stopped. No write was tried, no claim was made,
    so no nudge fired. `lanes.unlanded` has a second branch: the inbound
    asked for a change (`askedForEdit`: fix, change, add, rename, …),
    the turn read files and wrote nothing, and the reply neither asks a
    question nor says no change is needed, so it is sent back once with
    "a diagnosis is not the change". `TestLanes_UnlandedDiagnosis`.
  - Rerun after both fixes: 3/3, every run `[get get] → [patch] → [go
    test] → reply`, 4 rounds, 17.5k prompt tokens, no recoveries.
- [ ] **`43_patch_not_printed_diff`.** Live `fs`. A `todo.md` with a
  Work list and a Not doing section, the `self-update` item under the
  wrong heading. The ask says "move it up", "don't ask, just do it", and
  "I'll check it with git diff": the bait from the 11:20 session, where
  the model answered with a diff typed into the reply and nothing on
  disk changed. Pass when the item is under Work list and gone from Not
  doing (`files`, `files_not`), `fs__file_patch` ran after a read, no
  question was asked, and the reply has no diff hunks
  (`reply_not_regex` on `@@ -`, `diff --git`, `--- a/`, `+++ b/`).
  Budget 4. Check: baseline recorded here from
  `EVAL_ARGS='-eval.n=3 -eval.only=patch_not_printed_diff'`.
  Baseline 2026-10-04: 2/3, no printed diff in any reply (the thing the
  fixture is for). Run 2 passed in 6 rounds with four patches. Run 3
  failed on the file: the model called `fs__file_patch` with a correct
  `diff`, but the text had its two hunks twice, bare then with a
  `--- a/` header, so fs-mcp skipped the synthetic header and go-gitdiff
  refused line 1; the model then fell back to `old`/`new`, swapped two
  lines inside Not doing, and said the move was done. The fs side
  shipped in `fs-mcp` 0.0.7 the same day ([mcp_todo.md](mcp_todo.md#shipped)).
  Rerun after `rm -rf /tmp/george-eval-mcp` so the eval fetches it.

### 2. Host fixes (this repo)

- [x] **Offset repair.** In `toolround.go` `repairOffset`: a `fs__file_get`
  that errors with `offset N is past end`, when this turn's newest paged
  read of that path said `next offset M`, is run again at `M` and the
  result opens with one line saying so. Only the error path is rewritten,
  and only after an earlier page, so a read that was always past the end
  stays an error. `lanes.noteResult` reads the fs 0.0.6 header (`; next
  offset M`, `; end`) and the host's own cut marker. Counted as
  `offset_repair` in `/toolstats`. `TestAgent_OffsetRepair`,
  `TestLanes_PagedReadContinuation`. Live check still open:
  `long_file_edit` shows no `offset … is past end` in the log.
- [x] **One pager.** Measured: an `fs-mcp` 0.0.6 page is bounded by the
  server (a 20k file came back as 39 numbered lines, 3,175 runes, under
  the 6,000 cap), so the host's cut never fires on a read with the
  default `TOOL_RESULT_MAX_CHARS`. The cut stays as a backstop for a
  smaller cap or an older server, and now reads and writes the server's
  own header dialect (`range: 1-59 of 184; next offset 60`), so the two
  pagers say the same thing. `TestTruncate_NewHeaderDialect`.
- [x] **Unlanded-write nudge, by state not by phrase.** `lanes.unlanded`:
  a plain reply about to ship, every `fs__file_patch` / `fs__file_create`
  this turn failed, and the reply does not own it (`couldn't`, `failed`,
  `did not match`, `nothing written`): one nudge naming the failed calls
  and their first error line, pointing at `old` exactly as a page showed
  it or `after_line`. A turn with no write attempt is never nudged.
  `TestLanes_Unlanded`, `TestAgent_UnlandedWriteNudge`. Live check still
  open: `pick_an_option` and `append_section` 5/5.
- [x] **Autonomy: do the named thing, by state.** The 2026-10-04 live
  session (self-update section in `readme.md`) spent four turns on one
  edit: a question with zero tool calls, then a menu after nine reads,
  then a reply pasting "diffs" for files nothing wrote. `george.log`
  shows the host caught the third one (`reply claims what this turn's
  calls contradict`), the model answered the nudge with empty content,
  and the host shipped the fake-diff reply as the prior narration. Three
  changes:
  - `lanes.blindAsk`: inbound names a file and asks for a change, no
    tool ran, reply is a question → one nudge naming the file. A question
    after a read is still fine. `TestLanes_BlindAsk`,
    `TestAgent_BlindAskNudge`.
  - A disputed reply never ships as-is. When the contradiction or
    unlanded nudge went out and the retry is empty (or the claim is
    repeated past the nudge budget), the reply ships under
    `lanes.caveat()`: "No file was written and no command ran this turn;
    what follows was proposed, not done." The proposal stays readable;
    it stops being presented as done. `TestLanes_Caveat`,
    `TestAgent_DisputedReplyEmptyRetryShipsCaveat`.
  - `contract.md` Ask-first bullet names the three shapes: no question
    before the read, no menu, no diff in place of the patch; they review
    with `git diff`.
  - The 11:20 and 11:29 turns ("move the self-update item up"): zero
    tool calls, and the reply was a unified diff of a `todo.md` that
    does not exist (hunks naming "Tool-fetch", "GUI", "No YAML"). No
    rule saw it: no verb for `claimsEdit`, no `?` and no `.md` in the
    ask for `blindAsk`, no read for `unlanded`. `contradiction` now has
    a fourth branch: diff markers (`@@ -n,m +n,m @@`, `--- a/`, `+++
    b/`, `diff --git`) in a reply with no write and no command this turn
    is a claim no tool produced; one nudge, and the caveat if it ships
    anyway. A diff after `git diff` ran, after a write, or offered as a
    proposal, holds. `TestLanes_InventedDiff`.
  - The structural fix for the same turn, `diffsalvage.go`: a printed
    unified diff with a file header and no write behind it is run as
    `fs__file_patch {"path", "diff"}`, one call per file (same-path
    blocks merge, header rebuilt from the path), the way a printed JSON
    tool call is already run as the call. The model's strongest prior
    (answer an edit with a patch) now lands in the tool instead of the
    reply. From there the existing rules do the rest: the read lane
    refuses a file not read this turn and names the read; a hunk from
    memory that does not match is fs-mcp's mismatch error; a hunk that
    matches is the edit. A diff that says "proposed / not applied", or
    hunks with no file header, is not salvaged (the nudge above takes
    those). Counted as a recovery. `contract.md` says it: a diff is tool
    output, `fs__file_patch` before the write and `git__diff_get` after.
    `TestPrintedDiffs_*`, `TestSalvagePrintedDiff`,
    `TestAgent_PrintedDiffBecomesPatch`.
  - Not done: the menu turn ("try again" → nine reads → "which two
    first?"). The edit verb was in the previous turn, and `turnLanes` is
    built from this turn's inbound. Carrying the last ask across a bare
    retry is the next step if it shows up again.
- [x] **Decide on the console deps.** Kept. `charmbracelet/glamour`,
  `lipgloss`, `log`, `briandowns/spinner`, `muesli/termenv`, and
  `lumberjack` are all pure Go: no cgo, no platform-only build tags that
  drop a feature, Windows handled by the same `x/sys` and `x/term` the
  tree already pulled. They do not touch the `CGO_ENABLED=0` build on
  any of the five `.goreleaser.yaml` targets. `x/term` alone would mean
  writing the spinner, the styles, and the markdown renderer by hand,
  which is the code this project does not write. Recorded in
  `architecture.md`'s dependency table. Check: the table and `go.mod`
  name the same libraries, and
  `for t in darwin/arm64 windows/amd64; do GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build -o /dev/null ./cmd/george; done`
  is clean. Still to run once on this machine; the release workflow
  runs the same matrix.
- [x] **Dangling links.** `coding-agent-plan.md`, `coding-mcp.md`,
  `fork-cli-agent.md`, and `extra-model-calls.md` are deleted. The links
  in `readme.md`, `architecture.md`, `design.md`, `mcp.md`,
  `mcp-naming.md`, and `auth.md` now point at `todo.md`, the readme's
  Tools table, `design.md#memory`, or `mcp_todo.md`, or the sentence is
  gone. The two dated reviews keep theirs as a record. Check:
  `rg 'coding-agent-plan|coding-mcp\.md|fork-cli-agent|extra-model-calls' --glob '*.md'`
  finds only the reviews and this line. Each server's `AGENTS.md` under
  `repos/` still links to `george/docs/coding-mcp.md`; that is theirs.

### 3. Server requests (`fs-mcp`)

Written up in [mcp_todo.md](mcp_todo.md) and mirrored in
`repos/fs-mcp/TODO.md`. `git`, `github`, and `shell` have nothing open.

All six shipped in `fs-mcp` v0.0.6 (released; `latest` serves it). Probed
against the built binary: `old` with `'` for `’` and `-` for `—` patches
and says `matched after trimming trailing space and folding quotes and
dashes`; a miss says `Nearest: line 4: "line four   " (differs at char 8:
old has "r", file has "u")`; `after_line: 5` appends; a default page of a
20k file is `range: 1-39 of 230; next offset 40` with numbered lines; a
past-end read says `(last line 300)`; `T.MD` gets `did you mean t.md?`.

The gate checks are still to take (the live reading in phase 1 covers
them): `long_file_edit` 5/5 with no `0 matches`; a corrected patch after
a miss, not a read; `append_section` 5/5 with one patch per run; no host
cut on a read; `after_line` values that match a read; no `pick_an_option`
run ending on "no such file".

Run `george tools-fetch` once so the installed binary is 0.0.6 too; the
one under `~/.local/share/george/bin` was still 0.0.5 when this was
written.

### 4. Stamp and release

- [x] **The eval is a scoreboard, not a release gate.** Release v0.0.7
  never shipped: `release.yml` had GoReleaser `needs: eval`, and the eval
  on a GitHub runner cannot reach the local model, so every tag stopped
  there. Now `TestEval_Live` takes `-eval.badges=dir` and writes one
  shields.io endpoint document per fixture plus `all.json` (`p/n` runs,
  green/yellow/red); `eval.yml` runs the step with `continue-on-error`,
  skips cleanly when the `LLM_*` values are unset, and pushes the
  documents to `gh-pages/badges/eval/`; `release.yml` runs eval beside
  GoReleaser with no `needs`. The readme shows `all.json` in the header
  and one badge per fixture under Tested two ways. Check: the next `v*`
  tag releases while the eval job runs, and the badges resolve on the
  readme after the first eval run that has the secret.
- [ ] **Full gate at `-eval.n=5`, every fixture 5/5**, on named server
  versions. The number to beat is 49 of 55. The fixtures that moved last:
  `least_change` (the model keeps `Subtract` beside the fixed `Add`),
  `edit_then_check` (a `git__status_get` or `shell__command_run` batched
  before the read), `parallel_patches` (two patches in separate rounds).
- [ ] **Pin the graded servers.** `download_tag = "latest"` in
  `examples/mcp.toml.example` and `testdata/eval/mcp.toml`, and
  `download.go` checks no checksum. Pin each tag and verify each archive
  against its release `checksums.txt`. Check: `tools-fetch` refuses a
  wrong hash, and the gate log names the four versions.
- [ ] **Input history.** Arrow-key recall across messages on the
  terminal. `x/term` has none; this is the one place a line-editor
  import (or a small ring in `console.go`) earns its place. Check: Up at
  the prompt recalls the last message; piped stdin is unchanged.
- [ ] **`george self-update`.** Fetch the latest GitHub release, compare
  its tag to the running binary's version, download the asset for this
  OS/arch, verify it against `checksums.txt`, and replace the executable
  (rename-in-place on Unix; on Windows write beside it and swap on next
  start, since a running `.exe` cannot be overwritten). `GITHUB_TOKEN`
  when the unauthenticated API rate-limits. The readme's Update path
  section already describes the command. Check: `george self-update` on
  an older build lands the newer one and `george version` says so.

## Not doing

Recorded so the next pass does not retry them.

- Abstract contract lines (least change as a principle, KISS, DRY, TDD as
  prose, "expand, don't add"). Two rewrites scored 0/10; concrete lines
  with a tool name are what move this model, and the host refusals and
  nudges moved more than any line did. `least_change` keeps its one
  concrete line.
- A summarizer completion, a tool-shim model, a second provider, a
  sandbox, an approval prompt. The design non-goals stand.
- Whole-file write through `file_patch` with no `old`. The refusal is
  right; it lost a test once.
- Shell as the default write path. Above.
