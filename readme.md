# <img src="assets/logo.svg" alt="" width="40" height="40"> george

<p align="center">
  <img src="assets/banner.svg" alt="george — a CLI coding agent. It reads your files and edits your code." width="100%">
</p>

<p align="center">
  <a href="https://github.com/shotah/george/actions/workflows/ci.yml"><img src="https://github.com/shotah/george/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/shotah/george/actions/workflows/ci.yml"><img src="https://github.com/shotah/george/raw/gh-pages/badges/coverage.svg" alt="Coverage"></a>
</p>

> **george** — a coding agent for your terminal. One process, one local
> model, four MCP servers, stdin/stdout.

You run it in a repo and type. It reads files, patches them, runs the
check, and shows you `git status`. It talks to any OpenAI-compatible
endpoint, and the eval grades it against a pinned local model
(`qwen3-coder:30b-a3b-q4_K_M` on Ollama).

```text
static binary + persona + OpenAI-compat LLM + fs / git / shell / github MCP  →  edits in your repo
```

There is no shell sandbox and no approval prompt. The agent edits and runs
freely inside `--root`, and git is the undo. Untracked files and anything
outside the root have no history, so the persona is the only guard there.

The engineering budget went into the **loop**: parallel tool batches,
name repairs so a small local model can finish a turn, tool results that
collapse instead of rotting the context, and memory that survives `/new`.

george is a fork of an assistant harness. The assistant parts are gone:
chat apps, cron, the planner, watches, aims, and todos. What is left and
what comes next: **[docs/coding-agent-plan.md](docs/coding-agent-plan.md)**.
The work list is **[docs/todo.md](docs/todo.md)**.

---

## Get started

You need an OpenAI-compatible model on this machine (Ollama, llama.cpp,
or an API you already have).

```bash
ollama pull qwen3-coder:30b-a3b-q4_K_M
```

Two ways to get a binary: [download a release](https://github.com/shotah/ai-george/releases),
or clone the repo and build it. Then install it once and put `george` on
`PATH` (`~/.local/bin` usually is).

### Download and run

Open the [releases page](https://github.com/shotah/ai-george/releases) and
take the archive for your machine. Names look like
`george_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows). `checksums.txt`
is on the same page.

```bash
tar -xzf george_*_linux_amd64.tar.gz   # the archive you downloaded
```

That leaves a `george` binary in the current directory. On Windows, unzip
and use `george.exe` in the install step below.

### Build and run

You need Go.

```bash
git clone https://github.com/shotah/ai-george.git && cd ai-george
make build
```

That leaves `bin/george`.

### Install once

```bash
mkdir -p "$HOME/.local/bin"
install -m 755 george "$HOME/.local/bin/george"
# from a clone: install -m 755 bin/george "$HOME/.local/bin/george"
george init
george tools-fetch
```

`init` writes four files to `~/.config/george/` and skips any that
already exist:

| File | What it is |
| --- | --- |
| `env` | The model and keys (`LLM_*`, `GITHUB_TOKEN`, `BRAVE_SEARCH_API_KEY`). Mode 0600. It starts pointed at local Ollama. |
| `mcp.toml` | The four MCP servers. |
| `PERSONA.md` | Your name, voice, and timezone. All comments until you write in it. |
| `SELF.md` | What george learns about how you like the work done. |

`tools-fetch` downloads `fs-mcp`, `git-mcp`, `shell-mcp`, and
`github-mcp` into `~/.local/share/george/bin`. george puts that directory
first on `PATH` for its servers, so your shell doesn't need it. The
database is `~/.local/share/george/george.db`.

Then `cd` into any repo and run `george`. The root is the git toplevel
of the directory you started in (or that directory, outside a repo), and
`mcp.toml` hands it to the servers as `--root ${GEORGE_ROOT}`. Every repo
gets the same model, persona, database, and tools.

Anything set in your shell wins over `~/.config/george/env`. To use a
different config directory, set `GEORGE_CONFIG_DIR`. `george help` lists
every path override.

Send `/status`, then a task: `greet.txt says hi. Change it to hello, then
run wc -l greet.txt.`

### Drop in a different model

The agent does not care which endpoint you picked. Change these three in
`~/.config/george/env`:

| You have | Set |
| --- | --- |
| Ollama on this machine | `LLM_BASE_URL=http://127.0.0.1:11434/v1`, `LLM_API_KEY=ollama`, `LLM_MODEL=<id>` |
| Another OpenAI-compatible API | `LLM_BASE_URL` + `LLM_API_KEY` + `LLM_MODEL` |

The eval numbers only hold for the model they were run against.

---

## Tools

| Server | Tools | Job |
| --- | --- | --- |
| `fs` | `file_get`, `file_list`, `file_search`, `file_patch`, `file_create` | Read and edit inside `--root` |
| `git` | `status_get`, `diff_get`, `commits_list`, `stage_update`, `commit_create` | Look at and record changes. No push |
| `shell` | `command_run` | Build, test, lint |
| `github` | issues, pulls, checks, contents | Optional. Reads `GITHUB_TOKEN` or `GH_TOKEN` |

Built in, with no MCP server: `memory_store` / `memory_recall` /
`memory_forget`, `self_note`, `mcp_enable`, and `web_search` (Brave).
Wiring and naming: [docs/coding-mcp.md](docs/coding-mcp.md),
[docs/mcp.md](docs/mcp.md).

## Slash commands

| Command | What it does |
| --- | --- |
| `/new` | Reset the session. Taste goes into `SELF.md`, facts park in memory |
| `/cancel` | Stop the turn in flight |
| `/status` `/perf` `/tokens` | Uptime and model, last turns' rounds and batches, prompt size |
| `/tools` `/toolstats` `/memstats` | Tool catalog, per-tool calls since boot, memory row counts |
| `/brief` `/short` `/off` | Hold an MCP prefix on for ~6h or ~27h, or drop the hold |
| `/help` `/quit` | This list, exit |

## What it remembers

| Where | What | Who writes it |
| --- | --- | --- |
| `PERSONA.md` | How the agent works: the edit chain, when to batch, never claim a green it did not see, commit only when asked | You |
| `SELF.md` | How you like the work done, in every repo: review style, commit shape, what to never do | The agent, when you say it or correct it |
| memory (SQLite) | Repo facts: how to test and build, conventions, goals | The agent, via `memory_store` |

Same kind and subject replaces the live row, so a corrected fact
supersedes the old one. Memory scoped per repo is planned
([Memory](docs/coding-agent-plan.md)); today rows are shared.

## Tested two ways

| Contract | What it pins | Runs |
| --- | --- | --- |
| **Golden** | The bytes the model sees: one stdin line → `Handle` → Completer request → HTTP body | `go test ./...`, free |
| **Behavior eval** | What the model did with the shipped persona: which tools, their args, which calls shared a batch, the order, and the reply. Never the exact sentence | `make integration-test`, against the live model |

The eval fetches the real MCP releases and grades against their live
tool catalog, so a renamed tool fails before any model call. Each fixture
runs N times and every run must pass. Fixtures cover the serial edit
chain and parallel reads, checks, and patches. Setup:
**[docs/eval_setup.md](docs/eval_setup.md)**.

---

## Read next

| If you want… | Go here |
| --- | --- |
| What changes from the assistant, and in what order | **[docs/coding-agent-plan.md](docs/coding-agent-plan.md)** |
| The work list | **[docs/todo.md](docs/todo.md)** |
| How the harness is put together | **[docs/architecture.md](docs/architecture.md)** |
| Run the live behavior eval | **[docs/eval_setup.md](docs/eval_setup.md)** |
| The coding MCP servers | **[docs/coding-mcp.md](docs/coding-mcp.md)** |
| Why this tree, not goose | **[docs/fork-cli-agent.md](docs/fork-cli-agent.md)** |

## License

MIT — see [LICENSE](LICENSE).
