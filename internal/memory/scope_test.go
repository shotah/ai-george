package memory_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/shotah/george/internal/memory"
	"github.com/shotah/george/internal/session"
)

func subjects(es []memory.Entry) map[string]bool {
	out := map[string]bool{}
	for _, e := range es {
		out[e.Subject] = true
	}
	return out
}

func TestBuiltin_RepoScope(t *testing.T) {
	ctx := context.Background()
	store, err := session.Open(t.TempDir(), 50, 8000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	a, err := memory.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	a.Repo = "github.com/me/a"
	b, err := memory.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	b.Repo = "github.com/me/b"

	if _, err := a.Store(ctx, memory.KindFact, "cmd/test", "make test in a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store(ctx, memory.KindPreference, "pref/commits", "small commits"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Store(ctx, memory.KindFact, "cmd/test", "go test ./... in b"); err != nil {
		t.Fatal(err)
	}

	for name, m := range map[string]*memory.Builtin{"a": a, "b": b} {
		hyd, err := m.Hydrate(ctx, "test commits", 30)
		if err != nil {
			t.Fatal(err)
		}
		if got := subjects(hyd); !got["pref/commits"] || !got["cmd/test"] {
			t.Fatalf("%s hydrate = %v, want its cmd/test and the shared pref", name, got)
		}
		recall, err := m.Recall(ctx, "test", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(recall) != 1 {
			t.Fatalf("%s recall = %#v, want only its own cmd/test", name, recall)
		}
	}
	liveA, ok, err := a.ActiveByKindSubject(ctx, memory.KindFact, "cmd/test")
	if err != nil || !ok || liveA.Content != "make test in a" {
		t.Fatalf("b's cmd/test superseded a's: %#v ok=%v err=%v", liveA, ok, err)
	}
	var scopes int
	if err := store.DB().QueryRow(`SELECT COUNT(DISTINCT scope) FROM memory`).Scan(&scopes); err != nil || scopes != 3 {
		t.Fatalf("distinct scopes = %d, %v; want user, a, b", scopes, err)
	}
}

func TestBuiltin_OldDBGetsScope(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "george.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE memory (
		id INTEGER PRIMARY KEY, kind TEXT NOT NULL, subject TEXT NOT NULL,
		content TEXT NOT NULL, source TEXT NOT NULL, confidence REAL NOT NULL DEFAULT 1.0,
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL, expires_at TEXT,
		superseded_by INTEGER, consolidated INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO memory (kind, subject, content, source, created_at, updated_at)
		VALUES ('fact', 'cmd/lint', 'make lint', 'chat', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	m, err := memory.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	m.Repo = "github.com/me/a"
	live, ok, err := m.ActiveByKindSubject(context.Background(), memory.KindFact, "cmd/lint")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("an old row is user scope, not this repo's fact: %#v", live)
	}
	hyd, err := m.Hydrate(context.Background(), "lint", 30)
	if err != nil {
		t.Fatal(err)
	}
	if !subjects(hyd)["cmd/lint"] {
		t.Fatalf("old row should hydrate everywhere as user scope, got %v", subjects(hyd))
	}
}
