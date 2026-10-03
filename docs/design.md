# Design

Harness contract: principles, env, agent loop, memory, ops, packaging,
security. Pitch and hello path: [root readme](../readme.md). Diagrams:
[architecture.md](architecture.md). What was cut and what is still open:
[coding-agent-plan.md](coding-agent-plan.md).

george is the runtime around one local model so a coding turn finishes.
The model predicts tokens. The harness makes a tool call land, a bad name
resolve, and the reply come back on stdout.

```text
process = contract + persona + one model + fs/git/shell/github + one sqlite file
```

You run it in a repo. It edits that tree and exits when you quit. A second
model or a second persona is a second process. No in-process routing, no
dashboard.

## Principles

1. **Stupid simple.** One agent, one model, stdin/stdout. If a feature needs
   a diagram to explain, it belongs in an MCP binary.
2. **Progress per Completer call.** The standing prompt is re-billed every
   round. Independent reads and patches go out as one batch. The last two
   tool rounds stay whole; older rounds collapse in-process, with no extra
   completion.
3. **Highly portable.** `CGO_ENABLED=0` static binary. No CGO, no glibc
   dependency in our binary.
4. **Plugin-centric.** Workspace read, patch, git, and shell are MCP
   children. The harness hosts them. Builtins are memory, `self_note`, and
   `web_search`. Import a library over writing one.
5. **1:1.** One process, one conversation (`george`), one model endpoint.
   No multi-provider config, no multi-agent config, no peer routing.
6. **Env + files is the config plane.** Keys in `~/.config/george/env`.
   Structure in persona markdown, `mcp.toml`, and the data directory. The
   process environment wins over the env file.
7. **Memory is structured and inspectable.** SQLite rows you can read and
   delete with `sqlite3`. No embeddings. Repo files outrank recalled rows.
   The contract outranks `PERSONA.md`.

## Who it’s for

Someone who wants `cd repo && george` against a local OpenAI-compatible
model (the eval grades `qwen3.6:35b-a3b-coding` on Ollama). The human
types a task. The agent reads, patches, runs the check, and reports.

Anti-fit: a phone assistant, a web UI, a team workspace, inbound webhooks,
a control UI, skills directories, subagents. A yard console for several
agents is [gantree](https://github.com/shotah/gantree).

## Non-goals

- Web dashboard, gateway, REST/WS API, pairing flows, inbound port
- Multi-agent / multi-provider / model fallback chains
- A second model for tool shims, titles, or summarising each tool pair
- Built-in workspace tools (those are `fs-mcp`, `git-mcp`, `shell-mcp`, `github-mcp`)
- Vector DB / embedding service
- An in-process sandbox or an approval prompt (git is the undo)
- Clock-driven work: cron, watches, a daily planner, proactive pings

Streaming is on: `STREAM_REPLIES=true`.

## Local-model hardening

Most agent stacks assume a frontier model and a huge catalog. This harness
is hardened where a local coder model actually fails. The same levers cut
prompt tokens, because schemas, history, and tool results are re-billed
every round.

| Lever | What we do | Why it matters |
| --- | --- | --- |
| Tool surface | `force = true` on `fs` `git` `shell` `github`; other prefixes wait for `mcp_enable` | The edit schemas are on every turn; the rest stay off until asked |
| Name repair | Hyphenated prefix, wrong separator (`fs.file_get`), unique bare name, then at most five closest names and one grammar-constrained retry | A typo does not cost a whole extra guess when the host can see the answer |
| Printed calls | Parse a tool call written as text and run it | A model that prints `{"name":…}` still calls the tool |
| Landing call | At `TOOL_MAX_ITERATIONS` the next call has no tools | The turn ends in a reply, not an error that drops the work |
| Round warning | Past ~70% of the budget, one note says how many rounds remain | The model can wrap up before the landing call |
| Tool collapse | Last two rounds stay whole, including a parallel batch; older rounds become a one-line marker | A long edit does not fill the window with old file bodies |
| Memory | SQLite + FTS5 in-process | No embedding call before the reply |
| Taste | `SELF.md` + `self_note` + distill on `/new` | How you like a change shaped survives the reset |
| Runtime | One static binary on `PATH` | No gateway process in front of the model |

MCP tools share one `{server}__{tool}` name and one repair path. Details:
[mcp.md](mcp.md). The four coding servers: [coding-mcp.md](coding-mcp.md).

The graded socket is the local one; Gemini support was dropped.
`LLM_SYSTEM_FOLD=auto` (the default) folds every system block into one leading
system message. Ollama's `qwen3-coder` renderer keeps only the first system
message and silently drops the rest, so the layout with system text after the
user turn (`many`) hides the harness, budget, and loop notes from that model.
The `qwen3.5` renderer behind `qwen3.6:35b-a3b-coding` does read a later
system message; the gate runs folded on it anyway.

## Progress per invocation

A Completer invocation is the expensive unit. The completed edit is what
we optimize.

```text
context → next state → fan out tools → read the results → repeat
```

| Piece | Job |
| --- | --- |
| Short contract | How the work is done, without eating the window |
| SQLite memory | Don't rediscover this repo's commands every round |
| Caps, collapse, `mcp_enable` | Enough context for the next edit, not the whole catalog |
| Parallel tool batches | Several independent reads, or one `file_patch` per file, between Completer calls |
| MCP | The action surface stays out of the Go binary |
| Go runtime | A batch actually runs concurrently (one stdio child still serializes its own calls) |
| `SELF.md` | Taste survives `/new` |
| `/perf` | Invocations, tools, max batch, recoveries, prompt/gen estimates, wall time, outcome |

## Configuration contract

Scalars are env. Structure is files. No config UI. Boot is fail-fast:
missing `LLM_BASE_URL`, `LLM_API_KEY`, or `LLM_MODEL` exits 1. A variable in
`config.Retired` (`CHANNEL`, `TELEGRAM_*`, `CRON_*`, `WATCH_*`,
`MEMORY_CONSOLIDATE_MINUTES`, `COALESCE_SETTLE_MS`, and the rest) also
exits 1. An old env file should fail, not quietly do nothing.

`config.Load` reads `~/.config/george/env` first. A variable already set in
the process environment wins. `GEORGE_CONFIG_DIR` overrides the config
directory.

### Environment variables

| Var | Required | Example / default |
| --- | --- | --- |
| `LLM_BASE_URL` | yes | `http://127.0.0.1:11434/v1` |
| `LLM_API_KEY` | yes | any non-empty string for a local server that ignores it |
| `LLM_MODEL` | yes | `qwen3.6:35b-a3b-coding` |
| `LLM_MAX_TOKENS` | no | `4096` (completion output cap, including tool-call args; `0` = provider default) |
| `LLM_REASONING_EFFORT` | no | empty (Ollama/Qwen: `none` so max tokens are not eaten by hidden chain-of-thought) |
| `LLM_SYSTEM_FOLD` | no | `auto` (same as `one`: every system block folded into one leading message). `many` posts the layout with `[harness]` after the user, for a server that renders every system message |
| `GEORGE_CONFIG_DIR` | no | `~/.config/george` |
| `GEORGE_ROOT` | no | git toplevel of the cwd |
| `PERSONA_DIR` | no | `GEORGE_CONFIG_DIR` |
| `DATA_DIR` | no | `~/.local/share/george` (`XDG_DATA_HOME`) |
| `MCP_MANIFEST` | no | `$GEORGE_CONFIG_DIR/mcp.toml` |
| `HISTORY_MAX_MESSAGES` | no | `200` |
| `HISTORY_MAX_TOKENS` | no | `32000` (chars/4 estimate; older turns fold into `Facts:` / `Voice:`) |
| `HISTORY_STRIP_FILLERS` | no | `true` (prompt-only; last 40 messages verbatim; assistant never stripped) |
| `TOOL_RESULT_MAX_CHARS` | no | `6000` |
| `TOOL_MAX_ITERATIONS` | no | `25` (then one no-tools landing call) |
| `TOOL_SCHEMA_MAX_TOKENS` | no | `0` (log the estimate; `>0` fails boot if the catalog is over) |
| `TOOLS_ENABLED` | no | `true` (`false` omits every tool schema) |
| `WEB_SEARCH_ENABLED` | no | `true` (builtin `web_search`; a leftover `google-search` grant is omitted) |
| `BRAVE_SEARCH_API_KEY` | no | Brave Search token — titles, URLs, snippets; off without it |
| `MCP_ENABLE_FORCE` | no | comma-separated prefixes always published when `dynamic_tools` is on (the manifest's `force = true` is the other way) |
| `SELF_NOTES_ENABLED` | no | `true` (off when `PERSONA_DIR` is not writable) |
| `MEMORY_ENABLED` | no | `true` |
| `MEMORY_BACKEND` | no | `builtin` (or `mcp:<server-name>`) |
| `STREAM_REPLIES` | no | `true` |
| `SHOW_THINKING` | no | `true` (needs `STREAM_REPLIES`) |
| `TOOL_TRACE` | no | `compact` (`compact`\|`full`\|`off`; needs `STREAM_REPLIES`) |
| `SPINUP_NOTICE_MS` | no | `4000` (`0` = off; a still-working line before the first token) |
| `LOG_LEVEL` | no | `info` |
| `GITHUB_TOKEN` | no | `github-mcp` and `tools-fetch` rate limits; a missing token skips that server |

Source of truth is `internal/config/config.go`. Add a variable there in the
same change as this table.

`HISTORY_MAX_TOKENS=32000` and `TOOL_RESULT_MAX_CHARS=6000` were sized for a
cloud window. The eval sets `OLLAMA_CONTEXT_LENGTH=32768`. Resetting those
two from a measured `/tokens` reading is still open
([coding-agent-plan.md](coding-agent-plan.md#defaults-that-fight-a-32k-window)).

### MCP manifest

Lists of processes don't fit env vars. One TOML file. If a server is listed,
the agent may call it — membership is the grant. A server that fails to
connect is skipped; the others stay up.

```toml
[[server]]
name    = "fs"
command = "fs-mcp"
args    = ["--root", "${GEORGE_ROOT}", "--tool-tier", "core"]
force   = true
```

`args` and `env` expand `${VAR}` before the child starts. `george run` sets
`GEORGE_ROOT` and puts `~/.local/share/george/bin` first on `PATH`.
`download_url` + `download_tag` feed `george tools-fetch`. Placeholders:
`{os}` `{arch}` `{tag}` `{version}` (`version` = tag without a leading `v`).
`george tools-plan` prints the inventory without downloading.

`tools` / `exclude` filter what is **published**. The child still starts.
Names are `{server}__{tool}`. Full contract: [mcp.md](mcp.md).

### Files

| Role | Path |
| --- | --- |
| Env, manifest, persona | `~/.config/george/` (`env`, `mcp.toml`, `PERSONA.md`, `SELF.md`) |
| SQLite | `~/.local/share/george/george.db` |
| MCP binaries | `~/.local/share/george/bin` |
| Workspace | `GEORGE_ROOT` |

Prompt order is the embedded contract, then `PERSONA.md`, then `SELF.md`.
Other `*.md` in that directory is ignored. A missing directory still yields
the contract. `george init` writes the config directory and skips files
that exist. The env file is mode `0600`.

`PERSONA.md` is name, voice, and tone. Empty is the shipped default. george
never writes it. Headings the harness used to own (`## Self-notes`,
`## Location pins`, `## Follow-up`, `## Reactions`) are dropped from the
prompt at load time and left on disk.

## Agent loop

One human line is one turn. The REPL reads the next line only after the
reply. Keep the loop bounded:

1. **Assemble.** Contract + `PERSONA.md` + `SELF.md` + optional summary +
   history + optional `[memory]` + server health + the user line +
   `[harness]` clock. Tool schemas are on the request, not in the persona.
2. **Call the model** with the published catalog (`force = true` prefixes
   always; other prefixes only after `mcp_enable`).
3. **Tool iteration.** One batch per round. Independent calls run
   concurrently; one stdio child still serializes its own calls. Repair an
   unambiguous name, otherwise return at most five closest names and
   constrain the next call to them. Salvage a call printed as JSON.
   Truncate each result to `TOOL_RESULT_MAX_CHARS`. Past ~70% of
   `TOOL_MAX_ITERATIONS`, one note says how many rounds remain. At the cap,
   one landing call runs with tools withheld.
4. **Reply** on stdout, then append the turn. Finish the stream before the
   SQLite write so a complete draft is not held for the fold.

Every turn logs `turn perf`: `iterations`, `tool_calls`, `max_batch`,
`recoveries`, prompt/gen estimates, native `usage` when the completer sent
it, `model_ms` / `tool_ms` / `total_ms`, `outcome`. `/perf` prints the same
record.

| Mechanism | Behavior |
| --- | --- |
| History caps | Drop oldest past `HISTORY_MAX_MESSAGES` / `HISTORY_MAX_TOKENS` (chars/4 estimate). Filler strip is prompt-only, on user lines older than the last 40. SQLite stays verbatim. |
| Trimmed turns | Gone. No completion folds them into `session.summary`. A summary already in an older `george.db` is still read until `/reset`. |
| Tool truncate | Each tool result capped at `TOOL_RESULT_MAX_CHARS` |
| Tool collapse | Payloads older than the last 2 tool rounds become one-line markers; matching tool-call args are stubbed to `{}`. A round is one model-emitted batch. Session history stores the reply text, not tool payloads. |
| Iteration cap | `TOOL_MAX_ITERATIONS` rounds with tools, then one landing call |

`/new` wipes that session. `Voice:` folds into `SELF.md` when self-notes are
on. `Facts:` park as a memory episode. `PERSONA.md` is never written.
Other memory rows stay.

### Self-notes (`SELF.md`)

`PERSONA.md` is who you asked the agent to sound like. `SELF.md` is the
coding taste it has learned — review style, commit shape, what never to do —
and it outlives `/new` and a change of repo.

- Lives in `PERSONA_DIR`, in the stable prompt prefix, after the contract
  and `PERSONA.md`.
- `self_note` appends one short line. It does not rewrite the file.
- On history trim, new `Voice:` bits append the same way.
- On `/new`, distill merges into `SELF.md`. A bland tool session with no
  Voice does not rewrite the file.
- Cap 4096 characters. At capacity the tool refuses until distill or you
  prune.
- Needs a writable persona directory (`SELF_NOTES_ENABLED`, default on).

Repo facts (commands, conventions, goals) are memory rows, not this file.
Audit or delete `SELF.md` if the agent drifts.

## Memory

Facts that should survive the session live in SQLite. The model writes the
rows. Nothing promotes them on a timer: a consolidator would spend a
completion on every boot of a CLI. Auto-save of every turn is off.

### Store

One file, `$DATA_DIR/george.db`, WAL, pure-Go driver. Open it with `sqlite3`.

```sql
CREATE TABLE memory (
  id            INTEGER PRIMARY KEY,
  kind          TEXT NOT NULL,  -- fact | preference | person | episode | insight
  subject       TEXT NOT NULL,
  content       TEXT NOT NULL,
  source        TEXT NOT NULL,
  confidence    REAL DEFAULT 1.0,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL,
  expires_at    TEXT,
  superseded_by INTEGER,
  consolidated  INTEGER NOT NULL DEFAULT 0
);
CREATE VIRTUAL TABLE memory_fts USING fts5(subject, content, content='memory', content_rowid='id');
```

`consolidated` is a leftover column. No job sets it.

The coding subjects the tools already describe:

| Subject | What lands there |
| --- | --- |
| `pref/<what>` | How you like a change shaped (`preference`) |
| `cmd/<what>` | The command this repo actually runs (`fact`) |
| `order/<what>` | Order of steps |
| `convention/<what>`, `gotcha/<what>` | What the tree does not say in a file you would have read |
| `goal/<slug>` | A goal for this repo (`insight`); done is `memory_forget` |

`AGENTS.md`, `CONTRIBUTING.md`, and the `Makefile` win. A memory row is what
the agent learned that is not written there. Memory never writes into the
repo.

A `scope` column (you versus this repo) and a session id per repo are
planned and not in the schema yet
([coding-agent-plan.md](coding-agent-plan.md#memory-you-and-the-repo)).
Today every row is visible to every `george` that opens this database, and
the session id is the constant `george`.

### Builtin tools

| Tool | Role |
| --- | --- |
| `memory_store` | One row: `kind`, `subject`, `content`. Same kind + subject supersedes the live row. |
| `memory_recall` | FTS5 + recency |
| `memory_forget` | By id or query |

`web_search` is the other builtin (Brave HTTP). It is off without
`BRAVE_SEARCH_API_KEY`.

### Read path

On each turn, hydrate at most ~30 rows: active rows plus FTS5 hits for the
current line, as a `[memory]` block after history so the prefix stays
cacheable. The block is skipped when memory is off.

### Why not vectors

- One person, one file: hundreds or thousands of rows. FTS5 is enough and
  you can read the row.
- Embeddings add a second model and a dimension migration.
- A cloud vector store puts the most sensitive file on the network.

## Ops

The terminal is the console. No port, no dashboard, no `george status`.

| Command / signal | Behavior |
| --- | --- |
| `george` / `george run` | REPL in this repo until `/quit`, EOF, or SIGINT |
| `george init` | Write `~/.config/george/` once; skip existing files |
| `george tools-fetch` | Download the binaries named in `mcp.toml` into `~/.local/share/george/bin` |
| `george tools-plan` | Print that inventory as JSON; do not download |
| `george version` | Build ldflags |
| SIGINT / SIGTERM | Cancel the run context (the in-flight turn stops with it), close MCP, close the DB |
| Logs | JSON `slog` on stderr |

REPL commands: `/new` `/cancel` `/status` `/tools` `/perf` `/memstats`
`/toolstats` `/tokens` `/help` `/brief` `/short` `/off`, plus `/quit`.
`/cancel` only runs if it is the line being handled. The first Ctrl-C is
SIGINT, which cancels the process. Bracketed paste and Ctrl-C that cancels
only the turn are still open
([coding-agent-plan.md](coding-agent-plan.md#input-on-stdio)).

`SPINUP_NOTICE_MS` (default 4s) prints a still-working line before the first
token. `TOOL_TRACE=compact` prints tool activity on the stream.

Dev: `make build|test|lint|run|ci|check`.

## Packaging

- Go 1.26, one module, `CGO_ENABLED=0`.
- The artifact is the `george` binary. Put it on `PATH`. There is no image.
- CI: `go vet`, `golangci-lint`, `go test` with coverage. The badge is
  pushed to `gh-pages` from `main`.
- Release: `make release` (`BUMP=minor|major` or `TAG=vX.Y.Z`).
  `.github/workflows/release.yml` runs GoReleaser on `v*` tags, after the
  live eval workflow.

## Decisions

1. **Name: george.** Binary `george`. One process, one local model, stdin/stdout.
2. **One OpenAI-compat client.** Ollama, llama.cpp, and the cloud APIs already speak that shape. A provider registry is a second product.
3. **Token counting is chars/4**, labeled as an estimate. No tokenizer dependency.
4. **Memory is builtin SQLite.** `MEMORY_BACKEND=mcp:<name>` is the escape hatch. No vector column. Auto-save off. No consolidator.
5. **Taste is `SELF.md`.** Repo commands and goals are memory subjects. A scope column is the planned split and is not shipped.
6. **Four coding servers, `force = true`.** Their schemas stay on the request. GitHub without a token is skipped; `fs` still patches.
7. **Streaming replies default on.**
8. **Logs on stderr.** stdout is the reply.
9. **No listen port and no heartbeat.** Nothing is left running to health-check.
10. **No sandbox.** `shell__command_run` runs as you, in `--root`. Git is the undo.
11. **Fail-soft MCP boot.** One missing binary does not exit the process. A bad manifest does.
12. **No session fold.** History trim spends no completion; trimmed turns are dropped.

**Rejected:** pairing codes; embeddings on the hot path; a tool-shim model; stuffing the MCP catalog into `PERSONA.md`; cron, watches, and a planner; a container as the security boundary.

## Security

The agent edits and runs freely inside `GEORGE_ROOT`. `$DATA_DIR` and
`~/.config/george/env` are the crown jewels. There is no allowlist and no
approval prompt.

| Risk | What we rely on |
| --- | --- |
| A task that asks for a bad command | The contract: no `git push`, `reset --hard`, or `clean` through the shell; no deletes outside the tree; no publishes. `git-mcp` has no push. |
| Untracked or ignored files (`.env`, build output) | No history to revert to. The contract is the only guard. |
| Anything outside `--root` | `fs` and `git` refuse a path that escapes the root, including a symlink. `shell` sets the cwd; it is not a jail. |
| Prompt injection in a file or a tool result | Can still drive any published tool, including `shell__command_run` |
| A buggy MCP binary | Inherits the process environment unless the manifest sets `env` |
| Someone who can read your home directory | Can read `george.db` and the env file |

**Controls that ship:** no listen port; keys in `~/.config/george/env` (init writes it `0600`); manifest membership is the grant; `SELF.md` capped at 4096 characters; memory rows are readable and forgettable; SIGINT cancels the in-flight turn.

**Residual (accepted):** the shell can `curl`, `gh`, and `git push`; caps bound context size, not spend. Treat `mcp.toml` like a list of programs you are willing to run as yourself.

## Related

- [architecture.md](architecture.md) — diagrams and sequences
- [coding-agent-plan.md](coding-agent-plan.md) — the cut list and the phases still open
- [coding-mcp.md](coding-mcp.md) — `fs`, `git`, `shell`, `github`
- [mcp.md](mcp.md) — tool naming and the host
- [todo.md](todo.md) — the work list
