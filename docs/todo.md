# george — work after the fork

george is the CLI cut of [ai-gantry](https://github.com/shotah/ai-gantry). One process, one local model, MCP tools, stdin/stdout. The binary is `george`.

The job is a local agent you can grade. A green run on Gemini does not count. A family tag (`qwen3`, `qwen2.5`, `qwen3-coder:latest`) does not count. The eval names one model with a size and a quant, and every run on that model has to pass.

This file is the work list. The other notes beside it are the goose-side evidence (`fork-cli-agent.md`, `ollama-and-llama.md`, `extra-model-calls.md`, `storage.md`). They are not the design of george.

goose stays where it is. Do not port its provider matrix, recipes, desktop, or second agent loop.

## 1. Fork and rename

- [x] Fork `ai-gantry` into the new repo. Module, binary, module path, and user-facing strings become `george`.
- [x] Leave the gantry name on the crane. Pendant, Cab, and gantree stay their own repos. george does not import them.
- [x] Default channel is stdio. `george` with no `CHANNEL` set talks on stdin/stdout. A missing channel must not come up as Telegram.
- [ ] Health stays an exit code. Nothing in this process listens.

## 2. Pin the model the eval grades

Write one id, in one place, that the live eval reads. Include the size and the quant. Example shape, not a decision: `qwen2.5-coder:32b` at `Q4_K_M`, or a GGUF repo plus quant such as `bartowski/Llama-3.2-1B-Instruct-GGUF:Q4_K_M`. Pick the model you will actually run.

- [ ] Record that id in the eval env (the same three values gantry already uses: base URL, model, key). The model string is the full id.
- [ ] Refuse to start the live eval when the model string is a family tag: `qwen3`, `qwen2.5`, `qwen3-vl`, `qwen3-coder:latest`, `llama3.2`, or a Gemini id.
- [ ] Print the id at the top of every live run, next to the pass line, so a green log says which weights were graded.
- [ ] First live reading is that local id. A Flash log is unread until this one passes.

goose’s published local claims, for the record you are walking away from:

| Place | String they wrote |
| --- | --- |
| Known issues, “works with extensions” | `qwen2.5` |
| Ollama setup example | `ollama run qwen2.5` |
| LM Studio example | `qwen2.5-7b-instruct` |
| Custom distro default | `qwen3-coder:latest` |
| Code default `OLLAMA_DEFAULT_MODEL` | `qwen3` |
| Known-name list | `qwen3`, `qwen3-vl`, `qwen3-coder:30b`, `qwen3-coder:480b-cloud` |
| Skipped live test | `qwen3`, `qwen3-vl` |
| XML-parser comment, models that drop native tool calls | `qwen3-coder`, `qwen3-coder-32b` |

Those strings do not name one model. The March 2025 goose benchmark did (`qwen2.5-coder:32b` Q4_K_M and three siblings). That post is not a test that still runs.

## 3. Grade tool calls on that model

Keep gantry’s two contracts.

- [ ] Goldens stay free and on every push: the bytes from stdin JSON through the completer request to the HTTP body.
- [ ] `make integration-test` stays the paid gate: tools, arguments, call counts, memory rows. It checks shape, not the sentence. Every run passes, or the rule is not carried.
- [ ] Add one fixture whose only job is a mangled MCP name on the pinned model: dot instead of the separator, a hyphen in the server prefix, and a near-miss. The child that runs is the real tool. A pasted XML string in a unit test does not satisfy this.
- [ ] Keep the repair on the session model: prefix alias, at most five closest names, one grammar-constrained retry, salvage a call printed as JSON, a landing call when the iteration budget runs out. Count them.
- [ ] Confirm `LLM_SYSTEM_FOLD` against this model’s chat template before you trust a red or a green. The open risk is a template that renders system text only at position 0. If the model cannot see the harness block, tool-call failures are a prompt-placement bug.

## 4. Keep the loop small enough for that model

- [ ] Last two tool rounds stay whole, including a parallel batch. Older rounds become a one-line marker in-process. No completion spent to describe a tool pair.
- [ ] Schemas stay off until that prefix is enabled. The prefix list itself stays a stable line.
- [ ] Fold stays one completion. No second “store everything now” turn.
- [ ] Quiet watches poll a tool and do not call the model.
- [ ] Memory stays SQLite you can open with `sqlite3`. FTS is fine. No embedding call, no vector column, no vector database.
- [ ] Auto-save of every turn stays off. A row lands because the model stored it, or because a consolidator promoted an episode that was stored on purpose.

## 5. Cut what grew up on Gemini

Do this after the fork boots on stdio, before adding features. The planner and the mouths are why a local model stopped being the daily driver.

- [ ] Remove Telegram, Discord, and Slack from the default path. Delete them once stdio is the product, or leave them behind a build tag you do not ship.
- [ ] Remove the daily planner note (the long policy string, measured near 460 words). Small models drop the middle. Goldens will stay green while the model ignores a clause. That is the failure mode.
- [ ] Remove the aims ledger, the todo-as-memory rows, and the pendant room frames (`[room]`, avatar, backdrop, theme) from the default prompt. They are the crane’s household. They are not the CLI runner.
- [ ] Drop Gemini thought-signature handling unless that provider is still on the socket. One socket, the local one.
- [ ] Re-read every fixture that was green on Flash. Retarget it at the pinned model. A miss is a sentence to shorten, not an expectation to loosen.

## 6. Do not bring these over from goose

Each one is an extra completion, a second model, or a vague model id.

- [ ] No tool shim and no interpreter model (`mistral-nemo`, `qwen2.5:3b`, or a local second GGUF).
- [ ] No session-title completion on the first turns.
- [ ] No auto-compaction that asks the model to summarize the history.
- [ ] No per-tool-pair summarizer.
- [ ] No Smart Approve judge call, and no adversary-inspector completion.
- [ ] No skills directory, no subagents, no inbound port, no control UI.
- [ ] No name repair that only accepts an exact rewrite of `extension__tool` and then pastes the full tool list back into the next prompt.
- [ ] No live test that skips unless a host env var is set, and no CI that never sets it. The golden stays on every push. The live eval is the release gate, and it names the model.

## Done when

- [ ] `george` on a fresh tree talks stdio and calls the pinned local model.
- [ ] The live log prints that full model id.
- [ ] The mangled-name fixture passes on that model, every run.
- [ ] A long tool session still answers after older rounds have been marked down, and the process log shows no extra completion for those markers.
- [ ] `sqlite3` on the memory file shows rows, and the repo has no embedding dependency.

## 7. Coding tools

Workspace read, patch, git, and shell are MCP binaries. The plan, the server ids, and the tool names are [coding-mcp.md](coding-mcp.md). Nouns are reserved in [mcp-naming.md](mcp-naming.md).

New repos (`fs-mcp`, `git-mcp`, `shell-mcp`, later `github-mcp`). They are not features of this process, and they are not patches to the plugins under `repos/`.
