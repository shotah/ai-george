package session_test

import (
	"context"
	"testing"

	"github.com/shotah/george/internal/session"
)

func TestStore_TrimDropsWithoutSummary(t *testing.T) {
	ctx := context.Background()
	store, err := session.Open(t.TempDir(), 4, 100000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	id := "s1"
	for i := 0; i < 3; i++ {
		if err := store.Append(ctx, id,
			session.Message{Role: session.RoleUser, Content: "u"},
			session.Message{Role: session.RoleAssistant, Content: "a"},
		); err != nil {
			t.Fatal(err)
		}
	}
	msgs, err := store.Messages(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("messages=%d want 4", len(msgs))
	}
	sum, err := store.Summary(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if sum != "" {
		t.Fatalf("trim wrote a summary: %q", sum)
	}
}

func TestLedgerParts(t *testing.T) {
	facts, voice := session.LedgerParts("Facts: likes espresso\nVoice: dry; gag: \"gull\"")
	if facts != "likes espresso" || voice != "dry; gag: \"gull\"" {
		t.Fatalf("facts=%q voice=%q", facts, voice)
	}
	facts, voice = session.LedgerParts("unlabeled legacy paragraph")
	if facts != "unlabeled legacy paragraph" || voice != "" {
		t.Fatalf("legacy facts=%q voice=%q", facts, voice)
	}
}
