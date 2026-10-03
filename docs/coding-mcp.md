# Coding-agent MCP plugins

Work list for the local coding loop. Names follow [mcp-naming.md](mcp-naming.md). Host behavior is [mcp.md](mcp.md).

The harness stays the harness. Design already leaves workspace tools to MCP binaries. The checkouts under `repos/` are the personal-assistant set (google, garmin, strava, youtube, beam, math, gemini-search, feeds, twitter, maps, image, pendant, boards, health, rentals, fleet). None of them reads a tree, patches a file, or runs a command. Those are new repos. They do not land in this process, and they are not features added to those plugins.

## Four servers

The edit loop is one server. Git is a second. Shell is a third. GitHub is the fourth.

| Server id | Binary | Repo | In the default grant |
| --- | --- | --- | --- |
| `fs` | `fs-mcp` | new `fs-mcp` | yes, `force = true` |
| `git` | `git-mcp` | new `git-mcp` | yes, `force = true` |
| `shell` | `shell-mcp` | new `shell-mcp` | yes, `force = true` |
| `github` | `github-mcp` | `repos/github-mcp` | yes, `force = true` |

One process for all of that would boot-fail together. Fail-soft is per server: a directory that is not a git repo must still read and patch. Shell is the grant you drop. GitHub needs a token and the network; the inner loop does not.

A server per verb (`grep`, `diff`, `patch`, `write`) splits one edit across prefixes. Schemas for a prefix show up after `mcp_enable`. Search and patch belong on `fs` with read and create.

These four stay `force = true`. The schemas stay published, which is cheaper than an enable round-trip before every edit. GitHub needs a token and the network; a missing `GITHUB_TOKEN` skips that server and leaves the others up.

Showing a diff and applying a diff are different tools on purpose. `git__diff_get` shows the repo. `fs__file_patch` writes a hunk. The host ranks closest names by tokens and strips generic verbs (`get`, `list`, `search`, `create`, `update`, `delete`, `add`). `update` on the edit tool would score 0 against a call of `apply_patch`. The token that survives is `patch`.

## `fs` — files

Server id `fs`. Tools do not start with `fs`. Core tier is five tools. A miss that only shares the token `file` ties every `file_*` tool, and the host returns at most five names. A sixth `file_*` tool pushes one of those names off the hint.

| Tool | Host call | Description starts |
| --- | --- | --- |
| `file_list` | `fs__file_list` | List entries in one directory. |
| `file_get` | `fs__file_get` | Read one text file by path. |
| `file_search` | `fs__file_search` | Search file contents (grep) under a path. |
| `file_create` | `fs__file_create` | Create one new file. |
| `file_patch` | `fs__file_patch` | Apply a unified diff (patch) to one existing file. |

`file_delete` ships in the binary behind `--tool-tier write`. The coding-agent manifest stays on `core` until a fixture needs delete. One path, no globs.

Args are snake_case: `path`, `query`, `glob`, `diff`, `offset`, `limit`.

- `--root` is the jail. Refuse a path that escapes it, including symlink escape.
- `file_list` is one directory, depth 1. A recursive listing blows the host result cap.
- `file_get` returns text. A size cap, plus `offset` / `limit`. A binary is a one-line marker, not bytes in the prompt.
- `file_search` returns hits (`path`, `line`, `text`), not whole files.
- `file_patch` is one file per call. The hunk must match or the call fails and changes nothing. The model fans out several files as one parallel batch. There is no multi-file apply tool.
- `file_create` refuses when the path exists. Overwrite is `file_patch`.

## `git` — local repo

Server id `git`. Tools do not start with `git`. No network in v1: no push, pull, fetch, or anything that dials a remote.

| Tool | Host call | Description starts |
| --- | --- | --- |
| `status_get` | `git__status_get` | Show the working tree status. |
| `diff_get` | `git__diff_get` | Show the working tree diff, or the diff for one revision. |
| `commits_list` | `git__commits_list` | List recent commits (git log). |
| `stage_update` | `git__stage_update` | Stage paths (git add) for the next commit. |
| `commit_create` | `git__commit_create` | Create a commit from the index. |

Same `--root` as `fs`. Git runs only in that tree. No `-C` outside it.

`commits_list` keeps the stable verb `list`. Closest-match will see the token `commits` and will miss a bare `log`, because `list` is stripped and descriptions are not scored. The schema is on every turn (`force = true`), and the description says “git log”. One name. If the pinned model emits `log` anyway, rename once and bump semver. Do not also register `log_list`.

`stage_update` has the same shape: the token is `stage`. The description carries “git add”. `add` is a stripped token, so it cannot be the thing that makes the name rank.

Leave these unregistered in v1: `branches_list`, `branch_create`, `commits_search`, push, pull, fetch, rebase, reset, checkout. A tool that can push will be called.

`commit_create` takes `message` and commits the index. It does not stage, and it does not push. The description says to call it when the user asked for a commit.

## `shell` — one command

Server id `shell`. One tool.

| Tool | Host call | Description starts |
| --- | --- | --- |
| `command_run` | `shell__command_run` | Run one shell command in the workspace and return stdout, stderr, and the exit code. |

`run` is a domain verb, same kind of exception as `beam` and `generate`. It is not in the host’s generic-verb set, so `run_command` shares both tokens with `command_run`. Do not also register `command_create` or `test_run`. Tests, builds, and linters are commands.

- Cwd is `--root`. Unlike `fs` and `git`, that is a working directory, not a jail: a command can name any path.
- One command string. A timeout. The result leads with the exit code and the tail of the output, so a test failure survives truncation.
- There is no sandbox. The command runs as you, and git is the undo ([design.md](design.md#security)). This server does not grow an allowlist of binaries.

## `github`

The checkout is `repos/github-mcp`. It is in the default manifest. A missing token skips this server.

Server id `github`. Tools do not start with `github`, and they do not use nouns already taken.

| Tool | Host call | Notes |
| --- | --- | --- |
| `issues_list` | `github__issues_list` | |
| `issues_get` | `github__issues_get` | |
| `issues_search` | `github__issues_search` | |
| `issues_create` | `github__issues_create` | |
| `pulls_list` | `github__pulls_list` | |
| `pulls_get` | `github__pulls_get` | |
| `pulls_create` | `github__pulls_create` | |
| `checks_list` | `github__checks_list` | |
| `account_get` | `github__account_get` | Shared noun with flights / rentals / cars. The host prefix disambiguates. |
| `contents_get` | `github__contents_get` | Remote file body. `file_*` belongs to `fs`. |

No `diff_*` (that is `git`). No `link_*` (maps, flights, rentals, cars). No second client for local commit or stage.

## Manifest shape

Binaries on `PATH`. `fs`, `git`, and `shell` share `--root`. `github` does not.

```toml
[[server]]
name  = "fs"
command = "fs-mcp"
args  = ["--root", "${GEORGE_ROOT}", "--tool-tier", "core"]
force = true

[[server]]
name  = "git"
command = "git-mcp"
args  = ["--root", "${GEORGE_ROOT}", "--tool-tier", "core"]
force = true

[[server]]
name  = "shell"
command = "shell-mcp"
args  = ["--root", "${GEORGE_ROOT}"]
force = true

[[server]]
name    = "github"
command = "github-mcp"
force   = true
```

## Names that will get invented

Closest-match is name tokens only. Descriptions are for the model reading the schema, not for the ranker. Core `fs` stays at five `file_*` tools so a tied `file` hint still lists the whole set.

| Call the model may emit | Where it should land |
| --- | --- |
| `fs__file_read`, `read_file` | `fs__file_get` (token `file`; description says “Read”) |
| `grep_files`, `fs__file_grep` | `fs__file_search` |
| `apply_patch`, `fs__patch_update` | `fs__file_patch` (token `patch`) |
| `git__git_status`, `status` | `git__status_get` |
| `git_diff`, `diff` | `git__diff_get` |
| `shell__run_command` | `shell__command_run` |

No dual registration. Tests in each repo assert every tool matches `^[a-z]+_[a-z]+` and the first token is not the server id.

## Work

Each box is a new repo, or a change in this repo that only grants one. Package TODOs link back to [mcp-naming.md](mcp-naming.md) and to this file.

- [x] `fs-mcp`: the five core tools, `--root`, `--tool-tier core`, name tests. Checkout: `repos/fs-mcp`.
- [x] `git-mcp`: the five core tools, repo-only, no remotes, name tests. Checkout: `repos/git-mcp`.
- [x] `shell-mcp`: `command_run` only, timeout, exit code, tail of output. Checkout: `repos/shell-mcp`.
- [x] Grants in `examples/mcp.toml.example` (what `george init` writes) and `internal/agent/testdata/eval/mcp.toml`, including `github`. Each `download_tag` is `latest`.
- [x] One live eval fixture on the pinned model: `fs__file_get`, then `fs__file_patch`, then `shell__command_run`. Fixture `edit_then_check`. Every run passes, in that order. A unit test that only pastes the name does not count.
- [x] Add `git__status_get` to that fixture after `git-mcp` is granted.
- [x] `github-mcp`: ten tools, `GITHUB_TOKEN`, name tests. Checkout: `repos/github-mcp`. Granted with `force = true`.

## Done when

- [x] A non-git directory still lists and patches files, and the git server is the one that skips.
- [x] Omitting `shell` from `mcp.toml` removes `command_run` and leaves `fs` and `git` up.
- [x] The live log for the fixture shows those host names and no `file_update`, `grep`, `git_status`, or `run_command` registration.
- [x] This process still has no workspace tools of its own.
