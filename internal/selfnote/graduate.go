package selfnote

import "strings"

func quotedSpan(s string) string {
	i := strings.IndexByte(s, '"')
	if i < 0 {
		return ""
	}
	j := strings.IndexByte(s[i+1:], '"')
	if j < 0 {
		return ""
	}
	return s[i+1 : i+1+j]
}

// RestoreQuotedLines appends prior SELF.md bullets whose quoted spans are
// missing from next. Distill must not flatten a joke into a vibe word.
// Header/blockquote lines are ignored (Body). Empty prior is a no-op.
func RestoreQuotedLines(prior, next string) string {
	priorBody := Body(prior)
	if priorBody == "" {
		return next
	}
	nextLower := strings.ToLower(next)
	var missing []string
	for _, line := range strings.Split(priorBody, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		q := quotedSpan(line)
		if q == "" {
			continue
		}
		if strings.Contains(nextLower, strings.ToLower(q)) {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			line = "- " + strings.TrimLeft(line, "- ")
		}
		missing = append(missing, line)
	}
	if len(missing) == 0 {
		return next
	}
	return strings.TrimSpace(next) + "\n" + strings.Join(missing, "\n")
}
