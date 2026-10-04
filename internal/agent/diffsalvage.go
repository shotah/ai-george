package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/shotah/george/internal/provider"
)

// A coding model's strongest prior is to answer an edit with a patch as
// text. Left alone, that text is the reply: a diff of a file it may never
// have read, rendered so well it reads as done. The same salvage that turns
// a printed JSON tool call into the call turns a printed diff into
// fs__file_patch calls, one per file, with the diff as the argument. From
// there the host's own rules take over: a file not read this turn is
// refused with "read it first", a hunk that does not match is the server's
// mismatch error, and a hunk that matches is the edit they asked for.

// printedDiff is one file's worth of unified diff found in a reply: the
// path and its hunks, header lines dropped. Blocks for the same path are
// merged so one file is one call.
type printedDiff struct {
	path  string
	hunks string
}

// diff is the one-file unified diff fs-mcp takes, with a header it can
// parse whatever the model wrote above the hunks.
func (d printedDiff) diff() string {
	return "--- a/" + d.path + "\n+++ b/" + d.path + "\n" + d.hunks
}

var (
	diffGitLine  = regexp.MustCompile(`^diff --git a/(\S+) b/(\S+)`)
	diffOldLine  = regexp.MustCompile(`^--- (?:a/)?(\S+)`)
	diffNewLine  = regexp.MustCompile(`^\+\+\+ (?:b/)?(\S+)`)
	diffHunkLine = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@`)
)

// printedDiffs splits content into the per-file unified diffs it holds. A
// file starts at `diff --git a/p b/p` or at `--- p` followed by `+++ p`,
// and runs through its hunks to the next file header, a code fence, or
// the first line that is not part of a hunk. A file with no hunk, or a
// hunk with no file header, is not returned: there is nothing to place it
// against. Two blocks for one path come back as one entry, hunks in the
// order written.
func printedDiffs(content string) []printedDiff {
	var (
		out    []printedDiff
		cur    *printedDiff
		lines  []string
		inHunk bool
		hunks  int
	)
	flush := func() {
		if cur != nil && hunks > 0 && cur.path != "" {
			// Blank lines inside a hunk stand in for blank context lines
			// the model forgot to prefix; trailing ones are the gap before
			// the prose, not context.
			for len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			for i, l := range lines {
				if l == "" {
					lines[i] = " "
				}
			}
			body := strings.Join(lines, "\n") + "\n"
			merged := false
			for i := range out {
				if out[i].path == cur.path {
					out[i].hunks += body
					merged = true
					break
				}
			}
			if !merged {
				out = append(out, printedDiff{path: cur.path, hunks: body})
			}
		}
		cur, lines, inHunk, hunks = nil, nil, false, 0
	}
	start := func(path string) {
		flush()
		cur = &printedDiff{path: path}
	}
	all := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	// fileHeader is a `--- p` line that opens a file: the next line is its
	// `+++ p`. Inside a hunk that is how it differs from a removed line
	// that happens to start with "--".
	fileHeader := func(i int) bool {
		return diffOldLine.MatchString(all[i]) && i+1 < len(all) && diffNewLine.MatchString(all[i+1])
	}
	for i, line := range all {
		switch {
		case strings.HasPrefix(strings.TrimSpace(line), "```"):
			flush()
			continue
		case diffGitLine.MatchString(line):
			start(diffGitLine.FindStringSubmatch(line)[2])
		case fileHeader(i):
			// The second header line of a diff --git block stays with it;
			// anything else opens a new file.
			if cur == nil || hunks > 0 || inHunk {
				start(diffOldLine.FindStringSubmatch(line)[1])
			}
		case diffNewLine.MatchString(line) && !inHunk:
			if cur == nil {
				continue
			}
			if p := diffNewLine.FindStringSubmatch(line)[1]; p != "/dev/null" {
				cur.path = p
			}
		case diffHunkLine.MatchString(line):
			if cur == nil {
				continue
			}
			inHunk = true
			hunks++
		case inHunk && (line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, `\`)):
			// A hunk body line.
		case cur != nil && !inHunk && (strings.HasPrefix(line, "index ") || strings.HasSuffix(line, " mode 100644") || strings.HasSuffix(line, " mode 100755")):
			// Git's extended header lines ride along.
		default:
			flush()
			continue
		}
		// Hunks only; the header is rebuilt from the path.
		if cur != nil && inHunk {
			lines = append(lines, line)
		}
	}
	flush()
	return out
}

// salvagePrintedDiff is the fs__file_patch calls for a reply that is a
// diff and nothing else happened: no write landed this turn, the reply
// does not call itself a proposal, and a patch tool is published. One
// call per file, in the order written. Nil when there is nothing to
// salvage or no way to apply it.
func salvagePrintedDiff(content string, defs []provider.ToolDef, l *turnLanes) []provider.ToolCall {
	if !showsDiff.MatchString(content) || saysProposal.MatchString(content) || l == nil || l.wrote() {
		return nil
	}
	patch := ""
	for _, d := range defs {
		if strings.HasSuffix(d.Name, "__file_patch") {
			patch = d.Name
			break
		}
	}
	if patch == "" {
		return nil
	}
	var calls []provider.ToolCall
	for i, d := range printedDiffs(content) {
		args, err := json.Marshal(map[string]string{"path": d.path, "diff": d.diff()})
		if err != nil {
			continue
		}
		calls = append(calls, provider.ToolCall{
			ID:        fmt.Sprintf("printed-diff-%d", i+1),
			Name:      patch,
			Arguments: string(args),
		})
	}
	return calls
}
