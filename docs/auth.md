# Auth

george has no auth flow of its own. The assistant build had a chat `/auth`
command, a `george auth <server>` subcommand, and a GitHub Pages catch page
for OAuth redirects. All three were removed with the chat channels
([coding-agent-plan.md](coding-agent-plan.md#what-comes-out)). `docs/oauth-catch/`
is the leftover static page; nothing in this binary points at it.

What the four coding servers need:

| Server | Credential | Where |
| --- | --- | --- |
| `fs`, `git`, `shell` | none | They run as you, rooted at `GEORGE_ROOT` |
| `github` | `GITHUB_TOKEN` or `GH_TOKEN` | `~/.config/george/env`, or your shell. With neither set, the server is skipped at boot and the other three stay up |

The model endpoint takes `LLM_API_KEY` from the same env file. For a local
Ollama any non-empty string works.

Another MCP server that needs a login runs its own command for it. Do that
in your shell before you start george, with whatever the server documents.
`auth_command` and `auth_args` are still accepted in `mcp.toml` so an older
manifest loads, but george does not run them.
