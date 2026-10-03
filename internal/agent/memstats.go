package agent

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/shotah/george/internal/memory"
)

func (a *Agent) formatMemStats(ctx context.Context) string {
	if a.memory == nil {
		return "memory: disabled"
	}
	builtin, ok := a.memory.(*memory.Builtin)
	if !ok {
		return "memory: mcp backend (no local row counts)"
	}
	snap, err := builtin.Stats(ctx)
	if err != nil {
		return fmt.Sprintf("memory: stats failed: %v", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "memory: %d rows  fact=%d preference=%d person=%d insight=%d episode=%d\n",
		snap.Total,
		snap.ByKind[memory.KindFact],
		snap.ByKind[memory.KindPreference],
		snap.ByKind[memory.KindPerson],
		snap.ByKind[memory.KindInsight],
		snap.ByKind[memory.KindEpisode],
	)
	fmt.Fprintf(&b, "state: active=%d expired=%d superseded=%d\n",
		snap.Active, snap.Expired, snap.Superseded)
	scopes := slices.Sorted(maps.Keys(snap.ByScope))
	parts := make([]string, 0, len(scopes))
	for _, s := range scopes {
		p := fmt.Sprintf("%s=%d", s, snap.ByScope[s])
		if s == builtin.Repo {
			p += " (this repo)"
		}
		parts = append(parts, p)
	}
	fmt.Fprintf(&b, "scopes: %s\n", strings.Join(parts, " "))
	fmt.Fprintf(&b, "db: %s (WAL)", formatBytes(snap.DBBytes))
	return b.String()
}

// memoryMove is `/memory move <old repo id>`: after a remote rename, bring
// the old id's rows into this repo.
func (a *Agent) memoryMove(ctx context.Context, args []string) string {
	builtin, ok := a.memory.(*memory.Builtin)
	if !ok {
		return "memory: move needs the builtin backend"
	}
	if len(args) != 2 || args[0] != "move" {
		return "usage: /memory move <old repo id> (the ids are under scopes: in /memstats)"
	}
	n, err := builtin.Move(ctx, args[1])
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("moved %d row(s) from %s to %s", n, args[1], builtin.Repo)
}

func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const mb = 1024 * 1024
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
	const kb = 1024
	if n >= kb {
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	}
	return fmt.Sprintf("%d B", n)
}
