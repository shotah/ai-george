package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
	"sync"
)

// The two contract rules a live run kept breaking, held in code: a patch
// waits on a read of that file this turn, and commit or stage waits on the
// human asking for it this turn. A refusal is the tool result, so the model
// reads why and goes on. The same record holds the reply to what the turn
// did: a check it reports needs a command that ran.

var errLaneRefused = errors.New("refused")

// askedForGit is the inbound asking to commit or stage. "Don't commit"
// matches too; the contract line still covers that one.
var askedForGit = regexp.MustCompile(`(?i)\b(commit|stag(e|ed|ing)|git add)`)

type laneKind int

const (
	laneNone laneKind = iota
	laneRead
	lanePatch
	laneGit
	laneRun
)

// laneOf reads the tool by its suffix, so a mangled name the host will
// repair (fs.file_patch, functions.fs__file_patch) is held to the same rule.
func laneOf(name string) laneKind {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, "file_get"), strings.HasSuffix(n, "file_create"):
		return laneRead
	case strings.HasSuffix(n, "file_patch"):
		return lanePatch
	case strings.HasSuffix(n, "commit_create"), strings.HasSuffix(n, "stage_update"):
		return laneGit
	case strings.HasSuffix(n, "command_run"):
		return laneRun
	}
	return laneNone
}

type turnLanes struct {
	gitAsked bool
	mu       sync.Mutex
	read     map[string]bool
	patched  []string
	ran      bool
}

type turnLanesKey struct{}

func withTurnLanes(ctx context.Context, inbound string) context.Context {
	return context.WithValue(ctx, turnLanesKey{}, &turnLanes{
		gitAsked: askedForGit.MatchString(inbound),
		read:     map[string]bool{},
	})
}

func lanesFrom(ctx context.Context) *turnLanes {
	l, _ := ctx.Value(turnLanesKey{}).(*turnLanes)
	return l
}

// check returns one refusal per call (nil = allowed), against what was read
// before this round, so a read and a patch in one batch refuse the patch
// whichever finishes first.
func (l *turnLanes) check(calls []callView) []error {
	out := make([]error, len(calls))
	if l == nil {
		return out
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, c := range calls {
		switch laneOf(c.name) {
		case lanePatch:
			p := argPath(c.args)
			if p != "" && !l.wasRead(p) {
				out[i] = laneError("read " + p + " with fs__file_get first. A patch waits on a read of that file this turn, so it is made against what is on disk")
			}
		case laneGit:
			if !l.gitAsked {
				out[i] = laneError("they didn't ask for a commit or stage this turn. Git is their lane: leave the change in the working tree, say what changed, and stop")
			}
		}
	}
	return out
}

// wasRead is an exact match or one spelling ending in the other at a
// separator (a.go against an absolute path under the root). Caller holds mu.
func (l *turnLanes) wasRead(p string) bool {
	if l.read[p] {
		return true
	}
	for r := range l.read {
		if strings.HasSuffix(p, "/"+r) || strings.HasSuffix(r, "/"+p) {
			return true
		}
	}
	return false
}

// record notes the files a successful call has shown this turn. A patch
// counts too: the model saw its own change in the patch result.
func (l *turnLanes) record(name string, args json.RawMessage) {
	if l == nil {
		return
	}
	k := laneOf(name)
	l.mu.Lock()
	defer l.mu.Unlock()
	if k == laneRun {
		l.ran = true
		return
	}
	if k != laneRead && k != lanePatch {
		return
	}
	if p := argPath(args); p != "" {
		l.read[p] = true
		if k == lanePatch || strings.HasSuffix(strings.ToLower(name), "file_create") {
			l.patched = append(l.patched, p)
		}
	}
}

// A reply that reports a check result, a change, or no change. The turn's
// own calls say whether each is true.
var (
	claimsCheckPassed = regexp.MustCompile(`(?i)\b(tests?|lint|linter|build|vet|checks?)\b[^\n]{0,24}?\b(pass(es|ed)?|passing|succeed(s|ed)?|green|clean)\b|\bexit (code |status )?0\b`)
	saysNotRun        = regexp.MustCompile(`(?i)(\bnot|n't|\bnever) (been )?(run|ran|verified|checked)\b|\bunverified\b|\buntested\b`)
	claimsNoChange    = regexp.MustCompile(`(?i)\bno (code|files?|changes?) (was |were |is )?(changed|made|modified|needed|necessary)\b|\bnothing (was )?(changed|modified)\b`)
	claimsEdit        = regexp.MustCompile(`(?i)\b(fixed|patched|renamed)\b|\bnow (returns|reads|says|uses|calls)\b`)
	saysNotEdited     = regexp.MustCompile(`(?i)(\bnot|n't|\bnever) (been )?(fixed|patched|renamed|changed)\b`)
)

// contradiction is the nudge for a reply the turn's calls disprove: a check
// that passed with no command run, a change with nothing written, or no
// change after a write landed. Empty when the reply holds.
func (l *turnLanes) contradiction(reply string) string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.ran && claimsCheckPassed.MatchString(reply) && !saysNotRun.MatchString(reply) {
		if len(l.patched) == 0 {
			return "[system] Your reply reports a check result, but nothing produced it. No command ran and no file was patched this turn. " +
				"Do the work now with the real tool calls (the patch, then the check), or reply again saying only what a tool returned this turn."
		}
		return "[system] Your reply reports a check result, but no command ran this turn. " +
			"Run the check now with shell__command_run, or reply again saying what changed and that it was not run."
	}
	if len(l.patched) == 0 && !l.ran && claimsEdit.MatchString(reply) && !saysNotEdited.MatchString(reply) {
		return "[system] Your reply says a file changed, but no file was written this turn. " +
			"Make the change now with the real tool calls, or reply again saying only what a tool returned this turn."
	}
	if len(l.read) == 0 && !l.ran && claimsNoChange.MatchString(reply) {
		return "[system] Your reply says no change was needed, but no file was read and no command ran this turn. " +
			"Read the file with fs__file_get, then make the change or say what it holds."
	}
	if len(l.patched) > 0 && claimsNoChange.MatchString(reply) {
		return "[system] Your reply says nothing changed, but a write landed this turn on " +
			strings.Join(l.patched, ", ") + ". Reply again naming what changed."
	}
	return ""
}

type callView struct {
	name string
	args json.RawMessage
}

func laneError(why string) error {
	return &laneRefusal{why: why}
}

type laneRefusal struct{ why string }

func (e *laneRefusal) Error() string { return "refused: " + e.why }

func (e *laneRefusal) Unwrap() error { return errLaneRefused }

// argPath is the call's "path", cleaned so ./a.go, /a.go, and a.go agree.
func argPath(args json.RawMessage) string {
	var v struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(args, &v) != nil || strings.TrimSpace(v.Path) == "" {
		return ""
	}
	p := path.Clean("/" + strings.TrimSpace(v.Path))
	return strings.TrimPrefix(p, "/")
}
