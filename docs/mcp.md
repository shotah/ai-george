# MCP host

How the harness loads tools, names them for the model, recovers from common
hallucinations, and how to exercise that path locally.

Capabilities live in **external MCP stdio binaries**, plus a few harness
builtins (`memory_*`, `self_note`, `mcp_enable`, `web_search`). The host
supervises MCP children: spawn → list → call → truncate → restart. See
[architecture.md](architecture.md) for the process diagram; this page is the
operator contract for naming and local use. The four coding servers and
their tools: [coding-mcp.md](coding-mcp.md).

A coding turn is several tool rounds on a local model, and a bounced call
costs a whole round. The host has to repair names and finish turns, or the
edit never lands.

---

## Why MCP (and nothing else)

| Goal | How MCP helps |
| --- | --- |
| Keep the harness small | Workspace read, patch, git, and shell stay out of `george`. Web search is a builtin. |
| Clear grant model | A server in `mcp.toml` is granted; omit it and it does not exist |
| One install step | Static Go binaries over stdio; `george tools-fetch` drops them in one directory |
| Swappable brains | The same tool schemas work for any OpenAI-compatible endpoint |

Memory, `self_note`, and web search work with **zero** MCP servers. Editing
a file needs `fs`.

---

## Naming: `{server}__{tool}`

**Authoring new MCP tools?** Follow the shared package contract:
[mcp-naming.md](mcp-naming.md) (`{service}_{verb}_{object}`, no server-id in the
tool name, stable verbs, sibling nouns). This section is the **host** side.

Every published tool reaches the model as:

```text
{server_or_tools_prefix}__{original_tool_name}
```

Examples:

| Manifest `name` | MCP tool | Name the model must call |
| --- | --- | --- |
| *(builtin)* | `web_search` | `web_search` (no prefix) |
| *(builtin)* | `memory_store` | `memory_store` (no prefix) |
| `fs` | `file_get`, `file_patch` | `fs__file_get`, `fs__file_patch` |
| `git` | `status_get`, `commit_create` | `git__status_get` |
| `shell` | `command_run` | `shell__command_run` |
| `github` | `issues_list`, `pulls_get` | `github__issues_list` |

**Why the prefix?** OpenAI-safe characters, no collisions across servers, and
obvious provenance in logs and collapsed history markers.

Optional override in `mcp.toml`:

```toml
[[server]]
name = "github"
tools_prefix = "gh"   # tools become gh__issues_list, …
```

Default prefix is the server `name`. Prefer short, stable prefixes when a
hyphenated product name fights the model (see below).

Inspect what the running agent sees:

- `/tools` (total + per-server `schema_est_tokens`)
- `/status` includes `schema_est_tokens` alongside history size
- Boot logs: `mcp server connected` (`tools_listed` vs `tools_published`), then
  `tool schema estimate` + one `tool schema by server` line per prefix
- **Fail-soft boot:** if one `[[server]]` fails to spawn/initialize (missing API
  key, broken binary, EOF), george logs `mcp server boot skipped` with a stable
  `reason` (`no_binary` / `no_key` / `no_oauth` / `connect`) and continues.
  Calling a skipped prefix returns `tool error [<reason>]: … is skipped — do
  not invent <name>__* names`. A missing `GITHUB_TOKEN` skips `github` and
  leaves `fs`, `git`, and `shell` up.

---

## Local models and mangled names

Small local models (Qwen via Ollama in the eval) rewrite tool names: the
whole name normalized to underscores, a dot or a single underscore where the
`__` should be, a bare tool name with no prefix, or a prefix that does not
exist. Each miss is a full model round-trip, the most expensive thing in a
local-model turn, so the host repairs what it can without the model.

Web search is the unprefixed builtin `web_search` (Brave Search HTTP:
titles, URLs, snippets). A bare `google_search` still reaches it.

### Alias resolve (automatic)

On `Call`, if the exact name is missing, the host tries in order:

1. **Hyphenate the prefix.** `my_server__tool` → `my-server__tool`. Tool
   suffixes are never rewritten.
2. **Wrong separator.** A real prefix joined to a real tool by `.`, `_`, `-`,
   `/`, or `:` instead of `__`:

   ```text
   fs.file_get        →  fs__file_get        ✅ called
   git_status_get     →  git__status_get     ✅ called
   shell-command_run  →  shell__command_run  ✅ called
   ```

3. **Unique bare name.** A real tool name wearing an invented or missing
   prefix, when exactly one published tool has that name:

   ```text
   mcp__file_get  →  fs__file_get   ✅ called
   file_patch     →  fs__file_patch ✅ called
   ```

Two cases are left to the model on purpose:

- **Real prefix, wrong tool** (`fs__status_get` when only `git` has it).
  The model chose that server deliberately, so it gets that server's catalog
  rather than a silent hop to a different server.
- **Two servers publish the name.** Guessing is a coin flip, so both real
  names go back as a hint.

When any repair fires, logs show:

```text
mcp tool name aliased  requested=…  resolved=…
```

`/toolstats` counts them as `prefix_alias`.

### Unknown-tool suggestions

If lookup still fails, the error string is model-facing and catalog-aware:

| Mistake | Hint shape |
| --- | --- |
| Wrong tool, real prefix | `no such tool; valid fs tools include: fs__file_get, fs__file_patch, … — retry with one of these exact names` |
| Underscored or hyphenated prefix, wrong tool | `no such tool or server prefix "f_s" (did you mean "fs"?); valid fs tools include: …` |
| Invented name (fake prefix, merged suffix) | `no such tool; closest real names are: fs__file_get, fs__file_patch — retry with one of these exact names` |
| Unknown prefix, nothing close | `available server prefixes are: fs, git, github, shell` |

The agent loop feeds that error back as a tool result so the next iteration
can self-correct (same pattern as argument/schema failures).

Closest-name ranking scores shared name tokens, ignoring generic verbs (`get`,
`list`, `set`, …) that would otherwise match everything. A model that invents a
tool tends to stitch real fragments together, so the fragments identify what
it was reaching for even when neither prefix nor suffix is real.

### Printed tool calls

A model can also skip the `tool_calls` field entirely and *print* the call:

```json
{ "name": "fs__file_get", "parameters": { "path": "a.go" } }
```

Without handling, that JSON is just assistant text, so it becomes the visible
reply, the agent answering in wire format. george parses it back into a real
call and runs it, accepting the object bare, fenced, inside `<tool_call>` tags,
or embedded in prose, with arguments under `arguments`, `parameters`, `args`, or
`input` (models pick all of them).

```text
model printed a tool call instead of emitting one; executing it  name=… chars=90
```

The name must look like a tool, published or at least carrying a `server__`
prefix, so an ordinary reply that happens to contain JSON is never hijacked. An
unpublished but prefixed name is still run on purpose: the host's answer is what
names the real tools, which feeds the retry below.

### Grammar-constrained retry

A hint only works if the model reads it. When a name cannot be resolved *and*
there are real candidates, the next model call is constrained instead: george
sends `response_format` with a JSON schema whose `name` is an `enum` of those
candidates. Ollama compiles that to a GBNF grammar and masks every token that
would spell anything else, so a bad name stops being unlikely and becomes
impossible.

```text
tool call failed        name=fs__read_file
constraining retry …    requested=fs__read_file candidates=2
model call              iteration=2 forced_tool_names=2
```

Three details make it work:

- **`tools` stays in the request.** The model reads real parameter schemas from
  there. Drop it and the name is still legal but the arguments are invented
  (measured on Qwen: `start_date`/`end_date` instead of `date`).
- **The call arrives in `content`, not `tool_calls`.** Ollama omits `tool_calls`
  whenever a `response_format` is set (still true in 0.32.4), so the provider
  parses the JSON object back into a `ToolCall`.
- **It is one-shot, and never streams.** A grammar forces *every* reply to be
  JSON, so leaving it on would make conversational answers impossible; and
  streaming it would type raw JSON into the user's terminal.

No candidates means no constraint; forcing a call out of the whole catalog is
just a different guess.

### What aliasing does *not* fix

- Invented tool suffixes (`fs__read_file`): no real name to repair to, so these
  still need the constrained retry above
- Wrong arguments (a hunk header that does not match the file): MCP/API errors
- Think-only turns with no tool call: agent nudge / stall path in
  `internal/agent` (separate from naming)
- A model that neither calls nor prints anything callable: still a nudge, then
  the turn answers with whatever prose it has

---

## Manifest filters

Listed servers **start**. `tools` / `exclude` only filter what is **published**
to the model:

```toml
[[server]]
name = "fs"
command = "fs-mcp"
args = ["--root", "${GEORGE_ROOT}"]
tools = ["file_get", "file_list", "file_patch"]  # allowlist
# exclude = ["raw_*"]
```

Boot logs `tools_listed` vs `tools_published`. Schema cost is estimated as
`est_tokens` (chars/4); `TOOL_SCHEMA_MAX_TOKENS` can hard-fail an oversized set.
Prefer MCP-native tiers (`--tool-tier core`) first: [design.md](design.md#decisions).

### Call budget (`budget`)

A metered API gets its quota written into the manifest, and the host
enforces it:

```toml
[[server]]
name = "github"
command = "github-mcp"
budget = "50/day"        # or "50/month"
```

Why the host and not the prompt: the model is not a careful caller. A single
question can produce three to five calls to the same server in one turn.
Every call ends in the same `Host.call`, so that is where the counter sits,
after argument validation (a malformed call spends nothing) and before the
child is touched (a refused call never reaches the vendor).

- Counted per server per period in `george.db` (`mcp_budget`). The day rolls
  at the **human's** midnight (the persona timezone), the month on their first.
- Over budget, the tool result is a refusal that names the reset and says
  not to retry: `mcp: github budget 50/day used (50 calls this day); resets
  2026-10-03 00:00 PDT — do not retry; use what you already have and tell
  the human`. The model reports instead of burning.
- `/tools` shows `budget_refused=N` once anything has been turned away.

The count is of attempts: a call the vendor rejects still spent a slot,
because it almost certainly spent one of theirs. None of the four coding
servers ships with a budget.

### Prefix enable (`dynamic_tools`)

By default (`dynamic_tools` omitted or `true`) MCP schemas stay **off** until
the agent calls `mcp_enable` (list of prefixes, next Completer call in the
same turn). The prompt lists on vs off under `[mcp prefixes]` and tells the
model to review that list and enable a needed off prefix this turn. Brief hold
idles out at 6h; short at 27h. Harness builtins stay on.

The four coding servers are `force = true` in the shipped manifest, so their
schemas are on every turn and no round is spent on `mcp_enable` before an
edit. With that manifest the block reads `on: fs (force); git (force); …`
and lists nothing under `off`; an added server without `force` is what
puts a prefix there.

Small models / rollback, full catalog every turn, no `mcp_enable`:

```toml
dynamic_tools = false
```

A server that should never idle-drop while dynamic tools are on:

```toml
[[server]]
name = "github"
force = true          # whole server prefix; pair with a tight `tools` allowlist
```

Or `MCP_ENABLE_FORCE=fs,git,shell`. Human overrides: `/brief` `/short`
`/off`. `/tools` shows published vs available.

---

## Using this locally

### A — REPL (fastest feedback)

```bash
# from repo root
make init          # ~/.config/george: env, mcp.toml, PERSONA.md, SELF.md
george tools-fetch # fs-mcp, git-mcp, shell-mcp, github-mcp into ~/.local/share/george/bin
# edit ~/.config/george/env — LLM_* (Ollama, llama.cpp, or a hosted OpenAI-compatible API)

make run           # same as `george` in this repo
```

In the REPL:

```text
/status     # model, history, tool count
/tools      # exact prefixed catalog the model sees
/toolstats  # calls, aliases, unknown names, constrained retries since boot
```

Ask something that needs a tool ("what's in `readme.md`?", "run `make
test`"). Watch stderr JSON for `tool call`, `mcp tool name aliased`, or
`tool call failed` with the suggestion string.

Point at another config directory:

```bash
GEORGE_CONFIG_DIR=/path/to/other/george make run
```

The four coding servers are granted by the manifest `george init` writes.
Add another MCP server the same way. Trust `/tools` plus the alias and
suggestion strings when the model mangles a name. Do not copy the MCP
catalog into `PERSONA.md`.

### Unit tests (no LLM)

```bash
go test ./internal/mcp/ -count=1
```

Covers catalog suggestions, separator and prefix aliasing, and "did you mean"
hints without spawning real MCP binaries. The live eval
([eval_setup.md](eval_setup.md)) boots the real releases and checks the
coding catalog before any model call.

---

## Operator checklist

- [ ] Only list servers this machine should run (`mcp.toml` = grant; they run as you)
- [ ] Prefer MCP `--tool-tier` / `tools = […]` so a local model sees tens of tools, not hundreds
- [ ] After `tools-fetch`, `/tools` once and confirm the published names
- [ ] On weird tool loops: check logs for `aliased` vs repeated `unknown tool`

---

## Related

- [architecture.md](architecture.md) — host restart sequence
- [design.md](design.md) — env contract + MCP manifest sketch
- [coding-mcp.md](coding-mcp.md) — `fs`, `git`, `shell`, `github`
- [mcp-naming.md](mcp-naming.md) — package-author naming contract
- [todo.md](todo.md) — work after the fork
