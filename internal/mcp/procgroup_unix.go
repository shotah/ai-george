//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup keeps a terminal Ctrl-C, which cancels george's turn, from
// also killing the MCP children.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
