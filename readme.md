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

## Hello

You need an OpenAI-compatible model on this machine (Ollama, llama.cpp,
or an API you already have) and Go to build.

```bash
ollama pull qwen3-coder:30b-a3b-q4_K_M
git clone https://github.com/shotah/george.git && cd george
make init                      # deploy/persona + deploy/mcp.toml from examples/
go run ./cmd/george tools-fetch   # fs-mcp, git-mcp, shell-mcp, github-mcp
```

Point `--root` in `deploy/mcp.toml` at the repo you want it to work in,
then:

```bash
export LLM_BASE_URL=http://127.0.0.1:11434/v1 LLM_API_KEY=ollama \
       LLM_MODEL=qwen3-coder:30b-a3b-q4_K_M LLM_REASONING_EFFORT=none
export DATA_DIR=./deploy/data MCP_MANIFEST=./deploy/mcp.toml
make run
```

Send `/status`, then a task: `greet.txt says hi. Change it to hello, then
run wc -l greet.txt.`

The paths are still container-shaped while the off-Docker step lands.
After it, config lives in `~/.config/george/`, data in
`~/.local/share/george/`, and the root is the git toplevel of the
directory you start in. See
[Where it lives](docs/coding-agent-plan.md#where-it-lives-no-docker).

### Drop in a different model

The agent does not care which endpoint you picked. Set these three:

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
