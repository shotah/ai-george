# Plan: match Qwen3.6 tool calls

The contract for the string is [tool-dialect.md](tool-dialect.md). This is
the change to the harness, in order, and what it costs.

It is not a wording change. Tool names, descriptions, `PERSONA.md`, and the
MCP children stay. `{server}__{tool}` is already a legal `<function=` name.

It is not a new agent loop. Session rows, round collapse, the loop guard,
and `tool_call_id` stay. Ollama's `qwen3.5` renderer already turns our
OpenAI `tools` array and `role: tool` rows into the
[Qwen3.6-35B-A3B chat template](https://huggingface.co/Qwen/Qwen3.6-35B-A3B/blob/main/chat_template.jinja).
The happy path, where that parser fills `tool_calls`, is already the dialect
the weights saw. We leave that request alone.

The break is the two paths where we still demand Hermes JSON, or fail to
read the XML the template told the model to write.

## What we gain

- A call printed as `<function=fs__file_get><parameter=path>…` runs, instead
  of becoming the visible reply.
- Several of those blocks in one message run as one batch, which is how the
  template writes parallel calls.
- The bad-name retry stops asking for `{"name","arguments"}`. Qwen staff
  say a prompt that forces JSON conflicts with this template, and
  `qwen3_coder` cannot parse that JSON
  ([Qwen3.6 #125](https://github.com/QwenLM/Qwen3.6/issues/125)).
- Hermes JSON salvage stays, so a printed `{"name","arguments"}` still runs.

## What we lose

- The token mask on the bad-name retry. Today `ForceToolNames` sends a JSON
  schema, Ollama compiles it to a grammar, and a name outside the candidate
  list cannot be sampled. That schema is the Hermes object. There is no
  `response_format` on our OpenAI client that means "one `<function=` block".
  Replacing it means the next sample can still invent a name. The tool error
  and the closest-name hint stay. They are the same text the model already
  gets back as a tool result.
- Nothing in the loop, memory, collapse, naming, or the catalog. We do not
  add a second `# Tools` block.
- An Ollama 500 on a drifted call. A spare `</function_invocation>` can make
  the `qwen3.5` parser reject the turn before we see `content`
  ([Ollama #16383](https://github.com/ollama/ollama/issues/16383)). Salvage
  never runs on a 500. That fix is an Ollama upgrade, not this change.
- We do not start rendering the chat template ourselves. A server with no
  `qwen3.5` renderer is out of this plan.

## Steps

Each step is done when its test is green. The eval is last.

### 1. Parse the XML

In `internal/provider/toolcalltext.go`, before the Hermes JSON parse, pull
every `<tool_call>…</tool_call>` whose body is `<function=name>` plus
`<parameter=key>value</parameter>`.

- The name is the text after `<function=`.
- A parameter value is the raw text, trimmed of one leading and one trailing
  newline, matching the coder parser. `readme.md` stays a string. A value
  that is JSON (`{…}` or `[…]`) is decoded.
- Several blocks become several calls, in order.
- A spare `</function>` and a `</function_invocation>` close are skipped,
  not a reason to reject the block
  ([Qwen3.6 #178](https://github.com/QwenLM/Qwen3.6/issues/178)).
- A reply that mentions `fs__file_get` in prose and has no `<function=`
  block is not a call. The existing Hermes cases in `toolcalltext_test.go`
  stay green.

`ParseToolCallText` today returns one call. Add a parse that returns the
list. The one-call helper keeps returning the first, so current callers
do not change in this step.

### 2. Run the whole batch

`salvageToolCall` in `internal/agent/agent.go` returns one `ToolCall`. A
message with two `<tool_call>` blocks would run the first and drop the
second. Return the list from step 1, keep the same name check (a `__`
name, or a name in the published defs), and set `res.ToolCalls` to that
list. Clear `res.Content`, as the one-call path already does, so the XML
is not also the reply.

`stripDanglingToolTags` stays as it is: a trailing empty `<tool_call>` on
a real answer is still stripped. Do not strip `<function=` out of prose.

### 3. Stop forcing Hermes JSON on the retry

`forcedToolCallSchema` in `internal/provider/provider.go` is
`{"name","arguments"}`. `toolround.go` sets `ForceToolNames` from the
closest-name candidates, and the next `Complete` sends that schema.
Ollama then omits `tool_calls`, and the model has to answer in the JSON
its own template forbids.

For a `qwen3.6` / `qwen3.5` model id, do not set `response_format`. Leave
`Tools` on the request. The failed call's error string is already the
next user-visible tool result, and it already names the candidates. The
following sample can emit `<function=>`, and steps 1–2 run it.

Keep `forcedToolCallSchema` for a model id that is not this family. The
constrained-retry tests that pin a JSON body stay on that path. Add one
test that a `qwen3.6` model id sends `tools` and no `response_format`.

This is the loss in the list above. Do not replace it with a hand-built
GBNF in this change.

### 4. Gate

`go test ./internal/provider/ ./internal/agent/`.

Then the live gate, narrow first:

```sh
make integration-test EVAL_ARGS='-eval.n=1 -eval.only=mangled_names'
make integration-test EVAL_ARGS='-eval.n=1 -eval.only=parallel_reads'
make integration-test EVAL_ARGS='-eval.n=1 -eval.only=stop_when_done'
```

`mangled_names` is the retry. It is allowed to spend a sample on the hint
instead of a masked name. `parallel_reads` is the batch. `stop_when_done`
is the turn that must not keep calling. Stamp at `-eval.n=3` only after
those three are clean at `-eval.n=1`.

## Not in this change

- Renaming tools, rewriting descriptions, or editing the contract prompt.
- A second `# Tools` system block beside the `tools` array.
- Storing `<tool_call>` text in the session in place of `role: tool`.
- Rendering `chat_template.jinja` in our process.
- An Ollama-native grammar for the retry.
