// Command george is the coding assistant. One process, one local model, stdin/stdout.
//
//	george run         — start a session in this repo (default)
//	george init        — scaffold persona + mcp.toml from examples/
//	george tools-plan  — JSON release/command inventory from mcp.toml
//	george tools-fetch — download + install MCP binaries from mcp.toml
//	george version     — build info
package main

import (
	"fmt"
	"os"
)

// Set via -ldflags at release build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		os.Exit(run())
	case "init":
		os.Exit(initCmd())
	case "tools-plan":
		os.Exit(toolsPlanCmd())
	case "tools-fetch":
		os.Exit(toolsFetchCmd())
	case "version":
		fmt.Printf("george %s (commit=%s date=%s)\n", version, commit, date)
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `george — coding assistant. One process, one local model, stdin/stdout.

Usage:
  george [run]        Start a session in this repo (default)
  george init         Write env, mcp.toml, and persona to ~/.config/george (skips existing)
  george tools-plan   JSON MCP binary inventory from mcp.toml
  george tools-fetch  Download + install MCP binaries declared in mcp.toml
  george version      Print build info
  george help         Show this help

Paths (env overrides, optional):
  GEORGE_CONFIG_DIR  default ~/.config/george (env, mcp.toml, PERSONA.md, SELF.md)
  DATA_DIR           default ~/.local/share/george (george.db)
  MCP_MANIFEST       default $GEORGE_CONFIG_DIR/mcp.toml
  PERSONA_DIR        default $GEORGE_CONFIG_DIR
  GEORGE_ROOT        default git toplevel of the cwd
MCP binaries from tools-fetch go to ~/.local/share/george/bin, which george
puts first on PATH. The process env wins over ~/.config/george/env.
`)
}
