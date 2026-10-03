package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
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

// repoID names the repo for its session and its memory rows: the normalized
// origin URL, which survives a re-clone and is the same in every worktree,
// else root itself.
func repoID(root string) string {
	if u := normalizeRemote(originURL(root)); u != "" {
		return u
	}
	return root
}

// originURL reads remote.origin.url from root's git config, following a
// .git file to a worktree's or submodule's git dir. Empty when there is none.
func originURL(root string) string {
	gitDir := filepath.Join(root, ".git")
	if b, err := os.ReadFile(gitDir); err == nil {
		d, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
		if !ok {
			return ""
		}
		gitDir = strings.TrimSpace(d)
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(root, gitDir)
		}
		if c, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
			common := strings.TrimSpace(string(c))
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitDir, common)
			}
			gitDir = common
		}
	}
	cfg, err := ini.Load(filepath.Join(gitDir, "config"))
	if err != nil {
		return ""
	}
	return cfg.Section(`remote "origin"`).Key("url").String()
}

// normalizeRemote makes the ssh, https, and scp-style spellings of one remote
// the same string: host/path, no user, port, scheme, or trailing .git.
func normalizeRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	host, path := "", raw
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		host, path = u.Hostname(), u.Path
	} else if at, rest, ok := strings.Cut(raw, ":"); ok && !strings.Contains(at, "/") && !filepath.IsAbs(raw) {
		host, path = at, rest
		if _, h, ok := strings.Cut(host, "@"); ok {
			host = h
		}
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" {
		return path
	}
	return strings.ToLower(host) + "/" + path
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
