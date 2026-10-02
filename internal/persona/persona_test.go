package persona_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/george/examples"
	"github.com/shotah/george/internal/persona"
)

func TestLoad_ContractThenPersonaThenSelfIgnoresExtras(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ZZZ.md", "extra-z")
	write("PERSONA.md", "persona-body")
	write("SELF.md", "self-body")
	write("SOUL.md", "soul-leftover")
	write("notes.txt", "ignored")

	got, err := persona.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasPrefix(got, persona.Contract()) {
		t.Fatalf("contract must lead the prompt: %q", got)
	}
	personaIdx := strings.Index(got, "persona-body")
	selfIdx := strings.Index(got, "self-body")
	if personaIdx < len(persona.Contract()) || selfIdx < personaIdx {
		t.Fatalf("order wrong in %q", got)
	}
	for _, bad := range []string{"extra-z", "soul-leftover", "ignored"} {
		if strings.Contains(got, bad) {
			t.Fatalf("unexpected %q in %q", bad, got)
		}
	}
}

func TestLoad_MissingDirIsContract(t *testing.T) {
	got, err := persona.Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != persona.Contract() {
		t.Fatalf("got %q, want the contract alone", got)
	}
	if !strings.Contains(got, "fs__file_get") || !strings.Contains(got, "How this human likes the work done") {
		t.Fatalf("contract missing the edit chain or self-note rules: %q", got)
	}
}

func TestLoad_MissingPreferredTolerant(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PERSONA.md"), []byte("only-persona"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := persona.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(got, "only-persona") {
		t.Fatalf("got %q, want to contain only-persona", got)
	}
}

// An old PERSONA.md still carries sections george used to stamp. They leave
// the prompt; the file is never written.
func TestLoad_DropsOwnedSectionsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SELF.md"), []byte("# SELF.md — Who You Are Becoming\n\n> stale\n\n- dry humor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "# PERSONA.md\n\n## Voice\n\nkeep me\n\n## Self-notes (`self_note` → SELF.md)\n\n- stale rule\n\n## Location pins\n\n- gps\n\n## Follow-up\n\n- [wait]\n\n## Reactions\n\n- [react]\n"
	path := filepath.Join(dir, "PERSONA.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := persona.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	mine := strings.TrimPrefix(got, persona.Contract())
	for _, gone := range []string{"stale", "gps", "[wait]", "[react]", "## Location pins", "## Self-notes"} {
		if strings.Contains(mine, gone) {
			t.Fatalf("owned %q kept: %q", gone, mine)
		}
	}
	if !strings.Contains(mine, "## Voice\n\nkeep me") || !strings.Contains(mine, "- dry humor") {
		t.Fatalf("lost the human's text: %q", mine)
	}
	if strings.Count(got, "## Self-notes") != 1 {
		t.Fatalf("self-note rules should appear once, from the contract: %q", got)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != body {
		t.Fatalf("PERSONA.md was written: %q %v", b, err)
	}
}

// The template `george init` ships is all comment: a fresh install runs the
// contract alone, and the example Timezone line is not the human's zone.
func TestLoad_ShippedTemplateIsEmpty(t *testing.T) {
	dir := t.TempDir()
	tmpl, err := examples.FS.ReadFile("persona/PERSONA.example.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PERSONA.md"), tmpl, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := persona.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != persona.Contract() {
		t.Fatalf("template leaked into the prompt: %q", strings.TrimPrefix(got, persona.Contract()))
	}
	if _, _, source := persona.ResolveTimezone(got, "UTC"); source == persona.FilePersona {
		t.Fatal("commented example Timezone was read as the human's")
	}
}

func TestTimezone_FromPersonaMarkdown(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"- **Timezone:** America/Los_Angeles\n- **Location:** Seattle",
		"Timezone: America/Los_Angeles",
		"**Timezone:** America/Los_Angeles",
	} {
		if got := persona.Timezone(in); got != "America/Los_Angeles" {
			t.Fatalf("input %q: got %q", in, got)
		}
	}
	if persona.Timezone("no tz here") != "" {
		t.Fatal("expected empty")
	}
	if persona.Timezone("- **Timezone:** Not/AZone") != "" {
		t.Fatal("invalid IANA must be empty")
	}
}

func TestResolveTimezone_PrefersPersonaMarkdown(t *testing.T) {
	t.Parallel()
	name, loc, source := persona.ResolveTimezone("- **Timezone:** America/Los_Angeles", "UTC")
	if name != "America/Los_Angeles" || source != "PERSONA.md" || loc == nil {
		t.Fatalf("name=%q source=%q loc=%v", name, source, loc)
	}
	name, _, source = persona.ResolveTimezone("", "America/New_York")
	if name != "America/New_York" || source != "fallback" {
		t.Fatalf("fallback name=%q source=%q", name, source)
	}
	name, loc, source = persona.ResolveTimezone("", "")
	if name != "America/Los_Angeles" || loc == nil {
		t.Fatalf("empty fallback name=%q source=%q", name, source)
	}
}
