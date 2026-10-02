# george

![george — the crane. A long-horizon AI harness around one model. Nothing listens.](https://raw.githubusercontent.com/shotah/george/main/assets/banner.png)

**The product is the crane.** Pull this image, point it at a local model or paste an
API key, and chat from your phone. No dashboard. No config UI. **No open ports.**
This image is an **AI harness** for **long-horizon planning** — memory, cron,
and personality survive `/new`.

```text
container + persona + any OpenAI-compat LLM  →  outbound chat
```

Chat, memory, cron, and web search work with **zero MCP servers**. Tools are optional
binaries on `PATH` (or baked into a richer image).

Source, full contract:
[github.com/shotah/george](https://github.com/shotah/george)

---

## Quick start

```bash
docker pull shotah/george:latest
docker run --rm shotah/george:latest version
```

The binary runs on the machine you are coding on. From a checkout:

```bash
git clone https://github.com/shotah/george.git && cd george
make init
cp .env.example .env
# set LLM_BASE_URL, LLM_API_KEY, LLM_MODEL
make run
```

Walkthrough: [readme.md](https://github.com/shotah/george/blob/main/readme.md)

---

## Chat is the console

No dashboard, no inbound ports — ops and MCP login live in Telegram (or
Discord / Slack / stdio).

| Command | Use it for |
| --- | --- |
| `/status` `/perf` `/memstats` `/toolstats` `/tokens` | Health, trajectory, memory, MCP timing, prompt size |
| `/tools` `/examples` `/planner` `/aims` `/new` `/cancel` `/help` | Catalog, capability ideas, daily planning session, aim ledger, reset, abort, command list |
| `/auth` | **Headless OAuth** (Google / Strava / …) — no laptop `localhost` callback |

Guide: [docs/auth.md](https://github.com/shotah/george/blob/main/docs/auth.md)

---

## Tags

| Tag | Meaning |
| --- | --- |
| `latest` | Last successful publish (`main` or a `v*` tag) |
| `edge` | Tip of `main` (moving) |
| `0.x.y` / `0.x` | Pinned release (prefer for production) |
| `sha-<commit>` | Exact CI build |

Also on GHCR: `ghcr.io/shotah/george` (same tags). Multi-arch: `linux/amd64`,
`linux/arm64`.

---

## What this image is

- **AI harness only** — Distroless `static-debian12:nonroot`, no shell, no MCP
  tools baked in. Long-horizon pieces (memory, cron, watches, `SELF.md`) are
  in this binary.
- Entrypoint: `george` (`run` | `status` | `version` | …)
- Healthcheck: `["CMD","/usr/local/bin/george","status"]` (exit code; **no port**)
- Config: env + bind mounts (`PERSONA_DIR`, `MCP_MANIFEST`, `DATA_DIR`)
- Channels: `stdio` (default), or Telegram, Discord, Slack, pendant

MCP tool binaries are **not** baked in. Grant tools by mounting static
binaries and uncommenting `mcp.toml`. Persona and env scaffolds live in the
source tree (`examples/`).

---

## Minimal compose

```yaml
services:
  george:
    image: shotah/george:latest
    env_file: .env
    environment:
      LLM_BASE_URL: https://generativelanguage.googleapis.com/v1beta/openai
      LLM_API_KEY: ${GEMINI_API_KEY}
      LLM_MODEL: gemini-3.5-flash
      CHANNEL: stdio
      PERSONA_DIR: /persona
      DATA_DIR: /data
      MCP_MANIFEST: /etc/george/mcp.toml
    volumes:
      # Persona writable for SELF.md (self_note + /new distill). Use :ro only with SELF_NOTES_ENABLED=false.
      - ./persona:/persona
      - ./mcp.toml:/etc/george/mcp.toml:ro
      - ./data:/data
    healthcheck:
      test: ["CMD", "/usr/local/bin/george", "status"]
      interval: 60s
      timeout: 10s
      retries: 3
```

Nothing publishes a host port. Health is `george status`. The process dials the model.

---

## Who it’s for

Anyone who wants to run a long-horizon personal agent in Docker and drop in a
local model or an API key. Tools are optional. Nothing listens on a port.
The image is the harness; you bring the model and the chat token.

| Pick this image when… | Pick something else when… |
| --- | --- |
| You want `docker compose up` and a bot on your phone | Several agents + a board → [gantree](https://github.com/shotah/gantree) |
| Allowlist + no inbound ports | You need WhatsApp / Teams webhooks |
| Env + mounts is enough config | You want a dashboard or no-code canvas |

---

## Docs

| Topic | Link |
| --- | --- |
| Docker deploy | [deploy-docker.md](https://github.com/shotah/george/blob/main/docs/deploy-docker.md) |
| Chat `/auth` (headless OAuth) | [auth.md](https://github.com/shotah/george/blob/main/docs/auth.md) |
| Native + Ollama | [deploy-native.md](https://github.com/shotah/george/blob/main/docs/deploy-native.md) |
| MCP host | [mcp.md](https://github.com/shotah/george/blob/main/docs/mcp.md) |
| Work after the fork | [todo.md](https://github.com/shotah/george/blob/main/docs/todo.md) |
| Hello path | [readme.md](https://github.com/shotah/george/blob/main/readme.md) |
| Harness contract | [design.md](https://github.com/shotah/george/blob/main/docs/design.md) |

License: MIT — [LICENSE](https://github.com/shotah/george/blob/main/LICENSE)

---

## Hub metadata (maintainers)

This file is what CI publishes to the Docker Hub **overview**
(`.github/workflows/dockerhub-description.yml`). Keep it pull-first and under
Hub’s ~25KB cap. Do **not** paste the full root `readme.md` here.

**Categories** (Hub UI only — pencil under the short description, max 3):

1. **Machine learning & AI**
2. **Developer tools**
3. **Security** *(outbound-only / allowlist story)*

**Short description** is set by the same workflow (≤100 chars):
`Long-horizon AI harness. Local model or API key. Telegram/Discord/Slack. No open ports.`
Banner must be **PNG** with an absolute `raw.githubusercontent.com` URL — Hub
does not render our SVG reliably.
