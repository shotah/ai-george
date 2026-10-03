package session

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const (
	factsLabel = "Facts:"
	voiceLabel = "Voice:"
)

// LedgerParts splits a session summary into Facts and Voice.
// An unlabeled legacy paragraph is returned as facts with empty voice.
func LedgerParts(s string) (facts, voice string) {
	facts, voice = splitLedger(s)
	return strings.TrimSpace(facts), strings.TrimSpace(voice)
}

func splitLedger(s string) (facts, voice string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	voiceAt := indexLineLabel(s, voiceLabel)
	if voiceAt < 0 {
		if factsAt := indexLineLabel(s, factsLabel); factsAt == 0 {
			return strings.TrimSpace(s[len(factsLabel):]), ""
		}
		return s, ""
	}
	head := strings.TrimSpace(s[:voiceAt])
	voice = strings.TrimSpace(s[voiceAt+len(voiceLabel):])
	if factsAt := indexLineLabel(head, factsLabel); factsAt == 0 {
		facts = strings.TrimSpace(head[len(factsLabel):])
	} else {
		facts = head
	}
	return facts, voice
}

func indexLineLabel(s, label string) int {
	lower := strings.ToLower(s)
	want := strings.ToLower(label)
	if strings.HasPrefix(lower, want) {
		return 0
	}
	i := strings.Index(lower, "\n"+want)
	if i < 0 {
		return -1
	}
	return i + 1
}

// Summary returns the rolling summary for sessionID (empty if none).
func (s *Store) Summary(ctx context.Context, sessionID string) (string, error) {
	var summary string
	err := s.db.QueryRowContext(ctx, `
		SELECT summary FROM session WHERE id = ?`, sessionID).Scan(&summary)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("session: summary: %w", err)
	}
	return summary, nil
}
