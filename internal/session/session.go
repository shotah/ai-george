// Package session stores bounded conversation history in SQLite.
package session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (database/sql)
)

// Role values persisted for conversation turns (system/persona is not stored).
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is one persisted conversation turn.
type Message struct {
	Role      string
	Content   string
	CreatedAt string // set when trimming so fold-failure restore keeps order
}

// Store is a SQLite-backed session history.
type Store struct {
	db           *sql.DB
	maxMessages  int
	maxEstTokens int
}

// Open opens (or creates) george.db under dataDir and runs migrations.
func Open(dataDir string, maxMessages, maxEstTokens int) (*Store, error) {
	if maxMessages < 1 {
		maxMessages = 200
	}
	if maxEstTokens < 1 {
		maxEstTokens = 32000
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("session: mkdir data dir: %w", err)
	}
	dbPath := filepath.Join(dataDir, "george.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("session: open db: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite: serialize writers
	db.SetMaxIdleConns(1)

	s := &Store{db: db, maxMessages: maxMessages, maxEstTokens: maxEstTokens}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA foreign_keys=ON;`,
		`CREATE TABLE IF NOT EXISTS session (
			id                 TEXT PRIMARY KEY,
			summary            TEXT NOT NULL DEFAULT '',
			updated_at         TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS session_message (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE,
			role       TEXT NOT NULL,
			content    TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_session_message_session
			ON session_message(session_id, id);`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("session: migrate: %w", err)
		}
	}
	return nil
}

// DB returns the underlying database handle (shared by memory when builtin).
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Messages returns the bounded history for sessionID (oldest first).
func (s *Store) Messages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT role, content
		FROM session_message
		WHERE session_id = ?
		ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session: query messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, fmt.Errorf("session: scan message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Append writes turns and trims the session to configured bounds.
func (s *Store) Append(ctx context.Context, sessionID string, msgs ...Message) error {
	if len(msgs) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("session: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session (id, summary, updated_at) VALUES (?, '', ?)
		ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at`,
		sessionID, now); err != nil {
		return fmt.Errorf("session: upsert session: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO session_message (session_id, role, content, created_at)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("session: prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		if role != RoleUser && role != RoleAssistant {
			return fmt.Errorf("session: invalid role %q", m.Role)
		}
		if _, err := stmt.ExecContext(ctx, sessionID, role, m.Content, now); err != nil {
			return fmt.Errorf("session: insert message: %w", err)
		}
	}

	dropIDs, err := s.planTrimTx(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if err := s.deleteMessageIDs(ctx, tx, dropIDs); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("session: commit: %w", err)
	}
	return nil
}

// Reset deletes all history for sessionID (memory untouched — different store).
func (s *Store) Reset(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM session WHERE id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("session: reset: %w", err)
	}
	return nil
}

// Stats returns message count and estimated tokens (chars/4) for a session.
func (s *Store) Stats(ctx context.Context, sessionID string) (messages int, estTokens int, err error) {
	msgs, err := s.Messages(ctx, sessionID)
	if err != nil {
		return 0, 0, err
	}
	return len(msgs), EstTokens(msgs), nil
}

// planTrimTx returns the ids of the oldest messages to remove to satisfy bounds.
func (s *Store) planTrimTx(ctx context.Context, tx *sql.Tx, sessionID string) ([]int64, error) {
	var ids []int64
	var skipChars int
	for {
		var count int
		var chars int
		err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*), COALESCE(SUM(LENGTH(content)), 0)
			FROM session_message WHERE session_id = ?`, sessionID).Scan(&count, &chars)
		if err != nil {
			return nil, fmt.Errorf("session: trim stats: %w", err)
		}
		remain := count - len(ids)
		est := (chars - skipChars + 3) / 4
		if remain <= s.maxMessages && est <= s.maxEstTokens {
			return ids, nil
		}
		if remain <= 2 {
			return ids, nil
		}

		var id int64
		var content string
		err = tx.QueryRowContext(ctx, `
			SELECT id, content FROM session_message
			WHERE session_id = ?
			ORDER BY id ASC
			LIMIT 1 OFFSET ?`, sessionID, len(ids)).Scan(&id, &content)
		if err == sql.ErrNoRows {
			return ids, nil
		}
		if err != nil {
			return nil, fmt.Errorf("session: trim peek: %w", err)
		}
		ids = append(ids, id)
		skipChars += len(content)
	}
}

func (s *Store) deleteMessageIDs(ctx context.Context, tx *sql.Tx, ids []int64) error {
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM session_message WHERE id = ?`, id); err != nil {
			return fmt.Errorf("session: trim delete: %w", err)
		}
	}
	return nil
}

// EstTokens returns the chars/4 token estimate for messages.
func EstTokens(msgs []Message) int {
	n := 0
	for _, m := range msgs {
		n += (len(m.Content) + 3) / 4
	}
	return n
}
