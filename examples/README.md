# examples/

Scaffolds that `george init` copies into `~/.config/george/` on the
machine you are coding on.

| Path | Role |
| --- | --- |
| [`persona/PERSONA.example.md`](persona/PERSONA.example.md) | Who it should be |
| [`persona/SELF.example.md`](persona/SELF.example.md) | Agent-written voice. Seed only; the running copy is `SELF.md` |
| [`mcp.toml.example`](mcp.toml.example) | Optional MCP grants. The coding loop (`fs`, `git`, `shell`, `github`) is listed. The other servers stay commented |
| [`env.example`](env.example) | `LLM_*` and the rest of the env. Written as `env` (mode 0600) |

`examples/embed.go` bakes those four into the binary. `george init` writes
`PERSONA.md`, `SELF.md`, `mcp.toml`, and `env` into `~/.config/george/`
(`GEORGE_CONFIG_DIR` overrides it) and skips any file that already exists.

```bash
george init
$EDITOR ~/.config/george/env   # LLM_BASE_URL, LLM_API_KEY, LLM_MODEL
george tools-fetch
cd your-repo && george
```
