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
  george init         Scaffold persona + mcp.toml (+ .env.example) from embedded templates
  george tools-plan   JSON MCP binary inventory from mcp.toml
  george tools-fetch  Download + install MCP binaries declared in mcp.toml
  george version      Print build info
  george help         Show this help

init / tools-* env (optional):
  PERSONA_DIR    default deploy/persona
  MCP_MANIFEST   default deploy/mcp.toml (tools-* default: mcp.toml)
`)
}
