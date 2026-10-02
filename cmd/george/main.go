// Command george is the coding assistant. One process, one local model, stdin/stdout.
//
//	george run         — start the daemon (default)
//	george init        — scaffold persona + mcp.toml from examples/
//	george auth        — run an MCP server's declared auth flow (mcp.toml)
//	george tools-plan  — JSON release/command inventory from mcp.toml
//	george tools-fetch — download + install MCP binaries from mcp.toml
//	george status      — exit-code healthcheck + JSON doctor (Docker healthcheck)
//	george doctor      — alias of status
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
	case "auth":
		os.Exit(authCmd())
	case "tools-plan":
		os.Exit(toolsPlanCmd())
	case "tools-fetch":
		os.Exit(toolsFetchCmd())
	case "status", "doctor":
		os.Exit(status())
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
  george [run]        Start the daemon (default)
  george init         Scaffold persona + mcp.toml (+ .env.example) from embedded templates
  george auth         Run MCP auth flows declared in mcp.toml (george auth [server])
  george tools-plan   JSON MCP binary inventory from mcp.toml
  george tools-fetch  Download + install MCP binaries declared in mcp.toml
  george status       Exit 0 if alive (heartbeat). JSON doctor on stdout
  george doctor       Alias of status
  george version      Print build info
  george help         Show this help

init / auth / tools-* env (optional):
  PERSONA_DIR    default deploy/persona
  MCP_MANIFEST   default deploy/mcp.toml (auth/tools-* default: mcp.toml)
`)
}
