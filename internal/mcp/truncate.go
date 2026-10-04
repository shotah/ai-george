package mcp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// rangeHeader is the first line of an fs file_get result: "range: 1-199 of 199".
var rangeHeader = regexp.MustCompile(`^range: (\d+)-(\d+) of (\d+)\n`)

// Truncate limits s to maxChars runes, appending a marker when cut.
// maxChars <= 0 means no truncation.
func Truncate(s string, maxChars int) string {
	if maxChars <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	if out, ok := truncateRange(s, maxChars); ok {
		return out
	}
	const marker = "\n…[truncated]"
	keep := maxChars - utf8.RuneCountInString(marker)
	if keep < 1 {
		keep = 1
	}
	runes := []rune(s)
	if keep > len(runes) {
		keep = len(runes)
	}
	return string(runes[:keep]) + marker
}

// truncateRange cuts a file read at a whole line and makes its range header
// say what is shown. Left alone, the header still claimed the whole read, so
// the model asked for the line after the last one and missed the cut part.
func truncateRange(s string, maxChars int) (string, bool) {
	m := rangeHeader.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	first, _ := strconv.Atoi(m[1])
	total := m[3]
	body := s[len(m[0]):]
	// Header and marker are ASCII and at most about 160 bytes; reserve that.
	room := maxChars - 160
	if room < 1 {
		return "", false
	}
	runes := []rune(body)
	if room > len(runes) {
		room = len(runes)
	}
	cut := string(runes[:room])
	i := strings.LastIndexByte(cut, '\n')
	if i < 0 {
		return "", false
	}
	cut = cut[:i+1]
	last := first + strings.Count(cut, "\n") - 1
	return fmt.Sprintf("range: %d-%d of %s\n%s…[cut at %d chars: lines %d-%d of %s shown; read again with offset %d for the rest]",
		first, last, total, cut, maxChars, first, last, total, last+1), true
}
