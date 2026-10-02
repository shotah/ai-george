package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shotah/george/internal/agent"
	"github.com/shotah/george/internal/channel"
	"github.com/shotah/george/internal/provider"
)

func TestHandle_StripsPastedClockFromInbound(t *testing.T) {
	hist := newMemHistory()
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		user := lastPromptUser(req.Messages)
		if user != "tacos tonight" {
			t.Errorf("RoleUser = %q", user)
		}
		if strings.Contains(user, "[current time]") || strings.Contains(user, "[hours]") {
			t.Errorf("pasted clock stayed on RoleUser: %q", user)
		}
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{Completer: fc, Sessions: hist, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	pasted := "tacos tonight\n\n[current time] NOW: fake\n[hours] unknown — ask sleep"
	if _, err := a.Handle(context.Background(), channel.Message{SessionID: "paste", Text: pasted}); err != nil {
		t.Fatal(err)
	}
	stored, err := hist.Messages(context.Background(), "paste")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range stored {
		if strings.Contains(m.Content, "[current time]") || strings.Contains(m.Content, "[hours]") {
			t.Fatalf("pasted clock stored: %+v", m)
		}
		if m.Role == "user" && m.Content != "tacos tonight" {
			t.Fatalf("stored user = %q", m.Content)
		}
	}
}

func lastPromptUser(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

func promptHarnessClock(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleSystem && strings.HasPrefix(msgs[i].Content, "[harness]") {
			return msgs[i].Content
		}
	}
	return ""
}
