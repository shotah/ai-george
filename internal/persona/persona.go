// Package persona builds the system prompt: george's contract, then the
// human's PERSONA.md, then the agent-written SELF.md.
package persona

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shotah/george/internal/selfnote"
)

const (
	// FilePersona is the human-owned personality file: name, voice, tone.
	// It is not evaluated; george never writes it.
	FilePersona = "PERSONA.md"
	// FileSelf is the agent-written personality file. Same basename as selfnote.FileName.
	FileSelf = "SELF.md"
)

// PreferredOrder is the concat order for the two persona files, after the
// contract. Missing files are skipped. Other *.md files are ignored.
var PreferredOrder = []string{
	FilePersona,
	FileSelf,
}

//go:embed contract.md
var contract string

// Contract is george's harness-owned prompt layer: how the work is done.
// The live eval grades exactly this text; PERSONA.md cannot replace it.
func Contract() string {
	return strings.TrimSpace(contract) + "\n\n" + selfnote.RulesSection
}

// Load returns the contract, then PERSONA.md, then SELF.md. A missing
// directory or missing files still yield the contract.
func Load(dir string) (string, error) {
	parts := []string{Contract()}
	for _, name := range PreferredOrder {
		text, err := readOptional(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		if text == "" {
			continue
		}
		text = stampPreferred(name, text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func stampPreferred(name, text string) string {
	switch name {
	case FileSelf:
		return stampSELF(text)
	case FilePersona:
		return userPersona(text)
	default:
		return text
	}
}

// tzField matches PERSONA.md lines like `- **Timezone:** America/Los_Angeles`
// (colon may sit inside the bold markers).
var tzField = regexp.MustCompile(`(?i)\**timezone\**\s*:\s*\**\s*([A-Za-z0-9_+\-/]+)`)

// Timezone extracts an IANA zone from PERSONA.md-style persona text.
// Empty if the field is missing or not a loadable location.
func Timezone(text string) string {
	m := tzField.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	name := strings.TrimSpace(m[1])
	if name == "" {
		return ""
	}
	if _, err := time.LoadLocation(name); err != nil {
		return ""
	}
	return name
}

// ResolveTimezone prefers PERSONA.md Timezone over fallback (the machine zone).
func ResolveTimezone(personaText, fallback string) (name string, loc *time.Location, source string) {
	if tz := Timezone(personaText); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err == nil {
			return tz, loc, FilePersona
		}
	}
	fb := strings.TrimSpace(fallback)
	if fb == "" {
		fb = "America/Los_Angeles"
	}
	loc, err := time.LoadLocation(fb)
	if err != nil {
		loc, err = time.LoadLocation("America/Los_Angeles")
		if err != nil {
			return "UTC", time.UTC, "UTC"
		}
		return "America/Los_Angeles", loc, "America/Los_Angeles"
	}
	return fb, loc, "fallback"
}

func readOptional(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("persona: read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}
