package agent_test

import (
	"strings"
	"testing"

	"github.com/shotah/george/internal/mcpenable"
	"github.com/shotah/george/internal/memory"
	"github.com/shotah/george/internal/provider"
	"github.com/shotah/george/internal/selfnote"
	"github.com/shotah/george/internal/websearch"
)

// Workspace read, patch, git, and shell are MCP binaries. The process
// publishes memory, self-notes, web search, and mcp_enable — nothing that reads a tree or runs a command.
func TestProcessHasNoWorkspaceTools(t *testing.T) {
	defs := append([]provider.ToolDef{}, memory.ToolDefs()...)
	defs = append(defs, selfnote.ToolDefs()...)
	defs = append(defs, websearch.ToolDefs()...)
	defs = append(defs, mcpenable.ToolDef())

	if len(defs) == 0 {
		t.Fatal("no builtin tools")
	}
	for _, d := range defs {
		if strings.Contains(d.Name, "__") {
			t.Errorf("builtin %s is an MCP name", d.Name)
		}
		switch {
		case strings.HasPrefix(d.Name, "file_"),
			strings.HasPrefix(d.Name, "status_"),
			strings.HasPrefix(d.Name, "diff_"),
			strings.HasPrefix(d.Name, "commits_"),
			strings.HasPrefix(d.Name, "stage_"),
			strings.HasPrefix(d.Name, "commit_"),
			d.Name == "command_run",
			d.Name == "grep",
			d.Name == "git_status",
			d.Name == "run_command",
			d.Name == "file_update":
			t.Errorf("workspace tool %s is builtin", d.Name)
		}
	}
}
