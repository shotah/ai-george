# Tool-call dialect

Feature. Not built. The loop still speaks OpenAI chat completions, and a
printed call is still salvaged as Hermes JSON. This is the string the
weights actually saw. The change, step by step, is
[tool-call-plan.md](tool-call-plan.md).

The graded model in [eval setup](eval_setup.md) is
`qwen3.6:35b-a3b-coding`. The tags on this machine are
`qwen3.6:35b-a3b-mtp-q8_0` and `george-q8` (same weight blob,
`qwen35moe`, 35.5B). Both Modelfiles say `RENDERER qwen3.5` and
`PARSER qwen3.5` (Ollama 0.35.0). Two public pages describe an older
binding of the same family:

- Qwen3 tool calling: [concepts.md](https://github.com/QwenLM/Qwen3/blob/main/docs/source/getting_started/concepts.md#tool-calling)
- Hermes function calling: [prompt format](https://github.com/NousResearch/Hermes-Function-Calling#prompt-format-for-function-calling)

Qwen3's own page says the template is akin to Hermes. That is why a Hermes
system block lands on a Qwen3 chat model. The weights were trained on that
prompt. It is the same fact as the page, read from the other side.

The tag we grade is one generation later. Matching the Hermes page on
`qwen3.6` would be matching the wrong call.

## What Qwen3.6-35B-A3B speaks

The string is the chat template shipped with the weights. Primary sources,
in the order that settles an argument:

| Source | What it settles |
| --- | --- |
| [`Qwen/Qwen3.6-35B-A3B` `chat_template.jinja`](https://huggingface.co/Qwen/Qwen3.6-35B-A3B/blob/main/chat_template.jinja) | The prompt the tokenizer builds, including the tool instructions and how history is replayed |
| [Model card](https://huggingface.co/Qwen/Qwen3.6-35B-A3B) | vLLM and SGLang are told to use `--tool-call-parser qwen3_coder` and `--reasoning-parser qwen3` |
| [Qwen3.6 #125](https://github.com/QwenLM/Qwen3.6/issues/125) | Qwen staff: this family natively emits the XML. The `qwen3_coder` parser turns that XML into API JSON. A prompt that forces JSON conflicts with the template, and `qwen3_coder` cannot parse raw JSON |
| [Qwen3.6 #178](https://github.com/QwenLM/Qwen3.6/issues/178) and [Ollama #16383](https://github.com/ollama/ollama/issues/16383) | The same template, plus the stray `</function_invocation>` the model sometimes adds |
| These Modelfiles | `RENDERER qwen3.5` / `PARSER qwen3.5` on `qwen3.6:35b-a3b-mtp-q8_0` and `george-q8` |

The template's own instruction, copied from `chat_template.jinja`:

```text
<|im_start|>system
# Tools

You have access to the following functions:

<tools>
{"type": "function", "function": {"name": "fs__file_get", "description": "…", "parameters": {"type": "object", "properties": {"path": {"type": "string"}}, "required": ["path"]}}}
</tools>

If you choose to call a function ONLY reply in the following format with NO suffix:

<tool_call>
<function=example_function_name>
<parameter=example_parameter_1>
value_1
</parameter>
<parameter=example_parameter_2>
This is the value for the second parameter
that can span
multiple lines
</parameter>
</function>
</tool_call>

<IMPORTANT>
Reminder:
- Function calls MUST follow the specified format: an inner <function=...></function> block must be nested within <tool_call></tool_call> XML tags
- Required parameters MUST be specified
- You may provide optional reasoning for your function call in natural language BEFORE the function call, but NOT after
- If there is no function call available, answer the question like normal with your current knowledge and do not tell the user about function calls
</IMPORTANT>
```

`tool | tojson` is what lands inside `<tools>`. On an OpenAI-compatible
server that object is the function tool we already send. The first system
message we wrote is appended after `</IMPORTANT>`, still inside that one
system turn. A second system message is an error in this template
(`System message must be at the beginning`).

A call the model is told to emit, and the shape the template writes back
when replaying `tool_calls`:

```text
<tool_call>
<function=fs__file_get>
<parameter=path>
readme.md
</parameter>
</function>
</tool_call>
```

A string parameter is the raw characters. A list or object parameter is
JSON (`tojson`). Several calls are several `<tool_call>` blocks in one
assistant turn. Prose is allowed before the first block. The instruction
says no suffix after it.

A tool result is not Hermes's `{"name","content"}` and not a `tool` role
in the rendered prompt. The template turns each `role: tool` row into a
user turn:

```text
<|im_start|>user
<tool_response>
range: 1-1 of 1
hello
</tool_response>
<|im_end|>
```

Consecutive tool rows stay inside that one user turn. There is no call id
in any of this.

The template walks `tool_call.arguments` with `|items`, so at render time
the arguments are a dict. A JSON string there is the failure
`Can only get item pairs from a mapping`
([model discussion](https://huggingface.co/Qwen/Qwen3.6-35B-A3B/discussions/30)).
OpenAI's wire stores arguments as a string. The server has to parse that
string into a dict before this template runs. Ollama's `qwen3.5` renderer
does that. A server that feeds the string straight into the jinja does not.

## What we change

The name stays `fs__file_get`. The agent loop keeps `ToolCall`, `role: tool`,
and `tool_call_id`. Those are our bookkeeping. The model never sees them
when the renderer is doing its job. Three of our paths still show the model
a Hermes object, or fail to read the XML it was trained to write.

1. **Printed calls.** `ParseToolCallText` strips `<tool_call>` and then
   looks for Hermes JSON. After the strip, a real Qwen3.6 call is
   `<function=fs__file_get><parameter=path>…</parameter></function>`, which
   we drop. Parse that block into `name` plus a JSON object of parameters.
   A string value stays a string. Also accept the drifted form that adds a
   spare `</function>` and a `</function_invocation>` close
   ([#178](https://github.com/QwenLM/Qwen3.6/issues/178)).
2. **The constrained retry.** `forcedToolCallSchema` is
   `{"name","arguments"}`. Ollama omits `tool_calls` under
   `response_format`, so the model has to answer in that JSON while its
   template says XML only, and the `qwen3_coder` parser cannot read the
   JSON ([#125](https://github.com/QwenLM/Qwen3.6/issues/125)). The grammar
   for this model is one `<function=` block whose name is the candidate
   enum. Keep the Hermes schema only for a server whose template is the
   Qwen3 concepts page.
3. **Do not add a second catalog.** With `tools` on the request, Ollama
   already writes the `# Tools` turn. Our system text is appended inside
   it, which is why the fold to one leading system message matches this
   template. A `# Tools` block of our own would be a second copy.

Leave the OpenAI `tools` array and the `role: tool` result rows in place
for Ollama. That is the input the `qwen3.5` renderer turns into the
template above. Rendering the template ourselves is the path for a server
that has no `qwen3.5` renderer: omit `tools`, put the block above in the
one system message, replay calls as `<function=` XML with arguments as a
dict, and replay results as one user message of `<tool_response>` bodies.

## Two generations, one catalog

| | Hermes 2 Pro | Qwen3 (the concepts page) | Qwen3.5 / 3.6 (this tag) |
| --- | --- | --- | --- |
| Catalog | `<tools>` plus a list of OpenAI function objects, and a pydantic `FunctionCall` schema in the system turn | `<tools>` plus one JSON schema per function, under a `# Tools` heading | `<tools>` plus the same OpenAI function objects, under `# Tools`. The pydantic paragraph is gone |
| Call | `<tool_call>{"name","arguments":{…}}</tool_call>` | The same object. Arguments are an object, a string is the wrong type | `<tool_call><function=name><parameter=key>value</parameter></function></tool_call>` |
| Result turn | role `tool`, body `{"name","content"}` inside `<tool_response>` | role `user`, raw body inside `<tool_response>` | role `user`, raw body inside `<tool_response>`. Consecutive tool messages join into one user turn |
| Call id | none in the prompt | none | none |
| Parallel | several `<tool_call>` blocks | several `<tool_call>` blocks | several `<tool_call>` blocks in one assistant turn |
| Planner | Hermes-3 adds `<scratch_pad>` | `<think>` on the final round, and on a multi-step tool call | The renderer writes the think block. The gate runs with thinking off |

The part that survived from Hermes into Qwen3.6 is the catalog and the
result turn: a list of OpenAI function objects in the system message, and
the tool body handed back as a user message of `<tool_response>` tags.
The call syntax moved. Qwen3 emits a JSON object. Qwen3.6 emits
`<function=` / `<parameter=` with `</parameter>` and `</function>` closed,
which is the coder XML. The
[Qwen3.6-35B-A3B model card](https://huggingface.co/Qwen/Qwen3.6-35B-A3B)
tells vLLM and SGLang to use `--tool-call-parser qwen3_coder`. Ollama
does not register that name on these tags. It registers parser `qwen3.5`,
and that parser reads the same XML. The
[`qwen35` renderer](https://github.com/ollama/ollama/blob/main/model/renderers/qwen35.go)
is what writes it back out on the next turn. An older Ollama build had
mapped the name `qwen3.5` at the Hermes JSON renderer; that is the wrong
string for this family, and it is not what these Modelfiles select.

The call this model emits is the `<function=` block in
[What Qwen3.6-35B-A3B speaks](#what-qwen36-35b-a3b-speaks). The concepts
page emits `{"name","arguments"}` inside `<tool_call>`. The tool name
stays `{server}__{tool}` either way ([mcp-naming.md](mcp-naming.md)).

## What we send today

`provider.Client` posts an OpenAI body: a `tools` array, assistant
`tool_calls` whose `arguments` are a JSON string, and `role: tool` rows
keyed by `tool_call_id`. Session rows, round collapse, and the loop guard
all key off that id. The agent loop in [architecture.md](architecture.md)
is that shape.

Ollama is the translator. With `tools` set, the `qwen3.5` renderer
injects the `# Tools` system turn, rewrites each `tool_calls` entry into
`<function=` XML, and folds each run of `role: tool` into one user turn of
`<tool_response>` blocks. The id never reaches the model. On the way back
the parser fills `tool_calls` and we keep our own ids.

That translation fails open in the two places listed under
[What we change](#what-we-change): the constrained retry still demands
Hermes JSON, and `ParseToolCallText` still cannot read `<function=`.
`stripDanglingToolTags` only drops a trailing empty `<tool_call>`.

## Alias

Keep `ToolCall`, `role: tool`, and the ids. Teach the provider boundary
two dialects. The agent loop stays. This is the same job the Ollama
renderer does, moved to where we can see it when the renderer is absent
or the parser drops the call.

| Dialect | When | Call we parse and, if we render, emit |
| --- | --- | --- |
| `qwen35` | model id contains `qwen3.5` or `qwen3.6`, which is the graded tag | `<function=` / `<parameter=` inside `<tool_call>` |
| `qwen3` | a server whose template is the concepts page | Hermes JSON inside `<tool_call>`, arguments an object |

Pick with one env, `LLM_TOOL_DIALECT`, defaulted from the model id as in
the table. Empty means today's behavior: trust the server's renderer,
salvage Hermes JSON.

Two placements, both aliases of the same internal call:

1. **Parse-only, while `tools` is on the request.** This is the Ollama
   path. Leave rendering to the `qwen3.5` renderer. When `tool_calls` is
   empty, parse `<function=` out of `content` the way `ForceToolNames`
   already parses Hermes JSON, synthesize `call_text_` ids, and clear the
   tags from the visible reply. Hermes JSON salvage stays behind it for
   the constrained retry, which still forces that object.
2. **Render ourselves, when the server has no qwen renderer.** Omit the
   `tools` array so nothing injects a second catalog. Fold in the same
   place `WireMessagesMode` folds system text. The system message gains
   the `# Tools` block for the chosen dialect. Assistant `ToolCalls`
   become the call text. A run of `role: tool` rows becomes one user
   message of `<tool_response>` blocks, raw body, call order, no id and
   no `{"name","content"}` wrapper. On the way back, parse the dialect
   into `ToolCall` and keep storing `role: tool` with synthesized ids, so
   collapse and the loop guard do not change.

The loose salvage (`parameters`, fn-style kwargs, args-only fences) stays
a recovery. The prompt asks for one shape. A real answer that mentions a
tool name is still not a call. `mostlyToolArgDump` is the rule for that.

## Match

Store the trained string in the agent. Drop `role: tool`. Assistant
history is the `<tool_call>` text. Results are user messages. The loop,
the session, and collapse would key off something other than
`tool_call_id`.

That emits the same bytes as the render-ourselves alias and rewrites
every caller of `ToolCallID`. Do it only if the OpenAI client goes away.
Until then the alias is the whole feature.

## Leave alone

- Hermes-3 `<scratch_pad>`. Qwen3.6 was not trained on it. Thinking stays
  `LLM_REASONING_EFFORT=none`.
- Tool names. `fs__file_get` is already the name inside `<function=>`.
- A second copy of the catalog beside `tools`.
- Asking the model for a `tool_call_id`. We mint ids when we parse.
- Putting a `<think>` block back into history. The renderer owns that,
  and the gate runs with it off.

## Check

A parser test for the graded shape: one `<tool_call>`, two in one
assistant message, a missing `</function>`, a parameter value that
contains `{`, and a prose reply that mentions `fs__file_get` and is left
as prose. The Hermes JSON cases in `toolcalltext_test.go` stay green.

Pin one rendered tool round next to
`internal/agent/testdata/stdio/wire.txt`: an assistant call, two tool
results, the next completion. The rendered string should match what the
`qwen3.5` renderer emits for that round (catalog once, results in one
user turn, arguments as parameter text).

Then the existing gate, not a new suite. Iterate
`mangled_names`, `parallel_reads`, and `stop_when_done` at `-eval.n=1`,
then stamp at `-eval.n=3`. The salvage check is a turn whose content is
the `<function=` block and whose `tool_calls` field is empty: it runs.
