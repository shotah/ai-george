# examples/

Scaffolds that `george init` copies onto the machine you are coding on.
The binary talks on stdin/stdout (`CHANNEL=stdio` when unset). Persona,
`mcp.toml`, and `.env` live next to that process.

| Path | Role |
| --- | --- |
| [`persona/PERSONA.example.md`](persona/PERSONA.example.md) | Who it should be |
| [`persona/SELF.example.md`](persona/SELF.example.md) | Agent-written voice. Seed only; the running copy is `SELF.md` |
| [`mcp.toml.example`](mcp.toml.example) | Optional MCP grants. Empty until you uncomment a server |
| [`env.example`](env.example) | `LLM_*` and the rest of the process env |

`examples/embed.go` bakes those four into the binary. `george init` writes
them under `deploy/persona/`, `deploy/mcp.toml`, and `.env.example`, and
skips any file that already exists.

```bash
make init
cp .env.example .env
# set LLM_BASE_URL, LLM_API_KEY, LLM_MODEL
make run
```
