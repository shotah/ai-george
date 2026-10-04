package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/shotah/george/internal/provider"
)

// canRunCommand is a command tool among this turn's defs.
func canRunCommand(defs []provider.ToolDef) bool {
	return slices.ContainsFunc(defs, func(d provider.ToolDef) bool { return laneOf(d.Name) == laneRun })
}

// The two contract rules a live run kept breaking, held in code: a patch
// waits on a read of that file this turn, and commit or stage waits on the
// human asking for it this turn. A refusal is the tool result, so the model
// reads why and goes on. The same record holds the reply to what the turn
// did: a check it reports needs a command that ran.

var errLaneRefused = errors.New("refused")

// askedForGit is the inbound asking to commit or stage. "Don't commit"
// matches too; the contract line still covers that one.
var askedForGit = regexp.MustCompile(`(?i)\b(commit|stag(e|ed|ing)|git add)`)

// askedForEdit is the inbound asking for a change to the tree, as opposed
// to a question or a review. A verb here plus a turn that only read is a
// turn that stopped at the diagnosis.
var askedForEdit = regexp.MustCompile(`(?i)\b(fix|change|add|make it|make the|rename|update|insert|append|remove|delete|replace|rewrite|implement|refactor|bump)\b`)

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
	gitAsked  bool
	editAsked bool
	mu        sync.Mutex
	read      map[string]bool
	patched   []string
	ran       bool

	// The finish line: code written, its test, and a command after it.
	codeWritten  bool
	ranSinceCode bool
	testWritten  bool
	addedFuncs   []string
	testNudges   int
	checkNudged  bool

	// Writes that were tried and did not land, and whether the reply was
	// already told so.
	failedWrites  []string
	unlandedNudge bool

	// nextPage is, per path, where the newest paged read of it stopped: the
	// last line shown and the offset the server named for the rest. 0 next
	// means the read reached the end.
	nextPage map[string]pageMark
}

type pageMark struct{ last, next int }

type turnLanesKey struct{}

func withTurnLanes(ctx context.Context, inbound string) context.Context {
	return context.WithValue(ctx, turnLanesKey{}, &turnLanes{
		gitAsked:  askedForGit.MatchString(inbound),
		editAsked: askedForEdit.MatchString(inbound),
		read:      map[string]bool{},
		nextPage:  map[string]pageMark{},
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
		l.ranSinceCode = true
		return
	}
	if k != laneRead && k != lanePatch {
		return
	}
	if p := argPath(args); p != "" {
		l.read[p] = true
		if k == lanePatch || strings.HasSuffix(strings.ToLower(name), "file_create") {
			l.patched = append(l.patched, p)
			l.noteWrite(p, args)
		}
	}
}

// A paged read says where it stopped. fs-mcp 0.0.6: "range: 1-39 of 230;
// next offset 40" (or "; end"); the host's own cut: "read again with offset
// 40". A read past the end: "offset 400 is past end (last line 300)".
var (
	pageHeader   = regexp.MustCompile(`^range: \d+-(\d+) of \d+(?:; next offset (\d+)|; end)?`)
	hostCutNext  = regexp.MustCompile(`read again with offset (\d+)`)
	offsetTooFar = regexp.MustCompile(`\boffset \d+ is past (the )?end\b`)
)

// noteResult reads what a successful call returned, for the parts of it the
// host acts on later: where a paged read stopped.
func (l *turnLanes) noteResult(name string, args json.RawMessage, text string) {
	if l == nil || laneOf(name) != laneRead || !strings.HasSuffix(strings.ToLower(name), "file_get") {
		return
	}
	p := argPath(args)
	if p == "" {
		return
	}
	m := pageHeader.FindStringSubmatch(text)
	if m == nil {
		return
	}
	last, _ := strconv.Atoi(m[1])
	next, _ := strconv.Atoi(m[2])
	if next == 0 {
		if c := hostCutNext.FindStringSubmatch(text); c != nil {
			next, _ = strconv.Atoi(c[1])
		}
	}
	l.mu.Lock()
	l.nextPage[p] = pageMark{last: last, next: next}
	l.mu.Unlock()
}

// continuation is where the newest paged read of path stopped, when it did
// not reach the end.
func (l *turnLanes) continuation(args json.RawMessage) (pageMark, bool) {
	if l == nil {
		return pageMark{}, false
	}
	p := argPath(args)
	l.mu.Lock()
	defer l.mu.Unlock()
	m, ok := l.nextPage[p]
	return m, ok && m.next > 0
}

// noteFailure keeps a write that did not land, so a reply that goes quiet
// about it can be sent back.
func (l *turnLanes) noteFailure(name string, args json.RawMessage, err error) {
	if l == nil || err == nil {
		return
	}
	k := laneOf(name)
	if k != lanePatch && !strings.HasSuffix(strings.ToLower(name), "file_create") {
		return
	}
	why := err.Error()
	if i := strings.IndexByte(why, '\n'); i >= 0 {
		why = why[:i]
	}
	if len(why) > 160 {
		why = why[:160] + "…"
	}
	l.mu.Lock()
	l.failedWrites = append(l.failedWrites, name+" "+argPath(args)+": "+why)
	l.mu.Unlock()
}

// saysWriteFailed is a reply that owns up to an edit that did not land.
var saysWriteFailed = regexp.MustCompile(`(?i)\b(could ?n[o']t|unable to|failed|did ?n[o']t (land|apply|match|work)|was ?n[o']t (able|written|applied)|no match(es)?|nothing (was )?written)\b`)

// unlanded is the nudge for a reply that ships with the asked edit not
// made and nothing said about it. Two shapes, once a turn:
//
//   - every write this turn failed, and the reply announces the edit or
//     moves on instead of owning the miss;
//   - the inbound asked for a change, the turn read files and wrote
//     nothing, and the reply is a diagnosis with no question in it ("Two
//     bugs in calc.go: …" and stop).
//
// A question or a review (no edit verb in the inbound), a reply that asks
// something, or one that says no change is needed or the edit failed, is
// never nudged here.
func (l *turnLanes) unlanded(reply string) string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unlandedNudge || len(l.patched) > 0 || saysWriteFailed.MatchString(reply) {
		return ""
	}
	if len(l.failedWrites) > 0 {
		l.unlandedNudge = true
		return "[system] No write landed this turn. " + strings.Join(l.failedWrites, "; ") + ". " +
			"Make the edit now: read the lines you are replacing with fs__file_get and send old exactly as shown " +
			"(or after_line with new to insert), then reply naming what changed. Or reply saying the edit was not made and why."
	}
	if l.editAsked && len(l.read) > 0 && !strings.ContainsAny(reply, "?？") && !claimsNoChange.MatchString(reply) && !declinesEdit.MatchString(reply) {
		l.unlandedNudge = true
		return "[system] They asked for a change and this turn only read. A diagnosis is not the change. " +
			"Make it now with fs__file_patch (old exactly as the read showed it, or after_line with new), run the check, " +
			"then reply naming what changed. Or reply saying why the change should not be made."
	}
	return ""
}

var (
	codeExt  = regexp.MustCompile(`(?i)\.(go|py|js|jsx|ts|tsx|rs|java|kt|c|h|cc|cpp|hpp|rb|php|cs|swift|scala)$`)
	testFile = regexp.MustCompile(`(?i)(_test\.(go|py)|(^|/)test_[^/]*\.py|\.(test|spec)\.[jt]sx?|Test\.(java|kt)|_spec\.rb)$`)
	funcDecl = regexp.MustCompile(`(?m)^\s*(?:func(?:\s*\([^)]*\))?|def|function|fn|pub fn)\s+([A-Za-z_]\w*)\s*\(`)
)

// noteWrite marks a code write, a test write, and the functions it adds:
// declared in what was written, not in what it replaced. Caller holds mu.
func (l *turnLanes) noteWrite(p string, args json.RawMessage) {
	if !codeExt.MatchString(p) {
		return
	}
	l.codeWritten = true
	l.ranSinceCode = false
	if testFile.MatchString(p) {
		l.testWritten = true
		return
	}
	var a struct{ Old, New, Diff, Body, Content string }
	_ = json.Unmarshal(args, &a)
	added, removed := a.New+a.Body+a.Content, a.Old
	for _, line := range strings.Split(a.Diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added += line[1:] + "\n"
		case strings.HasPrefix(line, "-"):
			removed += line[1:] + "\n"
		}
	}
	had := map[string]bool{}
	for _, m := range funcDecl.FindAllStringSubmatch(removed, -1) {
		had[m[1]] = true
	}
	for _, m := range funcDecl.FindAllStringSubmatch(added, -1) {
		if !had[m[1]] && !slices.Contains(l.addedFuncs, m[1]) {
			l.addedFuncs = append(l.addedFuncs, m[1])
		}
	}
}

// unfinished is the nudge for a turn about to reply with code work left:
// a new function with no test written (twice: after the first, the model
// often runs the old tests and stops), or code written with no command run
// after it (once). canRun is a command tool this turn.
func (l *turnLanes) unfinished(canRun bool) string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.addedFuncs) > 0 && !l.testWritten && l.testNudges < 2 {
		l.testNudges++
		return "[system] You added " + strings.Join(l.addedFuncs, ", ") + " but wrote no test this turn. " +
			"New behaviour comes with its test: read the matching test file, add a test that calls it, then run the tests."
	}
	if canRun && l.codeWritten && !l.ranSinceCode && !l.checkNudged {
		l.checkNudged = true
		return "[system] You changed code but no command ran after it. " +
			"Run the repo's test and lint now with shell__command_run, then reply naming what changed and what they returned."
	}
	return ""
}

// A reply that reports a check result, a change, or no change. The turn's
// own calls say whether each is true.
var (
	claimsCheckPassed = regexp.MustCompile(`(?i)\b(tests?|lint|linter|build|vet|checks?)\b[^\n]{0,24}?\b(pass(es|ed)?|passing|succeed(s|ed)?|green|clean)\b|\bexit (code |status )?0\b`)
	saysNotRun        = regexp.MustCompile(`(?i)(\bnot|n't|\bnever) (been )?(run|ran|verified|checked)\b|\bunverified\b|\buntested\b`)
	claimsNoChange    = regexp.MustCompile(`(?i)\bno (code|files?|changes?) (was |were |is )?(changed|made|modified|needed|necessary)\b|\bnothing (was )?(changed|modified)\b`)
	claimsEdit        = regexp.MustCompile(`(?i)\b(fixed|patched|renamed|inserted|appended)\b|\bnow (returns|reads|says|uses|calls)\b|\b(added|wrote|updated|edited)\b[^\n.]{0,40}?\b(to|in|into) ` + "`?" + `[\w./-]+\.\w{1,4}\b`)
	saysNotEdited     = regexp.MustCompile(`(?i)(\bnot|n't|\bnever) (been |yet )?(fixed|patched|renamed|changed|inserted|appended|added|written|updated|edited)\b`)
)

// declinesEdit is a read-only turn saying the asked change is not wanted:
// the file already has it, or there is nothing to do.
var declinesEdit = regexp.MustCompile(`(?i)\bnothing to (add|change|fix|do|remove|update)\b|\balready (has|have|contains?|does|is|are|returns?|says|prints?|reads)\b|\bshould not be (changed|made|fixed)\b`)

// contradiction is the nudge for a reply the turn's calls disprove: a check
// that passed with no command run, a change with nothing written, or no
// change after a write landed. Empty when the reply holds.
func (l *turnLanes) contradiction(reply string) string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	// A check that ran before the last code change says nothing about the
	// code as it is now: "go test, patch, 'tests pass'" is the same false
	// claim as never running it.
	stale := l.codeWritten && !l.ranSinceCode
	if (!l.ran || stale) && claimsCheckPassed.MatchString(reply) && !saysNotRun.MatchString(reply) {
		switch {
		case len(l.patched) == 0:
			return "[system] Your reply reports a check result, but nothing produced it. No command ran and no file was patched this turn. " +
				"Do the work now with the real tool calls (the patch, then the check), or reply again saying only what a tool returned this turn."
		case l.ran:
			return "[system] Your reply reports a check result, but no command ran after the last code change; the run you have is from before it. " +
				"Run the check again now with shell__command_run, or reply again saying what changed and that it was not re-checked."
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
