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
