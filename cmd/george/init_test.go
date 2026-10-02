package main

import (
	"os"
	"path/filepath"
	"testing"
)

// configDir points init at a temp config dir so a test never writes the
// real ~/.config/george.
func configDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "george")
	t.Setenv("GEORGE_CONFIG_DIR", dir)
	return dir
}

func TestInitCmd_ScaffoldsAndSkips(t *testing.T) {
	dir := configDir(t)
	t.Setenv("PERSONA_DIR", "")
	t.Setenv("MCP_MANIFEST", "")

	if code := initCmd(); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	for _, name := range []string{"PERSONA.md", "SELF.md", "mcp.toml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	st, err := os.Stat(filepath.Join(dir, "env"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("env mode = %v, want 0600 (it holds keys)", st.Mode().Perm())
	}

	// Second run skips existing.
	if code := initCmd(); code != 0 {
		t.Fatalf("re-init exit %d", code)
	}
}

func TestInitCmd_UnwritablePersona(t *testing.T) {
	configDir(t)
	root := t.TempDir()
	fileAsDir := filepath.Join(root, "notadir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PERSONA_DIR", filepath.Join(fileAsDir, "persona"))
	t.Setenv("MCP_MANIFEST", filepath.Join(root, "mcp.toml"))
	if code := initCmd(); code == 0 {
		t.Fatal("expected failure for unwritable persona dir")
	}
}

func TestInitCmd_LeavesExistingPersona(t *testing.T) {
	configDir(t)
	root := t.TempDir()
	persona := filepath.Join(root, "persona")
	if err := os.MkdirAll(persona, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "# PERSONA.md\n\n## Self-notes\n\nmy own words\n"
	if err := os.WriteFile(filepath.Join(persona, "PERSONA.md"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "mcp.toml")
	t.Setenv("PERSONA_DIR", persona)
	t.Setenv("MCP_MANIFEST", manifest)

	if code := initCmd(); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	b, err := os.ReadFile(filepath.Join(persona, "PERSONA.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != mine {
		t.Fatalf("init rewrote PERSONA.md: %q", b)
	}
}
