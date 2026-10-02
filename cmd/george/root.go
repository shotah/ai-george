package main

import (
	"os"
	"path/filepath"
)

// repoRoot is the git toplevel at or above dir (a .git file counts, for
// worktrees), or dir itself when no repo encloses it.
func repoRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}

// prependPath puts dir first on PATH, so tools-fetch's binaries win for the
// MCP children without the user editing their shell.
func prependPath(dir string) error {
	return os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// setGeorgeRoot exports GEORGE_ROOT for ${GEORGE_ROOT} in mcp.toml args,
// unless the operator already set it.
func setGeorgeRoot() (string, error) {
	if root := os.Getenv("GEORGE_ROOT"); root != "" {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root := repoRoot(cwd)
	return root, os.Setenv("GEORGE_ROOT", root)
}
