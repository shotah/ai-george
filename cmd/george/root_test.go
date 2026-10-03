package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoRoot_FindsGitAbove(t *testing.T) {
	top := t.TempDir()
	if err := os.Mkdir(filepath.Join(top, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(top, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := repoRoot(sub); got != top {
		t.Fatalf("repoRoot = %q, want %q", got, top)
	}
}

func TestRepoRoot_WorktreeGitFile(t *testing.T) {
	top := t.TempDir()
	if err := os.WriteFile(filepath.Join(top, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := repoRoot(top); got != top {
		t.Fatalf("repoRoot = %q, want %q", got, top)
	}
}

func TestRepoRoot_NoRepoIsDir(t *testing.T) {
	dir := t.TempDir()
	if got := repoRoot(dir); got != dir {
		t.Fatalf("repoRoot = %q, want %q", got, dir)
	}
}

func TestNormalizeRemote_SpellingsAgree(t *testing.T) {
	for _, raw := range []string{
		"git@github.com:shotah/george.git",
		"https://github.com/shotah/george.git",
		"https://user:tok@GitHub.com/shotah/george/",
		"ssh://git@github.com:22/shotah/george.git",
		"github.com:shotah/george",
	} {
		if got := normalizeRemote(raw); got != "github.com/shotah/george" {
			t.Errorf("normalizeRemote(%q) = %q", raw, got)
		}
	}
	if got := normalizeRemote(""); got != "" {
		t.Errorf("normalizeRemote(\"\") = %q", got)
	}
}

func writeGitConfig(t *testing.T, gitDir, url string) {
	t.Helper()
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + url + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRepoID_OriginWins(t *testing.T) {
	top := t.TempDir()
	writeGitConfig(t, filepath.Join(top, ".git"), "git@github.com:shotah/george.git")
	if got := repoID(top); got != "github.com/shotah/george" {
		t.Fatalf("repoID = %q", got)
	}
}

func TestRepoID_WorktreeSharesOrigin(t *testing.T) {
	main := t.TempDir()
	writeGitConfig(t, filepath.Join(main, ".git"), "https://github.com/shotah/george")
	wtGit := filepath.Join(main, ".git", "worktrees", "wt")
	if err := os.MkdirAll(wtGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtGit+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := repoID(wt); got != "github.com/shotah/george" {
		t.Fatalf("repoID(worktree) = %q", got)
	}
}

func TestRepoID_NoOriginIsRoot(t *testing.T) {
	top := t.TempDir()
	if err := os.MkdirAll(filepath.Join(top, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := repoID(top); got != top {
		t.Fatalf("repoID = %q, want %q", got, top)
	}
	plain := t.TempDir()
	if got := repoID(plain); got != plain {
		t.Fatalf("repoID(no repo) = %q, want %q", got, plain)
	}
}

func TestPrependPath_FirstWins(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	if err := prependPath("/opt/george/bin"); err != nil {
		t.Fatal(err)
	}
	want := "/opt/george/bin" + string(os.PathListSeparator) + "/usr/bin"
	if got := os.Getenv("PATH"); got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
}

func TestSetGeorgeRoot_KeepsOperatorValue(t *testing.T) {
	t.Setenv("GEORGE_ROOT", "/operator/pick")
	got, err := setGeorgeRoot()
	if err != nil || got != "/operator/pick" {
		t.Fatalf("setGeorgeRoot = %q, %v", got, err)
	}
}

func TestSetGeorgeRoot_DefaultsToCwdRepo(t *testing.T) {
	t.Setenv("GEORGE_ROOT", "")
	dir := t.TempDir()
	t.Chdir(dir)
	got, err := setGeorgeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := repoRoot(dir); got != want || os.Getenv("GEORGE_ROOT") != want {
		t.Fatalf("setGeorgeRoot = %q, env %q, want %q", got, os.Getenv("GEORGE_ROOT"), want)
	}
}
