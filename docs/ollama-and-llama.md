# Ollama and Llama

A Llama model is available as the bundled local backend. The live Ollama regression does not use one.

## Two ways to run a local model

**Ollama provider** (`crates/goose-providers/src/ollama.rs`, wired by `crates/goose/src/providers/ollama_def.rs`). Talks to an Ollama server. Default host is `localhost`, port `11434`, unless `OLLAMA_HOST` is a full URL. The default model constant is the string `qwen3` (`OLLAMA_DEFAULT_MODEL`). The known-name list is `qwen3`, `qwen3-vl`, `qwen3-coder:30b`, and `qwen3-coder:480b-cloud`. None of those pins a parameter count and a quantization together. There is a separate `ollama_cloud` provider.

**Bundled llama.cpp** (`crates/goose-local-inference`, feature `local-inference`). The provider name is `local`. Inference goes through `llama-cpp-2` in `llamacpp/`. Optional accelerators are feature flags: `cuda`, `vulkan`, `mlx`. This path loads a GGUF. It does not call Ollama.

## Regression that hits a real model

`cargo test` does not call Ollama or llama.cpp. GitHub workflows compile with libclang for `llama-cpp-sys-2`. They do not start Ollama or download a GGUF.

### Ollama

`crates/goose/tests/providers.rs`, `test_ollama_provider`:

```text
provider: Ollama
model string: qwen3
image model string: qwen3-vl
required env: OLLAMA_HOST
context probe: 50_000 tokens
expect context-length error: false
smart approve: false
```

`qwen3` and `qwen3-vl` are family tags. The test does not name a size, a quant, or a digest. The docs that say a local Qwen works with extensions name `qwen2.5` (`documentation/docs/troubleshooting/known-issues.md`, and `ollama run qwen2.5` in `documentation/docs/getting-started/providers.md`). The LM Studio example is `qwen2.5-7b-instruct`. The custom-distro example is `qwen3-coder:latest`. The XML fallback comment names `qwen3-coder` and `qwen3-coder-32b` as models that stop emitting native tool calls. Those strings do not match each other, and they do not match `qwen3-coder:30b` on the known-name list.

The test skips when `OLLAMA_HOST` is unset, or when provider setup fails. With the host set, one process runs, in Auto unless noted:

- list models
- basic completion
- MCP tool use
- image content (`qwen3-vl`)
- context-length probe (failure is not required)
- Approve mode: allow and deny
- mode switch from Auto to Approve

Smart Approve is off, so this run does not pay the permission-judge completion. Tool shim is not enabled here.

Run (after `source bin/activate-hermit`):

```bash
OLLAMA_HOST=http://127.0.0.1:11434 cargo test -p goose --test providers test_ollama_provider -- --nocapture
```

The model must already be pulled in that Ollama. The test does not pull it.

### llama.cpp

`crates/goose/tests/local_inference_integration.rs` is `#[ignore]`. Default model:

```text
bartowski/Llama-3.2-1B-Instruct-GGUF:Q4_K_M
```

Override with `TEST_MODEL`. Download first:

```bash
goose local-models download bartowski/Llama-3.2-1B-Instruct-GGUF:Q4_K_M
cargo test -p goose --test local_inference_integration -- --ignored --nocapture
```

Cases: stream produces text and usage, a large prompt completes, and a vision case. Vision runs only when `TEST_VISION_MODEL` is set. The documented example is `unsloth/gemma-4-E4B-it-GGUF:Q4_K_M`, which is not a Llama model.

`crates/goose/tests/local_inference_perf.rs` is the same idea for timing: two `complete` calls, also ignored.

## Gap if Llama is the model we mean

| Path | Model the test uses |
| --- | --- |
| Ollama live provider test | `qwen3`, images on `qwen3-vl` |
| Tool shim, when enabled | `mistral-nemo` unless overridden |
| Ignored local-inference test | `Llama-3.2-1B-Instruct` Q4_K_M |
| Ignored vision test | Gemma, and only if `TEST_VISION_MODEL` is set |

`goose-self-test.yaml` is a recipe the running agent executes against itself (files, shell, extensions, delegation). It is not an Ollama or Llama regression.
