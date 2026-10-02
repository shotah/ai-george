// Package examples embeds operator templates for `george init`.
// Keep files here as the source of truth; init copies them to config.Dir().
package examples

import "embed"

// FS holds persona templates and the sample MCP manifest.
//
//go:embed persona/*.example.md mcp.toml.example env.example
var FS embed.FS
