# MCP server changes george needs

Requests for the four coding servers. Each comes from a live gate run on `qwen3-coder:30b-a3b-q4_K_M` with the real `fs`, unless the item names another model (see [eval_setup.md](eval_setup.md)). Each item names the server, what the model did, the change, and the check that proves it from this repo with no change here. Names follow [mcp-naming.md](mcp-naming.md); the tool sets are [coding-mcp.md](coding-mcp.md).

Run a check with `make integration-test EVAL_ARGS='-eval.n=3 -eval.only=<fixture>'`. The eval fetches each server's `latest` release once and caches it; clear `/tmp/george-eval-mcp` after a release.

Status, on `fs` 0.0.5, `git` 0.0.3, `shell` 0.0.2, `github` 0.0.3:

- Seen working in gate logs: 1 (the model renames with `old`/`new`, `parallel_patches` 5/5), 2 (`new is required, e.g. …`, fixed the next round), 3 (patch results now echo the change), and 6 (`commit_only_when_asked` 5/5, no `stage_update`).
- Shipped, but no gate run has hit it yet: 4 (hunk mismatch lines), 5 (diff shape errors; the model now uses `old`/`new`, not diffs), 7 (`GITHUB_TOKEN` message), and 8 (a leading slash means the root).

## `fs-mcp`

### 1. Replace mode on `file_patch` (most important)

**What happens.** In a rename, the model renames the declaration and leaves the call as a context line in its diff. In `parallel_patches` it writes `-func TestHello` / `+func TestGreet` and keeps ` \tif Hello("bob")` unchanged. That one miss fails most runs. Prompt rules didn't fix it: they worked once, then didn't, and a concrete diff example made it worse.

**Change.** Give `file_patch` a second mode: `path`, `old`, `new`, and optional `count`. It replaces exact occurrences of `old` with `new` in one file. With no `count` it replaces all of them; with `count` it requires exactly that many. Zero matches writes nothing and says so. `diff` stays as it is, and exactly one of `diff` or `old`/`new` is required.

Expand `file_patch` instead of adding a `file_replace` tool. The host suggests at most five names, and a sixth `file_*` tool pushes one of the five off that list ([coding-mcp.md](coding-mcp.md)).

**Check.** `parallel_patches` reaches 3/3, with the model sending `old`/`new` for the rename.

### 2. Teach the call in the "required" errors

**What happens.** When the model is unsure, it sends `fs__file_get {}` or `fs__file_patch {}`, gets `tool error: path is required`, and sends `{}` again. One gate hit that error 18 times.

**Change.** Make every missing-argument error show a valid call: `path is required, e.g. {"path":"a.go"}`. For `file_patch`, show both modes.

**Check.** Across one full gate at `-eval.n=3`, no run sends the same empty call twice in a row.

### 3. Show the result of a patch

**What happens.** `file_patch` returns a short line (13 to 20 characters). The model often calls `fs__file_get` again just to see what it wrote, which costs a round, and sometimes re-patches.

**Change.** On success, return the changed lines with two lines of context on each side, with line numbers. Cap the output at about 40 lines.

**Check.** In `edit_then_check`, the mean number of rounds drops, and no run reads the file again after a patch that applied.

### 4. Show the real lines when a hunk doesn't match

**What happens.** `tool error: hunk does not match: conflict: fragment line does not match src line`. The model has to read the whole file again to find out what is actually there.

**Change.** Add the file's actual lines at the hunk's position, with line numbers, to the error. If the hunk's first context line appears elsewhere in the file, name that line number.

**Check.** After a `hunk does not match` error, the next call is a corrected patch, not a read.

### 5. Name the problem when the diff is the wrong shape

**What happens.** On `qwen3.6:35b-a3b-coding`, the renamed lines are right, but the diffs are malformed. One `file_patch` call with `"path":"a.go"` carried `--- a/a_test.go` sections, once with the same file five times. A missing final newline came back as `hunk does not match: unexpected EOF`. Others came back as `gitdiff: line 70: invalid fragment header` and `file creation fragment contains context or deletion lines`. Each error cost a full read-and-patch-again round. `parallel_patches` took 7 to 8 rounds against a budget of 3.

**Change.** Check the diff's shape before matching it, and say what is wrong:

- A `---`/`+++` file that is not `path`: `diff is for a_test.go; path is a.go. One file per call.`
- The same file more than once: `diff repeats a.go`.
- A file with no final newline: apply the patch anyway, without requiring `\ No newline at end of file`.
- A bad `@@` header: name the line and show the expected form, `@@ -4,6 +4,6 @@`.

**Check.** On `qwen3.6:35b-a3b-coding`, `parallel_patches` stays within its budget of 4 rounds (read, patch, test, reply).

### 8. A leading slash means the root

**What happens.** In `mangled_names`, the retry after an unknown tool name sent `fs__file_get {"path":"/c.txt"}` and got `tool error: path escapes workspace`. The file is `c.txt` at the root. It cost a round, and the model then fell back to the mangled spelling that had worked.

**Change.** Read a path that starts with `/` and is not under `--root` as relative to the root (`/c.txt` is `<root>/c.txt`). An absolute path that is under the root stays as it is. `..` that leaves the root is still refused.

**Check.** `mangled_names` at `-eval.n=5` shows no `path escapes workspace` error. Shipped in `fs-mcp` 0.0.5. The first reading was 5/5 with no such error, but no run sent a leading slash, so the fix itself hasn't been hit yet.

## `git-mcp`

### 6. `stage_update`: only when asked

**What happens.** Unasked `git__stage_update` then `git__commit_create`, in nearly every live gate; one gate had 12 `stage_update` calls. `commit_create` already says "Call only when the user asked for a commit." `stage_update` says only "Stage paths (git add) for the next commit," and the model stages first.

**Change.** Make the `stage_update` description say: "Call only when the user asked to stage or commit."

**Check.** At `-eval.n=5`, `edit_then_check` and `commit_only_when_asked` show no `git__stage_update` call.

## `github-mcp`

### 7. Say why it didn't start without a token

**What happens.** Without `GITHUB_TOKEN`, the process exits before the handshake. The host logs `initialize: EOF`, which doesn't say what's missing.

**Change.** Before exiting, write one line to stderr naming the variable: `GITHUB_TOKEN is not set`. Better still, finish the handshake and serve no tools, with the reason in the server's `instructions`.

**Check.** `george run` with no token logs the reason on the `github` server's line, and the other three servers stay up.

## `shell-mcp`

Nothing yet. `command_run` behaved in every gate run.
