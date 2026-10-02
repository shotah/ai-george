package persona

import (
	"regexp"
	"strings"

	"github.com/shotah/george/internal/selfnote"
)

func stampSELF(raw string) string {
	return selfnote.Stamp(raw)
}

// ownedSections are headings george wrote into PERSONA.md before the
// contract moved into the binary. An old file still carries them; they are
// dropped from the prompt, never from the file.
var ownedSections = []string{"## Self-notes", "## Location pins", "## Follow-up", "## Reactions"}

var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// userPersona is PERSONA.md as the prompt sees it: comments (the template's
// instructions) and george-owned sections removed.
func userPersona(raw string) string {
	raw = strings.TrimSpace(htmlComment.ReplaceAllString(raw, ""))
	for _, heading := range ownedSections {
		if start, end, ok := sectionSpan(raw, heading); ok {
			raw = strings.TrimSpace(raw[:start] + raw[end:])
		}
	}
	if strings.TrimSpace(strings.TrimPrefix(raw, "# PERSONA.md")) == "" {
		return ""
	}
	return raw
}

// sectionSpan is heading through the next ## heading or EOF.
func sectionSpan(raw, heading string) (start, end int, ok bool) {
	start = indexHeading(raw, heading)
	if start < 0 {
		return 0, 0, false
	}
	rest := raw[start+len(heading):]
	if rel := nextHeading(rest); rel >= 0 {
		return start, start + len(heading) + rel, true
	}
	return start, len(raw), true
}

func indexHeading(raw, heading string) int {
	if strings.HasPrefix(raw, heading) {
		return 0
	}
	i := strings.Index(raw, "\n"+heading)
	if i < 0 {
		return -1
	}
	return i + 1
}

func nextHeading(raw string) int {
	if i := strings.Index(raw, "\n## "); i >= 0 {
		return i + 1
	}
	return -1
}
