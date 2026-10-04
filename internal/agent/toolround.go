package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shotah/george/internal/channel"
	"github.com/shotah/george/internal/mcp"
	"github.com/shotah/george/internal/provider"
)

type toolRoundResult struct {
	name string
	id   string
	out  string
	err  error
}

type toolRound struct {
	results    []toolRoundResult
	forceNames []string
	wall       time.Duration
}

// runToolRound executes one model-emitted tool batch. Independent calls run
// concurrently; results are appended in the original ToolCalls order so the
// provider still sees a matching tool_call_id sequence. Same-server stdio
// serializes inside the MCP host.
func (a *Agent) runToolRound(ctx context.Context, calls []provider.ToolCall, iter int, hasProgress bool, progress channel.ProgressWriter) (toolRound, bool) {
	n := len(calls)
	out := toolRound{results: make([]toolRoundResult, n)}
	if n == 0 {
		return out, false
	}
	if hasProgress && a.toolTrace == ToolTraceCompact {
		_ = progress.UpdateProgress(ctx, compactCallsHeader)
	}

	views := make([]callView, n)
	for i, c := range calls {
		views[i] = callView{name: c.Name, args: json.RawMessage(c.Arguments)}
	}
	refusals := lanesFrom(ctx).check(views)

	start := time.Now()
	if n == 1 {
		out.results[0] = a.execToolCall(ctx, calls[0], iter, hasProgress, progress, refusals[0])
	} else {
		var wg sync.WaitGroup
		wg.Add(n)
		for i, call := range calls {
			go func() {
				defer wg.Done()
				out.results[i] = a.execToolCall(ctx, call, iter, hasProgress, progress, refusals[i])
			}()
		}
		wg.Wait()
	}
	out.wall = time.Since(start)

	canceled := false
	for _, r := range out.results {
		if r.err == nil {
			continue
		}
		if errors.Is(r.err, context.Canceled) || ctx.Err() != nil {
			canceled = true
			continue
		}
		var unknown *mcp.UnknownToolError
		if errors.As(r.err, &unknown) && len(unknown.Candidates) > 0 && len(out.forceNames) == 0 {
			out.forceNames = unknown.Candidates
			a.log.Info("constraining retry to nearest tool names",
				"requested", unknown.Name,
				"candidates", len(unknown.Candidates),
			)
		}
	}
	return out, canceled
}

// repairOffset answers a read past the end with the page the newest paged
// read of that file said came next. The model reads a cut at line 59, then
// asks for offset 200 (its schema's default page) and gets an error that
// costs the round; the host knows where the rest starts. Only the error
// path is rewritten, and only when this turn showed an earlier page, so a
// read that was always past the end stays an error.
func (a *Agent) repairOffset(ctx context.Context, name string, args json.RawMessage, orig error) (string, error) {
	mark, ok := lanesFrom(ctx).continuation(args)
	if !ok {
		return "", orig
	}
	var m map[string]any
	if json.Unmarshal(args, &m) != nil {
		return "", orig
	}
	asked, _ := m["offset"].(float64)
	m["offset"] = mark.next
	fixed, err := json.Marshal(m)
	if err != nil {
		return "", orig
	}
	text, err := a.tools.Call(ctx, name, fixed)
	if err != nil {
		return "", orig
	}
	a.offsetRepairs.Add(1)
	a.log.Info("tool call offset repaired", "name", name, "asked", int(asked), "offset", mark.next)
	return fmt.Sprintf("[offset %d is past the end; the last read of this file stopped at line %d, so this is the page from line %d]\n%s",
		int(asked), mark.last, mark.next, text), nil
}

func (a *Agent) execToolCall(ctx context.Context, call provider.ToolCall, iter int, hasProgress bool, progress channel.ProgressWriter, refusal error) toolRoundResult {
	a.log.Info("tool call",
		"name", call.Name,
		"id", call.ID,
		"iteration", iter+1,
	)
	if hasProgress && a.toolTrace == ToolTraceFull {
		_ = progress.UpdateProgress(ctx, toolProgressStart(call.Name))
	}
	args := json.RawMessage(call.Arguments)
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	shown, hasShown := channel.ToolWriterFrom(ctx)
	hasShown = hasShown && a.toolTrace != ToolTraceOff
	if hasShown {
		shown.ToolStart(ctx, call.Name, args)
	}
	toolStart := time.Now()
	err := refusal
	if err == nil {
		err = a.guardEnable(ctx, call.Name)
	}
	if err != nil {
		text := fmt.Sprintf("tool error: %v", err)
		a.log.Info("tool call blocked", "name", call.Name, "err", err)
		if hasShown {
			shown.ToolDone(ctx, call.Name, args, err)
		}
		if hasProgress && a.toolTrace == ToolTraceCompact {
			_ = progress.UpdateProgress(ctx, "✗")
		}
		return toolRoundResult{name: call.Name, id: call.ID, out: text, err: err}
	}
	text, err := a.tools.Call(ctx, call.Name, args)
	if err != nil && laneOf(call.Name) == laneRead && offsetTooFar.MatchString(err.Error()) {
		text, err = a.repairOffset(ctx, call.Name, args, err)
	}
	dur := time.Since(toolStart)
	if err != nil {
		text = fmt.Sprintf("tool error: %v", err)
		a.log.Info("tool call failed", "name", call.Name, "dur_ms", dur.Milliseconds(), "err", err)
		lanesFrom(ctx).noteFailure(call.Name, args, err)
	} else {
		a.log.Info("tool done",
			"name", call.Name,
			"dur_ms", dur.Milliseconds(),
			"result_chars", len(text),
		)
		a.touchEnable(ctx, call.Name)
		lanesFrom(ctx).record(call.Name, args)
		lanesFrom(ctx).noteResult(call.Name, args, text)
	}
	if hasShown {
		shown.ToolDone(ctx, call.Name, args, err)
	}
	if hasProgress {
		switch a.toolTrace {
		case ToolTraceFull:
			_ = progress.UpdateProgress(ctx, toolProgressDone(dur, len(text), err != nil))
		case ToolTraceCompact:
			mark := "✓"
			if err != nil {
				mark = "✗"
			}
			_ = progress.UpdateProgress(ctx, mark)
		}
	}
	return toolRoundResult{name: call.Name, id: call.ID, out: text, err: err}
}
