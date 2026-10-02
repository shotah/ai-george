package agent

import (
	"regexp"
	"strings"
)

// danglingToolTags is a chat-template tool-call tag left at the end of a
// reply with no call inside it (Qwen emits a bare <tool_call> after its
// answer). Only trailing tags: the same text mid-reply can be the answer.
var danglingToolTags = regexp.MustCompile(`(?:\s*</?(?:tool_call|function_call)>)+\s*$`)

func stripDanglingToolTags(s string) string {
	return strings.TrimSpace(danglingToolTags.ReplaceAllString(s, ""))
}

// harnessNotePrefix labels the per-turn block so Completer RoleUser stays
// speech. Trailing unlabeled RoleSystem was easy to skip; leading with
// [current time] primed calendar/tool fixation. After their words, tagged.
const harnessNotePrefix = "[harness] Not user text — "

// harnessTags is the stamp order inside the block and the word the header
// uses for each. The header names only what is actually present.
var harnessTags = []struct{ tag, word string }{
	{"[current time]", "clock"},
}

func formatHarnessClock(clock string) string {
	clock = strings.TrimSpace(clock)
	if clock == "" {
		return ""
	}
	return harnessNote(clock) + "\n" + clock
}

// harnessNote is the header built from the tags in this turn's block, so a
// memory-off turn is not told about hours it does not have.
func harnessNote(clock string) string {
	var words []string
	for _, t := range harnessTags {
		if !strings.Contains(clock, t.tag) {
			continue
		}
		if n := len(words); n > 0 && words[n-1] == t.word {
			continue
		}
		words = append(words, t.word)
	}
	return harnessNotePrefix + joinAnd(words) + " for this turn."
}

func joinAnd(words []string) string {
	switch len(words) {
	case 0:
		return "context"
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
	}
}

// stripHarnessContext drops pasted / old-client clock and hydration blocks
// from inbound text so they are not stored or used as the hydrate query.
func stripHarnessContext(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "\n\n")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if harnessBlock(p) {
			continue
		}
		trimmed := strings.TrimRight(stripTrailingHarnessLines(p), "\n")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func harnessBlock(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return harnessTagLine(line)
	}
	return false
}

func stripTrailingHarnessLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if !harnessTagLine(strings.TrimSpace(line)) {
			continue
		}
		if i == 0 {
			return s
		}
		return strings.Join(lines[:i], "\n")
	}
	return s
}

func harnessTagLine(line string) bool {
	if strings.HasPrefix(line, "[harness]") || strings.HasPrefix(line, "[memory]") {
		return true
	}
	for _, t := range harnessTags {
		if strings.HasPrefix(line, t.tag) {
			return true
		}
	}
	return false
}
