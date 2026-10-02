# Extra model calls

The user turn is one `Provider::stream` (or `complete`) on the configured model. Several other paths issue their own completion. Most of them reuse that same model. A few can call a second model.

## On by default

**Session title.** `SessionManager::maybe_update_name` calls `generate_session_name`, which calls `complete_one_shot` on the session provider. The agent does this from both reply paths when session naming is enabled (`crates/goose/src/agents/agent.rs`). It runs while the visible user-message count is at most 3 (`MSG_COUNT_FOR_SESSION_NAME_GENERATION`). Providers that manage their own context name from the first user message, and some of those build the title locally with no model call (`uses_local_session_naming`). Recipe sessions copy the recipe title. Scheduled sessions and user-renamed sessions skip the call.

**Auto-compaction.** Before a reply, `check_if_compaction_needed` can run a summarizer completion through `goose-context-management` (`summarize.rs` → `CompactionModel::complete`). The threshold is `GOOSE_AUTO_COMPACT_THRESHOLD`. This is another completion on the session model, sent because the history is long.

## Off unless configured

**Tool shim.** `GOOSE_TOOLSHIM` defaults to false (`global_toolshim` in `crates/goose/src/model_config.rs`). When it is on, the interpreter is a second completion. The Ollama interpreter defaults to `mistral-nemo` (`DEFAULT_INTERPRETER_MODEL_OLLAMA`). Override with `GOOSE_TOOLSHIM_OLLAMA_MODEL` (docs mention `llama3.2`). `GOOSE_TOOLSHIM_BACKEND=local` (or `llama.cpp`) sends that call through the bundled llama.cpp backend and requires `GOOSE_TOOLSHIM_MODEL` or `LOCAL_LLM_MODEL`.

**Smart Approve.** In `GooseMode::SmartApprove`, `permission_inspector` calls `detect_read_only_requests`, which is `provider.complete` with the permission-judge prompt. The Ollama provider regression turns this off (`.test_smart_approve(false)`).

**Adversary inspector.** A completion against `~/.config/goose/adversary.md` rules. Absent file means the inspector is disabled. Failure allows the tool call.

**Platform tools.** These complete only when the tool is invoked:

- `summarize` reads files and calls `provider.complete`
- apps extension calls `complete_app_content`, and if `model_config.toolshim` is set it then runs the tool shim on that response
- orchestrator mode `summarize` calls the LLM; mode `first_last` does not

**Dictation.** Separate from the chat model. Providers include OpenAI, ElevenLabs, Groq, a model-native endpoint, and, behind the `local-inference` feature, a local Whisper transcriber.

**Doctor and configure.** `goose doctor` and provider setup send a short `complete` ("Say hello") to prove the provider works. `goose info` does the same.

## Ollama HTTP that is not a generation

`OllamaProvider` GETs `/api/tags` when listing models. It POSTs `/api/show` once per model to read `thinking` and `vision`, then caches that. Thinking effort set to off skips `/api/show`. These are metadata calls. There is no `/api/embed` client in this provider.

## What a Llama-only turn still pays

With Ollama or local llama.cpp, tool shim off, Smart Approve off, no adversary file, and no summarize/apps tool: the extra generation that still happens is the session title (up to the first three user messages) and auto-compaction once the context threshold is crossed. Both use the session model.
