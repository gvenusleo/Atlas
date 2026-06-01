package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore persists Atlas sessions in a local SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// OpenSQLite opens or creates an Atlas database at path.
func OpenSQLite(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  workdir TEXT NOT NULL,
  model TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  tool_call_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_session_id_id ON messages(session_id, id);
`
	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

// CreateSession inserts a new session row.
func (s *SQLiteStore) CreateSession(session Session) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, title, workdir, model, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.Title,
		session.Workdir,
		session.Model,
		formatTime(session.CreatedAt),
		formatTime(session.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSession returns one session by ID.
func (s *SQLiteStore) GetSession(id string) (Session, error) {
	var row sessionRow
	err := s.db.QueryRow(
		`SELECT id, title, workdir, model, created_at, updated_at FROM sessions WHERE id = ?`,
		id,
	).Scan(&row.ID, &row.Title, &row.Workdir, &row.Model, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, fmt.Errorf("session %q not found", id)
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	return row.toSession()
}

// ListSessions returns sessions ordered by most recent update.
func (s *SQLiteStore) ListSessions() ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, title, workdir, model, created_at, updated_at
		 FROM sessions ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var row sessionRow
		if err := rows.Scan(&row.ID, &row.Title, &row.Workdir, &row.Model, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		session, err := row.toSession()
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return sessions, nil
}

// AddMessage persists one message and bumps the session update timestamp.
func (s *SQLiteStore) AddMessage(message Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin add message: %w", err)
	}
	defer tx.Rollback()

	created := message.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	if _, err := tx.Exec(
		`INSERT INTO messages (session_id, role, content, tool_call_id, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		message.SessionID,
		message.Role,
		message.Content,
		message.ToolCallID,
		formatTime(created),
	); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		formatTime(created),
		message.SessionID,
	); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add message: %w", err)
	}
	return nil
}

// Messages returns session messages in chronological order.
func (s *SQLiteStore) Messages(sessionID string, limit int) ([]Message, error) {
	query := `SELECT id, session_id, role, content, tool_call_id, created_at
	          FROM messages WHERE session_id = ? ORDER BY id ASC`
	args := []any{sessionID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var row messageRow
		if err := rows.Scan(&row.ID, &row.SessionID, &row.Role, &row.Content, &row.ToolCallID, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		message, err := row.toMessage()
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return messages, nil
}

// Close releases the database handle.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

type sessionRow struct {
	ID        string
	Title     string
	Workdir   string
	Model     string
	CreatedAt string
	UpdatedAt string
}

func (r sessionRow) toSession() (Session, error) {
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return Session{}, err
	}
	updated, err := parseTime(r.UpdatedAt)
	if err != nil {
		return Session{}, err
	}
	return Session{
		ID:        r.ID,
		Title:     r.Title,
		Workdir:   r.Workdir,
		Model:     r.Model,
		CreatedAt: created,
		UpdatedAt: updated,
	}, nil
}

type messageRow struct {
	ID         int64
	SessionID  string
	Role       string
	Content    string
	ToolCallID string
	CreatedAt  string
}

func (r messageRow) toMessage() (Message, error) {
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return Message{}, err
	}
	return Message{
		ID:         r.ID,
		SessionID:  r.SessionID,
		Role:       r.Role,
		Content:    r.Content,
		ToolCallID: r.ToolCallID,
		CreatedAt:  created,
	}, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", value, err)
	}
	return t, nil
}
