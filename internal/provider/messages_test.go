package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shotah/george/internal/provider"
)

func TestWireMessages_ManyKeepsTrailingHarness(t *testing.T) {
	in := []provider.Message{
		{Role: provider.RoleSystem, Content: "You are Kit."},
		{Role: provider.RoleUser, Content: "what's near me"},
		{Role: provider.RoleSystem, Content: "[harness]\n[location] 47.6\n[current time] NOW"},
	}
	got := provider.WireMessagesMode(provider.FoldMany, in)
	if len(got) != 3 || got[2].Role != provider.RoleSystem || !strings.Contains(got[2].Content, "[harness]") {
		t.Fatalf("many wire %+v", got)
	}
	if got[0].Content != "You are Kit." || got[1].Content != "what's near me" {
		t.Fatalf("many mutated standing turns %+v", got)
	}
}

func TestWireMessages_FoldOneFoldsHarnessIntoSystem(t *testing.T) {
	in := []provider.Message{
		{Role: provider.RoleSystem, Content: "You are Kit."},
		{Role: provider.RoleUser, Content: "old"},
		{Role: provider.RoleAssistant, Content: "prior"},
		{Role: provider.RoleSystem, Content: "[memory] a fact"},
		{Role: provider.RoleUser, Content: "what's near me"},
		{Role: provider.RoleSystem, Content: "[harness]\n[location] 47.6, -122.3\n[current time] NOW"},
		{Role: provider.RoleSystem, Content: "[system] wait note"},
	}
	got := provider.WireMessagesMode(provider.FoldOne, in)
	if len(got) != 4 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].Role != provider.RoleSystem {
		t.Fatalf("first role %s", got[0].Role)
	}
	sys := got[0].Content
	if !strings.HasPrefix(sys, "You are Kit.") {
		t.Fatalf("persona must lead the one system instruction:\n%s", sys)
	}
	if !strings.Contains(sys, "[location] 47.6") || !strings.Contains(sys, "[current time] NOW") {
		t.Fatalf("clock/GPS missing from system:\n%s", sys)
	}
	if !strings.Contains(sys, "[memory] a fact") || !strings.Contains(sys, "wait note") {
		t.Fatalf("standing system dropped:\n%s", sys)
	}
	// Standing (persona, hydration) before this-turn (harness, wait note):
	// identity first, stable prefix cacheable, clock last for recency.
	persona, mem := strings.Index(sys, "You are Kit."), strings.Index(sys, "[memory]")
	harness, wait := strings.Index(sys, "[harness]"), strings.Index(sys, "wait note")
	if persona >= mem || mem >= harness || harness >= wait {
		t.Fatalf("fold order persona=%d memory=%d harness=%d wait=%d:\n%s", persona, mem, harness, wait, sys)
	}
	if got[1].Role != provider.RoleUser || got[1].Content != "old" {
		t.Fatalf("history user %+v", got[1])
	}
	if got[2].Role != provider.RoleAssistant || got[3].Content != "what's near me" {
		t.Fatalf("later turns %+v %+v", got[2], got[3])
	}
	for i, m := range got[1:] {
		if m.Role == provider.RoleSystem {
			t.Fatalf("trailing system still on wire at %d: %+v", i+1, m)
		}
	}
}

func TestWireMessagesMode_Modes(t *testing.T) {
	in := []provider.Message{
		{Role: provider.RoleSystem, Content: "You are Kit."},
		{Role: provider.RoleUser, Content: "hi"},
		{Role: provider.RoleSystem, Content: "[harness] NOW"},
	}
	for _, mode := range []string{provider.FoldOne, provider.FoldAuto, "", "sideways"} {
		got := provider.WireMessagesMode(mode, in)
		if len(got) != 2 || !strings.HasPrefix(got[0].Content, "You are Kit.") || !strings.HasSuffix(got[0].Content, "[harness] NOW") {
			t.Fatalf("mode %q should fold: %+v", mode, got)
		}
	}
	if got := provider.WireMessagesMode(provider.FoldMany, in); len(got) != 3 || got[2].Content != "[harness] NOW" {
		t.Fatalf("many should keep the layout: %+v", got)
	}
}

func TestClient_Complete_DefaultFolds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || !strings.Contains(body.Messages[0].Content, "[harness]") {
			t.Fatalf("fold=one wire %+v", body.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "x",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "gemma4")
	if _, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are Kit."},
			{Role: provider.RoleUser, Content: "what's near me"},
			{Role: provider.RoleSystem, Content: "[harness]\n[current time] NOW"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestClient_Complete_FoldManyKeepsTrailingSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) != 3 || body.Messages[2].Role != "system" || !strings.Contains(body.Messages[2].Content, "[harness]") {
			t.Fatalf("openai wire %+v", body.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "x",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "gpt-5.4").WithSystemFold(provider.FoldMany)
	if _, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are Kit."},
			{Role: provider.RoleUser, Content: "what's near me"},
			{Role: provider.RoleSystem, Content: "[harness]\n[current time] NOW"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}
