# MCP server changes george needs

Requests for the four coding servers. Each item names the server, what the
model did, the change, and the check that proves it from this repo with no
change here. Names follow [mcp-naming.md](mcp-naming.md). The servers'
checkouts are under `repos/` (gitignored); each has its own `TODO.md` that
mirrors the open items here.

Run a check with `make integration-test EVAL_ARGS='-eval.n=3 -eval.only=<fixture>'`.
The eval fetches each server's `latest` release once and caches it; clear
`/tmp/george-eval-mcp` after a release.

## Open

Nothing. Every request below shipped. The live checks still to take are
listed with the fixtures in [todo.md](todo.md#1-make-the-gate-see-the-gap-this-repo).

## Shipped

On `fs` 0.0.7, `git` 0.0.3, `shell` 0.0.2, `github` 0.0.3. "Seen" is a
gate run or a hand probe of the built binary that hit the change.

| Server | What | Seen |
| --- | --- | --- |
| `fs` | `file_patch` `old`/`new` with optional `count` (the rename fix) | `parallel_patches` 5/5 |
| `fs` | Required-argument errors show a valid call (`path is required, e.g. {"path":"a.go"}`) | Gate; fixed the next round |
| `fs` | A patch result echoes the changed lines with numbers | Gate |
| `fs` | `hunk does not match` shows the real lines and where the context appears elsewhere | Not yet; the model sends `old`/`new`, not diffs |
| `fs` | Diff shape errors name the problem (wrong file, repeated file, bad `@@`) | Not yet, same reason |
| `fs` | A leading `/` means the root | Not yet; no run has sent one since |
| `fs` 0.0.6 | Forgiving `old`: exact, then trailing whitespace, then a fold of curly quotes, dashes, NBSP; the result names the level | Probe: `'`/`-` for `’`/`—` patched, "matched after trimming trailing space and folding quotes and dashes" |
| `fs` 0.0.6 | A miss names the nearest line and the first differing character | Probe: `Nearest: line 4: "line four   " (differs at char 8: old has "r", file has "u")` |
| `fs` 0.0.6 | `after_line` + `new` inserts; `0` is the top, the last line appends | Probe: `after_line: 5` on a 5-line file appended |
| `fs` 0.0.6 | A page is at most `limit` lines and `max_chars` (default 6000); header `range: 1-39 of 230; next offset 40`, last page `; end` | Probe: a 20k file paged at 39 lines, 3,175 runes |
| `fs` 0.0.6 | Every read line is `N: text`; the header is on every read | Probe |
| `fs` 0.0.6 | Missing file with one case-insensitive match: `did you mean t.md?` | Probe |
| `fs` 0.0.7 | A `--- `/`+++ ` header after the first `@@` is not the file header: the synthetic header is added when no header precedes the first hunk, and header pairs are counted like `diff --git` so a repeated file still answers `one file per call` | `patch_not_printed_diff` on 0.0.7: a diff with the header five times got `one file per call`, the right answer. 0.0.6 had refused line 1 of a bare-then-headed diff with `patch fragment without file header` |
| `git` | `stage_update` says "Call only when the user asked to stage or commit" | `commit_only_when_asked` 5/5 |
| `github` | `GITHUB_TOKEN is not set` on the server's line, other three stay up | Gate |
| `shell` | Exit code first, then the last 4,000 chars of stdout+stderr | Every run |

The 0.0.6 items came from one live session on `qwen3.6:35b-a3b-coding`
editing this repo's `readme.md` and `docs/todo.md`: every patch missed on
an exact-match drift, every second read asked for the wrong offset, and the
turn ended with nothing written. The session and the host-side half (offset
repair, one pager, the unlanded-write nudge) are in
[todo.md](todo.md#the-gap).

## Not changing

- Whole-file write through `file_patch` with no `old`. A patch with `new`
  alone once replaced a whole test file and lost a test; the refusal is
  right.
- A distance (Levenshtein) match for `old`. It can land on the wrong line,
  and a wrong patch costs more than a miss.
- A sixth `file_*` tool. The host suggests at most five names on a near
  miss; new modes go on `file_patch`.
