package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/shotah/george/internal/mcpenable"
	"github.com/shotah/george/internal/provider"
	"github.com/shotah/george/internal/websearch"
)

func (a *Agent) publishedTools(ctx context.Context, sessionID string) []provider.ToolDef {
	if a.tools == nil {
		return nil
	}
	all := a.tools.Tools()
	if a.enable == nil {
		return all
	}
	now := time.Now()
	rows, err := a.enable.List(ctx, sessionID, now)
	if err != nil {
		a.log.Warn("mcp enable list failed; publishing builtins only", "err", err)
		return mcpenable.Publish(all, nil, a.enableForce)
	}
	return mcpenable.Publish(all, rows, a.enableForce)
}

func (a *Agent) enableIndexBlock(ctx context.Context, sessionID string) string {
	if a.enable == nil || a.tools == nil {
		return ""
	}
	now := time.Now()
	rows, err := a.enable.List(ctx, sessionID, now)
	if err != nil {
		return ""
	}
	return mcpenable.FormatIndex(rows, mcpenable.Index(a.tools.Tools()), a.enableForce)
}

func (a *Agent) guardEnable(ctx context.Context, name string) error {
	if a.enable == nil || mcpenable.AlwaysOn(name) || name == mcpenable.ToolName || websearch.IsSearchTool(name) {
		return nil
	}
	sessionID := mcpenable.SessionID(ctx)
	if sessionID == "" {
		return nil
	}
	now := time.Now()
	rows, err := a.enable.List(ctx, sessionID, now)
	if err != nil {
		return err
	}
	var keys []string
	for _, r := range rows {
		keys = append(keys, r.Prefix)
	}
	if mcpenable.Allowed(name, keys, a.enableForce) {
		return nil
	}
	return fmt.Errorf("%s", mcpenable.EnableHint(name, a.catalogIndex()))
}

func (a *Agent) touchEnable(ctx context.Context, name string) {
	if a.enable == nil || mcpenable.AlwaysOn(name) || websearch.IsSearchTool(name) {
		return
	}
	sessionID := mcpenable.SessionID(ctx)
	if sessionID == "" {
		return
	}
	if err := a.enable.Touch(ctx, sessionID, name, time.Now()); err != nil {
		a.log.Debug("mcp enable touch skipped", "err", err)
	}
}

func (a *Agent) catalogIndex() []string {
	if a.tools == nil {
		return nil
	}
	return mcpenable.Index(a.tools.Tools())
}
