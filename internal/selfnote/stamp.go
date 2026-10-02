package selfnote

import "strings"

// RulesSection is the kernel-owned Self-notes block injected into PERSONA.md.
// Operators do not edit this; george overwrites the section on load/sync.
const RulesSection = "## Self-notes (`self_note` → SELF.md)\n\n" +
	"- **Append-only.** One short `-` line. Does **not** rewrite `SELF.md`.\n" +
	"- How this human likes the work done, in every repo: review style, commit shape, what to never do, the tone of a reply.\n" +
	"- Skip if it is already in the `SELF.md` bullets in this prompt. Not repo facts (commands, conventions): those are memory_store.\n" +
	"- Note it the turn they say it or correct you. Use their words."

// Body returns SELF.md without the kernel header (title + leading blockquotes).
// Operator/agent bullets are kept. An old header is dropped so Stamp can
// replace it.
func Body(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	i := 0
	if looksLikeSelfTitle(lines[0]) {
		i++
	}
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, ">") {
			i++
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines[i:], "\n"))
}

func looksLikeSelfTitle(line string) bool {
	t := strings.TrimSpace(strings.TrimLeft(line, "#"))
	t = strings.TrimSpace(t)
	return strings.Contains(strings.ToLower(t), "self.md")
}

// Stamp returns Header plus the file body. Empty body → Header only.
func Stamp(raw string) string {
	body := Body(raw)
	if body == "" {
		return Header
	}
	return Header + "\n\n" + body
}
