//go:build windows

package mcp

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup keeps a console Ctrl-C, which cancels george's turn, from
// also killing the MCP children.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
