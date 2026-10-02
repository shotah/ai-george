# Which tree to fork for a CLI agent

Fork [ai-gantry](https://github.com/shotah/ai-gantry). Keep its turn loop. Cut the mouths and the personal-assistant seed until what is left is a CLI process, one model, and MCP tools.

goose is the wrong base for that product. Its CLI is a front end on a desktop coding agent: two agent loops, many providers, recipes, subagents, and an Electron app. The two behaviors a small model needs — repair a mangled MCP name, and shrink old tool results without another completion — are thin here and load-bearing there.

This note reads goose from this checkout. It reads ai-gantry from the public tree on `main`: the README, `docs/features.md`, `docs/eval_setup.md`, `docs/evaluation_fable.md` (Claude Fable, 2026-09-26), and `docs/Evaluation_Grok.md` (Grok, 2026-09-28). Those two evaluations are model write-ups inside that repo. They compare gantry to OpenClaw, Letta, and Hermes. They do not mention goose. Grok did not re-run the live eval.

## The job

A CLI agent runner for a small local model, aimed at Llama:

- one process, one model socket, stdin/stdout
- MCP tools as optional children
- a misspelled or dot-separated tool name still runs
- old tool results shrink in the process, with no extra completion
- session text stays rows you can query, not vectors
- a test that fails when the model was shown different bytes, and a test that fails when the model called the wrong tool

## Side by side

| | goose (this checkout) | ai-gantry (`main`) |
| --- | --- | --- |
| Shape | Rust workspace. CLI plus Electron. Many providers, recipes, subagents, ACP. | One static Go binary. One OpenAI-compatible socket. Optional MCP children. |
| CLI | `crates/goose-cli`. The turn is `crates/goose/src/agents/state_machine/` and, until that migration finishes, `crates/goose/src/agents/agent.rs` as well. | `CHANNEL=stdio`, `make init && make run`. Chat can also be Telegram, Discord, Slack, or the pendant mailbox. One channel per process. |
| Model calls on a plain turn | The reply, plus a session-title completion for the first three user messages, plus auto-compaction once `GOOSE_AUTO_COMPACT_THRESHOLD` is crossed. See [extra-model-calls.md](extra-model-calls.md). | The reply rounds. Fold is one completion (`Facts:` and `Voice:`). A silent flush before fold is refused. Quiet watches poll a tool and do not call the model. A consolidator batch (default 30 minutes) is a separate completion over episodes the model already stored. |
| Second model | Tool shim, off by default. When on, the Ollama interpreter is `mistral-nemo`. | No interpreter model. Repairs stay on the session model. |
| MCP names | Tools are published as `extension__tool`. `recover_mangled_tool_name` accepts an exact rewrite: strip `functions.`, turn `__` into `.`, or glue the owner back on. Anything else is an unknown-tool error, and the next request can paste the full tool list into the conversation. | Prefix alias (hyphen vs underscore on the server prefix), at most five closest names, one grammar-constrained retry, printed-call salvage, a landing call when the iteration budget runs out. `/toolstats` counts the repairs. |
| Old tool results | Left in the transcript, or summarized by another completion when `GOOSE_TOOL_PAIR_SUMMARIZATION` is set (`summarize_tool_call`). | Last two tool rounds stay whole, including a parallel batch. Older rounds become a one-line marker. No model call. |
| Tool schemas in the prompt | `compact_tools_json` drops parameter schemas, and only on the bundled llama.cpp path. | `[mcp prefixes]` is a stable on/off index. `mcp_enable` ships schemas on the next call. Idle holds are 27h and 6h. |
| Memory | SQLite sessions. Chat recall is `LIKE` on message text. No embedding crate. See [storage.md](storage.md). | SQLite plus FTS5. Typed rows. No embedding API. Auto-save is off. `PERSONA.md` outranks recall. Personality is `SELF.md`, which you can delete a line from. |
| What a regression pins | `cargo test` does not call a model. The Ollama test runs only with `OLLAMA_HOST`, and the strings it passes are the family tags `qwen3` and `qwen3-vl`. Docs that say extensions work name `qwen2.5`. See [ollama-and-llama.md](ollama-and-llama.md). The Llama path is an ignored test on `Llama-3.2-1B` Q4_K_M. | `go test ./...` goldens the bytes from mouth JSON through the HTTP body. `make integration-test` replays fixtures on a live model and checks tools, arguments, rows, and cron jobs. It skips the sentence. Recorded readings are Gemini Flash, not a 4–30B local model. |

## Why goose is the wrong fork

A fix for small models has to land twice. The state machine (`ops_llm`, `ops_toolcalling`, `ops_unknown_tool`, `ops_tool_pair_compaction`) and `agent.rs` both still own a turn. Around that turn sit approvals, recipes, platform extensions, and the desktop session store.

The name repair that exists is an exact string match. A small model that writes `server.tool-name`, a hyphen, or a near-miss falls through to an error and a catalog dump. Models that never emit a native tool call are sent to a second model.

The compaction that exists is another completion per old tool pair, and it is off unless configured. Leaving the pair in place re-bills the payload every round. Summarizing it spends a call to throw the payload away. Neither is the gantry rule: keep the last two rounds, marker the rest, do it in-process.

Forking goose to get a CLI means carrying the desktop agent and deleting most of it, while the loop you want is the part that is weakest.

## Why ai-gantry is the fork

The loop is already the product. Name repair, round collapse, prefix disclosure, and one fold call are in the binary and in the tests. Memory is SQLite you can open. Vectors were declined on purpose. The behavior eval is the regression this checkout does not have: shape of the tool calls, on a live model, every run must pass.

The CLI is already a channel. `CHANNEL=stdio` is how you hack on the binary. Outbound chat and the yard are other processes. Nothing in the crane listens.

What you inherit that a CLI runner does not need, and should cut rather than grow:

- Telegram, Discord, and Slack. The README’s default mouth is still a vendor chat. Pendant and Cab are other repositories.
- The aims ledger, the todo rows, and the daily planner. The planner note is one long policy string (Grok measured it near 460 words). Small models drop the middle of a note that size.
- The Gemini-specific thought-signature path. Keep it only if that provider stays. The local-template check (`LLM_SYSTEM_FOLD=many` on a model that renders system text only at position 0) is still open. Both evaluations say the 4–30B sentence is the design, and the reading that exists is Gemini Flash.

Cut those after the fork. Do not start by adding goose’s provider matrix, recipes, or a second interpreter.

## What to keep from the gantry loop

1. **Same model for the repair.** Alias, five closest names, one constrained retry, salvage a printed call. Stop there. Do not add a tool-shim model.
2. **Rounds, not summaries.** Last two tool rounds in full. Older rounds are a one-line marker. A parallel batch is read back whole.
3. **Schemas on demand.** A stable prefix list every turn. Full schemas only after the model enables that prefix.
4. **One completion at fold.** Facts and voice. No second “store everything” turn.
5. **Two tests.** A golden of the request body on every push. A live fixture that fails when the tool, the arguments, or the memory row are wrong. Point the first live fixture at the Llama you mean to ship, and treat a Flash-only green run as unread for that model.

## What not to take from goose

Session-title generation, auto-compaction through a summarizer, tool-pair summarization, Smart Approve’s judge call, and the tool shim. Each one is an extra completion. The shim is a different model. None of them repair `extension__tool`.
