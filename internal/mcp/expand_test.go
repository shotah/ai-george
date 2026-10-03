package mcp_test

import (
	"testing"

	"github.com/shotah/george/internal/mcp"
)

func TestExpandArgs(t *testing.T) {
	t.Setenv("GEORGE_ROOT", "/src/repo")
	got := mcp.ExpandArgs([]string{"--root", "${GEORGE_ROOT}", "$GEORGE_ROOT/sub"})
	if got[1] != "/src/repo" || got[2] != "/src/repo/sub" {
		t.Fatalf("%v", got)
	}
}
