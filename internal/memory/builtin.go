package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const (
	defaultHydrateLimit = 30
	defaultRecallLimit  = 20
	episodeTTL          = 30 * 24 * time.Hour
)

// ScopeUser marks a row about the human, read in every repo.
const ScopeUser = "user"

// Builtin is the SQLite + FTS5 memory backend in george.db.
type Builtin struct {
	db    *sql.DB
	owned bool // close db on Close when we opened it
	// Repo is this process's repo id. Facts, goals, insights, and episodes
	// are stored under it and read only in that repo; preferences and people
	// are ScopeUser. Empty stores and reads every row as ScopeUser.
	Repo string
}

// scopeFor is the scope a row of kind is stored and superseded under.
func (b *Builtin) scopeFor(kind string) string {
	if b.Repo == "" || kind == KindPreference || kind == KindPerson {
		return ScopeUser
	}
	return b.Repo
}

// readScope is the repo half of the `scope IN (user, repo)` read filter.
func (b *Builtin) readScope() string {
	if b.Repo == "" {
		return ScopeUser
	}
	return b.Repo
}

// Open opens (or creates) memory tables in dataDir/george.db.
func Open(dataDir string) (*Builtin, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("memory: mkdir: %w", err)
	}
	dbPath := filepath.Join(dataDir, "george.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("memory: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	b := &Builtin{db: db, owned: true}
	if err := b.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return b, nil
}

// OpenDB attaches memory schema to an existing DB handle (shared with session).
func OpenDB(db *sql.DB) (*Builtin, error) {
	if db == nil {
		return nil, fmt.Errorf("memory: nil db")
	}
	b := &Builtin{db: db, owned: false}
	if err := b.migrate(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Builtin) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA foreign_keys=ON;`,
		`CREATE TABLE IF NOT EXISTS memory (
			id            INTEGER PRIMARY KEY,
			kind          TEXT NOT NULL,
			subject       TEXT NOT NULL,
			content       TEXT NOT NULL,
			source        TEXT NOT NULL,
			confidence    REAL NOT NULL DEFAULT 1.0,
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL,
			expires_at    TEXT,
			superseded_by INTEGER,
			consolidated  INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_active
			ON memory(kind, updated_at DESC);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
			subject, content,
			content='memory',
			content_rowid='id'
		);`,
		`CREATE TRIGGER IF NOT EXISTS memory_ai AFTER INSERT ON memory BEGIN
			INSERT INTO memory_fts(rowid, subject, content)
			VALUES (new.id, new.subject, new.content);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_ad AFTER DELETE ON memory BEGIN
			INSERT INTO memory_fts(memory_fts, rowid, subject, content)
			VALUES('delete', old.id, old.subject, old.content);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_au AFTER UPDATE ON memory BEGIN
			INSERT INTO memory_fts(memory_fts, rowid, subject, content)
			VALUES('delete', old.id, old.subject, old.content);
			INSERT INTO memory_fts(rowid, subject, content)
			VALUES (new.id, new.subject, new.content);
		END;`,
	}
	for _, q := range stmts {
		if _, err := b.db.Exec(q); err != nil {
			return fmt.Errorf("memory: migrate: %w", err)
		}
	}
	var hasScope int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('memory') WHERE name = 'scope'`).Scan(&hasScope); err != nil {
		return fmt.Errorf("memory: migrate: %w", err)
	}
	if hasScope == 0 {
		if _, err := b.db.Exec(`ALTER TABLE memory ADD COLUMN scope TEXT NOT NULL DEFAULT 'user'`); err != nil {
			return fmt.Errorf("memory: migrate scope: %w", err)
		}
	}
	return nil
}

// Close closes the DB when Open owned it.
func (b *Builtin) Close() error {
	if b == nil || b.db == nil || !b.owned {
		return nil
	}
	return b.db.Close()
}

// Move retags every row stored under the repo id from to this repo, for a
// remote rename that orphaned them. A moved row whose kind+subject is already
// live here is superseded by the newer of the two, as a Store would.
func (b *Builtin) Move(ctx context.Context, from string) (int, error) {
	from = strings.TrimSpace(from)
	switch {
	case b.Repo == "":
		return 0, fmt.Errorf("memory: no repo id to move rows into")
	case from == "" || from == ScopeUser:
		return 0, fmt.Errorf("memory: move needs the old repo id (see /memstats)")
	case from == b.Repo:
		return 0, fmt.Errorf("memory: %q is already this repo", from)
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("memory: move: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE memory SET scope = ? WHERE scope = ?`, b.Repo, from)
	if err != nil {
		return 0, fmt.Errorf("memory: move: %w", err)
	}
	n, _ := res.RowsAffected()
	if _, err := tx.ExecContext(ctx, `
		WITH live AS (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY kind, subject ORDER BY updated_at DESC, id DESC) AS rn,
			       FIRST_VALUE(id) OVER (PARTITION BY kind, subject ORDER BY updated_at DESC, id DESC) AS newest
			FROM memory
			WHERE scope = ? AND kind != ? AND superseded_by IS NULL
		)
		UPDATE memory SET superseded_by = (SELECT newest FROM live WHERE live.id = memory.id)
		WHERE id IN (SELECT id FROM live WHERE rn > 1)`, b.Repo, KindEpisode); err != nil {
		return 0, fmt.Errorf("memory: move supersede: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("memory: move: %w", err)
	}
	return int(n), nil
}

// StatsSnapshot is the /memstats view of the builtin store.
type StatsSnapshot struct {
	Total      int
	ByKind     map[string]int
	ByScope    map[string]int
	Active     int
	Expired    int
	Superseded int
	DBBytes    int64
}

// Stats returns row counts and an estimated DB size.
func (b *Builtin) Stats(ctx context.Context) (StatsSnapshot, error) {
	if b == nil || b.db == nil {
		return StatsSnapshot{}, fmt.Errorf("memory: nil store")
	}
	snap := StatsSnapshot{ByKind: make(map[string]int), ByScope: make(map[string]int)}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	rows, err := b.db.QueryContext(ctx, `SELECT kind, COUNT(*) FROM memory GROUP BY kind`)
	if err != nil {
		return StatsSnapshot{}, fmt.Errorf("memory: stats kind: %w", err)
	}
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			_ = rows.Close()
			return StatsSnapshot{}, err
		}
		snap.ByKind[kind] = n
		snap.Total += n
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return StatsSnapshot{}, err
	}
	_ = rows.Close()

	scopes, err := b.db.QueryContext(ctx, `SELECT scope, COUNT(*) FROM memory GROUP BY scope`)
	if err != nil {
		return StatsSnapshot{}, fmt.Errorf("memory: stats scope: %w", err)
	}
	for scopes.Next() {
		var scope string
		var n int
		if err := scopes.Scan(&scope, &n); err != nil {
			_ = scopes.Close()
			return StatsSnapshot{}, err
		}
		snap.ByScope[scope] = n
	}
	if err := scopes.Err(); err != nil {
		_ = scopes.Close()
		return StatsSnapshot{}, err
	}
	_ = scopes.Close()

	if err := b.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN superseded_by IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN superseded_by IS NULL
				AND expires_at IS NOT NULL AND expires_at <= ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN superseded_by IS NULL
				AND (expires_at IS NULL OR expires_at > ?) THEN 1 ELSE 0 END), 0)
		FROM memory`, now, now).Scan(&snap.Superseded, &snap.Expired, &snap.Active); err != nil {
		return StatsSnapshot{}, fmt.Errorf("memory: stats state: %w", err)
	}

	var pageCount, pageSize int64
	if err := b.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount); err != nil {
		return StatsSnapshot{}, fmt.Errorf("memory: page_count: %w", err)
	}
	if err := b.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return StatsSnapshot{}, fmt.Errorf("memory: page_size: %w", err)
	}
	snap.DBBytes = pageCount * pageSize
	return snap, nil
}

// Store inserts one atomic memory row. Durable kinds (not episode) with the
// same kind+subject supersede the previous live row so corrections replace
// without a duplicate (sushi dislike → like). History stays in sqlite.
func (b *Builtin) Store(ctx context.Context, kind, subject, content string) (Entry, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if err := ValidateKind(kind); err != nil {
		return Entry{}, err
	}
	subject = strings.TrimSpace(subject)
	content = strings.TrimSpace(content)
	if subject == "" || content == "" {
		return Entry{}, fmt.Errorf("memory: subject and content are required")
	}

	var old Entry
	var hadOld bool
	if kind != KindEpisode {
		prev, ok, err := b.ActiveByKindSubject(ctx, kind, subject)
		if err != nil {
			return Entry{}, err
		}
		old, hadOld = prev, ok
	}

	now := time.Now().UTC()
	var expires any
	if kind == KindEpisode {
		exp := now.Add(episodeTTL)
		expires = exp.Format(time.RFC3339Nano)
	}

	res, err := b.db.ExecContext(ctx, `
		INSERT INTO memory (kind, subject, content, source, confidence, created_at, updated_at, expires_at, consolidated, scope)
		VALUES (?, ?, ?, ?, 1.0, ?, ?, ?, 0, ?)`,
		kind, subject, content, SourceChat,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), expires, b.scopeFor(kind),
	)
	if err != nil {
		return Entry{}, fmt.Errorf("memory: store: %w", err)
	}
	id, _ := res.LastInsertId()
	e, err := b.get(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	if hadOld && old.ID != e.ID {
		if err := b.Supersede(ctx, old.ID, e.ID); err != nil {
			return Entry{}, fmt.Errorf("memory: supersede: %w", err)
		}
	}
	return e, nil
}

// Get loads a row by id (including superseded / expired).
func (b *Builtin) Get(ctx context.Context, id int64) (Entry, error) {
	e, err := b.get(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return Entry{}, fmt.Errorf("memory: id %d not found", id)
		}
		return Entry{}, err
	}
	return e, nil
}

// ActiveByKindSubject returns the live row for kind+subject, if any.
func (b *Builtin) ActiveByKindSubject(ctx context.Context, kind, subject string) (Entry, bool, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	subject = strings.TrimSpace(subject)
	if kind == "" || subject == "" {
		return Entry{}, false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	row := b.db.QueryRowContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory
		WHERE kind = ? AND subject = ? AND superseded_by IS NULL AND scope = ?
		  AND (expires_at IS NULL OR expires_at > ?)
		ORDER BY updated_at DESC
		LIMIT 1`, kind, subject, b.scopeFor(kind), now)
	e, err := scanEntry(row)
	if err == sql.ErrNoRows {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	return e, true, nil
}

// Recall runs FTS5 + recency ranking.
func (b *Builtin) Recall(ctx context.Context, query string, limit int) ([]Entry, error) {
	if limit < 1 {
		limit = defaultRecallLimit
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return b.listActive(ctx, limit)
	}
	return b.search(ctx, query, limit)
}

// Forget hard-deletes a row by id.
func (b *Builtin) Forget(ctx context.Context, id int64) error {
	res, err := b.db.ExecContext(ctx, `DELETE FROM memory WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("memory: forget: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: id %d not found", id)
	}
	return nil
}

// ForgetQuery deletes FTS matches for query (correctability).
func (b *Builtin) ForgetQuery(ctx context.Context, query string) (int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, fmt.Errorf("memory: forget query is required")
	}
	entries, err := b.search(ctx, query, 100)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if err := b.Forget(ctx, e.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Hydrate returns active durable rows plus FTS hits for query (cap ~30).
func (b *Builtin) Hydrate(ctx context.Context, query string, limit int) ([]Entry, error) {
	if limit < 1 {
		limit = defaultHydrateLimit
	}
	seen := map[int64]struct{}{}
	var out []Entry

	add := func(list []Entry) {
		for _, e := range list {
			if _, ok := seen[e.ID]; ok {
				continue
			}
			seen[e.ID] = struct{}{}
			out = append(out, e)
			if len(out) >= limit {
				return
			}
		}
	}

	// Reserve ~1/3 of slots for query-relevant hits so durable rows cannot starve FTS.
	durableCap := limit
	queryCap := 0
	if q := strings.TrimSpace(query); q != "" {
		queryCap = limit / 3
		if queryCap < 1 {
			queryCap = 1
		}
		durableCap = limit - queryCap
		if durableCap < 1 {
			durableCap = 1
			queryCap = limit - 1
		}
	}
	durable, err := b.listDurable(ctx, durableCap)
	if err != nil {
		return nil, err
	}
	add(durable)
	if queryCap > 0 {
		hits, err := b.search(ctx, strings.TrimSpace(query), queryCap+len(out))
		if err != nil {
			return nil, err
		}
		add(hits)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Supersede links oldID → newID without deleting.
func (b *Builtin) Supersede(ctx context.Context, oldID, newID int64) error {
	_, err := b.db.ExecContext(ctx, `
		UPDATE memory SET superseded_by = ?, updated_at = ? WHERE id = ?`,
		newID, time.Now().UTC().Format(time.RFC3339Nano), oldID)
	return err
}

func (b *Builtin) listActive(ctx context.Context, limit int) ([]Entry, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory
		WHERE superseded_by IS NULL
		  AND scope IN (?, ?)
		  AND NOT (kind = ? AND consolidated != 0)
		  AND (expires_at IS NULL OR expires_at > ?)
		ORDER BY updated_at DESC
		LIMIT ?`, ScopeUser, b.readScope(), KindEpisode, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (b *Builtin) listDurable(ctx context.Context, limit int) ([]Entry, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory
		WHERE superseded_by IS NULL
		  AND scope IN (?, ?)
		  AND kind IN (?, ?, ?, ?)
		  AND (expires_at IS NULL OR expires_at > ?)
		ORDER BY
			CASE kind
				WHEN 'preference' THEN 0
				WHEN 'person' THEN 1
				WHEN 'fact' THEN 2
				ELSE 3
			END,
			updated_at DESC
		LIMIT ?`, ScopeUser, b.readScope(), KindPreference, KindPerson, KindFact, KindInsight, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (b *Builtin) search(ctx context.Context, query string, limit int) ([]Entry, error) {
	fts := ftsQuery(query)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := b.db.QueryContext(ctx, `
		SELECT m.id, m.kind, m.subject, m.content, m.source, m.confidence,
		       m.created_at, m.updated_at, m.expires_at, m.superseded_by
		FROM memory_fts
		JOIN memory m ON m.id = memory_fts.rowid
		WHERE memory_fts MATCH ?
		  AND m.superseded_by IS NULL
		  AND m.scope IN (?, ?)
		  AND NOT (m.kind = ? AND m.consolidated != 0)
		  AND (m.expires_at IS NULL OR m.expires_at > ?)
		ORDER BY rank, m.updated_at DESC
		LIMIT ?`, fts, ScopeUser, b.readScope(), KindEpisode, now, limit)
	if err != nil {
		return b.searchLike(ctx, query, now, limit)
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (b *Builtin) searchLike(ctx context.Context, query, now string, limit int) ([]Entry, error) {
	like := "%" + query + "%"
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory
		WHERE superseded_by IS NULL
		  AND scope IN (?, ?)
		  AND NOT (kind = ? AND consolidated != 0)
		  AND (expires_at IS NULL OR expires_at > ?)
		  AND (subject LIKE ? OR content LIKE ?)
		ORDER BY updated_at DESC
		LIMIT ?`, ScopeUser, b.readScope(), KindEpisode, now, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: search: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (b *Builtin) get(ctx context.Context, id int64) (Entry, error) {
	row := b.db.QueryRowContext(ctx, `
		SELECT id, kind, subject, content, source, confidence, created_at, updated_at, expires_at, superseded_by
		FROM memory WHERE id = ?`, id)
	return scanEntry(row)
}

func ftsQuery(q string) string {
	parts := strings.Fields(q)
	if len(parts) == 0 {
		return `""`
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ReplaceAll(p, `"`, "")
		if p == "" {
			continue
		}
		out = append(out, `"`+p+`"`)
	}
	return strings.Join(out, " ")
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(row rowScanner) (Entry, error) {
	var e Entry
	var created, updated string
	var expires sql.NullString
	var superseded sql.NullInt64
	if err := row.Scan(&e.ID, &e.Kind, &e.Subject, &e.Content, &e.Source, &e.Confidence,
		&created, &updated, &expires, &superseded); err != nil {
		return Entry{}, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if expires.Valid {
		t, err := time.Parse(time.RFC3339Nano, expires.String)
		if err == nil {
			e.ExpiresAt = &t
		}
	}
	if superseded.Valid {
		v := superseded.Int64
		e.SupersededBy = &v
	}
	return e, nil
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
