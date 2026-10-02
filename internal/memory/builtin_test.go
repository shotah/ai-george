package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/shotah/george/internal/memory"
	"github.com/shotah/george/internal/session"
)

func TestBuiltin_StoreRecallForget(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	b, err := memory.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	e, err := b.Store(ctx, memory.KindPreference, "chris", "coaching tone, no fluff")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 {
		t.Fatal("expected id")
	}

	hits, err := b.Recall(ctx, "coaching", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Content != e.Content {
		t.Fatalf("recall = %#v", hits)
	}

	if err := b.Forget(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	hits, err = b.Recall(ctx, "coaching", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected empty after forget, got %#v", hits)
	}
}

func TestBuiltin_SurvivesSessionReset(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := session.Open(dir, 50, 8000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	mem, err := memory.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mem.Store(ctx, memory.KindPerson, "mom", "prefers calls over texts"); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, "s1",
		session.Message{Role: session.RoleUser, Content: "hi"},
		session.Message{Role: session.RoleAssistant, Content: "hey"},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Reset(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	msgs, err := store.Messages(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("session should be empty, got %d", len(msgs))
	}

	hits, err := mem.Recall(ctx, "mom", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("memory should survive /new, got %#v", hits)
	}
}

func TestBuiltin_HydrateAndForgetQuery(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	_, _ = b.Store(ctx, memory.KindFact, "climbing", "trains Tue/Thu")
	_, _ = b.Store(ctx, memory.KindEpisode, "today", "skipped gym while traveling")

	hydrated, err := b.Hydrate(ctx, "traveling", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(hydrated) < 1 {
		t.Fatal("expected hydrate rows")
	}
	block := memory.FormatHydration(hydrated, time.UTC)
	if block == "" || block[:8] != "[memory]" {
		t.Fatalf("bad hydration block: %q", block)
	}

	n, err := b.ForgetQuery(ctx, "climbing")
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected forget count >= 1, got %d", n)
	}
}

func TestBuiltin_StoreSupersedesSameSubject(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	old, err := b.Store(ctx, memory.KindPreference, "pref/food", "dislikes sushi")
	if err != nil {
		t.Fatal(err)
	}
	neu, err := b.Store(ctx, memory.KindPreference, "pref/food", "likes sushi now")
	if err != nil {
		t.Fatal(err)
	}
	if neu.ID == old.ID {
		t.Fatal("expected a new row")
	}
	got, err := b.Get(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SupersededBy == nil || *got.SupersededBy != neu.ID {
		t.Fatalf("old superseded_by=%v want %d", got.SupersededBy, neu.ID)
	}
	live, ok, err := b.ActiveByKindSubject(ctx, memory.KindPreference, "pref/food")
	if err != nil || !ok {
		t.Fatalf("live ok=%v err=%v", ok, err)
	}
	if live.ID != neu.ID || live.Content != "likes sushi now" {
		t.Fatalf("live=%#v", live)
	}
	hydrated, err := b.Hydrate(ctx, "sushi", 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range hydrated {
		if e.ID == old.ID {
			t.Fatalf("superseded row still hydrated: %#v", e)
		}
	}
	ep1, err := b.Store(ctx, memory.KindEpisode, "chris", "note one")
	if err != nil {
		t.Fatal(err)
	}
	ep2, err := b.Store(ctx, memory.KindEpisode, "chris", "note two")
	if err != nil {
		t.Fatal(err)
	}
	if ep1.ID == ep2.ID {
		t.Fatal("episodes with the same subject must not collapse")
	}
	first, err := b.Get(ctx, ep1.ID)
	if err != nil || first.SupersededBy != nil {
		t.Fatalf("episodes collapsed: %#v err=%v", first, err)
	}
}

func TestTools_CallRoundTrip(t *testing.T) {
	ctx := context.Background()
	b, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	tools := memory.Tools{Backend: b}
	out, err := tools.Call(ctx, memory.ToolStore, []byte(`{"kind":"fact","subject":"x","content":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty store result")
	}
	out, err = tools.Call(ctx, memory.ToolRecall, []byte(`{"query":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "no memories matched" {
		t.Fatal(out)
	}
	out, err = tools.Call(ctx, memory.ToolForget, []byte(`{"query":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty forget result")
	}
}
