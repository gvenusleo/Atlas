// Package session provides SQLite storage for Atlas local sessions.
package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/transcript"
	_ "modernc.org/sqlite"
)

const defaultDBFileName = "atlas.db"

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Store reads and writes the Atlas local session database.
type Store struct {
	db *sql.DB
}

// Session describes the metadata for a local session.
type Session struct {
	ID    string
	Title string
	CWD   string
	// AdditionalDirectories holds the ACP session-level additional working directory roots.
	AdditionalDirectories []string
	ContextSummary        string
	CompactedMessageCount int
	CompactedInputTokens  int
	LastInputTokens       int
	LastOutputTokens      int
	LastTotalTokens       int
	// MemoryExtractedMessageCount records the message boundary processed by the long-term memory background task.
	MemoryExtractedMessageCount int
	MemoryExtractedInputTokens  int
	MemoryExtractedHash         string
	MemoryExtractedAt           time.Time
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

// ListPage describes a single page of session list results.
type ListPage struct {
	Sessions   []Session
	NextCursor string
}

// SaveTranscriptOptions describes the session metadata overrides applied when saving a transcript.
type SaveTranscriptOptions struct {
	// AdditionalDirectories is the additional working directory roots to write in this save.
	AdditionalDirectories []string
	// AdditionalDirectoriesSet indicates the caller explicitly provided additional working directory roots.
	AdditionalDirectoriesSet bool
}

// DefaultPath returns the default session database path under the user's home directory.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".atlas", defaultDBFileName), nil
}

// Open opens the SQLite session database. The caller owns the connection lifecycle.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("session db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// OpenDB creates a session Store from an existing *sql.DB. The caller owns the DB lifecycle;
// do not call Store.Close when using this constructor.
func OpenDB(db *sql.DB) *Store {
	return &Store{db: db}
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// EnsureSchema creates the initial session table schema.
func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
create table if not exists sessions (
	id text primary key,
	title text not null default '',
	cwd text not null,
	additional_directories_json text not null default '',
	context_summary text not null default '',
	compacted_message_count integer not null default 0,
	compacted_input_tokens integer not null default 0,
	last_input_tokens integer not null default 0,
	last_output_tokens integer not null default 0,
	last_total_tokens integer not null default 0,
	memory_extracted_message_count integer not null default 0,
	memory_extracted_input_tokens integer not null default 0,
	memory_extracted_hash text not null default '',
	memory_extracted_at text not null default '',
	created_at text not null,
	updated_at text not null
);

create table if not exists messages (
	id integer primary key autoincrement,
	session_id text not null,
	role text not null,
	content text not null,
	content_parts_json text not null default '',
	reasoning_content text not null default '',
	tool_call_id text not null default '',
	tool_calls_json text not null default '',
	tool_metadata_json text not null default '',
	provider_items_json text not null default '',
	input_tokens integer not null default 0,
	output_tokens integer not null default 0,
	total_tokens integer not null default 0,
	created_at text not null,
	foreign key(session_id) references sessions(id) on delete cascade
);`)
	return err
}

// NewID generates a session ID suitable for CLI display and restoration.
func NewID(now time.Time) (string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return now.Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:]), nil
}

// ValidateID validates a user-provided or auto-generated session ID.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("session id is required")
	}
	if !sessionIDPattern.MatchString(id) {
		return fmt.Errorf("session id %q contains invalid characters", id)
	}
	return nil
}

// LoadTranscript reads the transcript for the specified session. Returns an empty transcript if not found.
func (s *Store) LoadTranscript(ctx context.Context, sessionID string) (*transcript.Transcript, error) {
	if err := ValidateID(sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
select role, content, content_parts_json, reasoning_content, tool_call_id, tool_calls_json, tool_metadata_json, provider_items_json, input_tokens, output_tokens, total_tokens
from messages
where session_id = ?
order by id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trans := transcript.New()
	for rows.Next() {
		var role, content, contentPartsJSON, reasoningContent, toolCallID, toolCallsJSON, toolMetadataJSON, providerItemsJSON string
		var usage model.Usage
		if err := rows.Scan(&role, &content, &contentPartsJSON, &reasoningContent, &toolCallID, &toolCallsJSON, &toolMetadataJSON, &providerItemsJSON, &usage.InputTokens, &usage.OutputTokens, &usage.TotalTokens); err != nil {
			return nil, err
		}
		parts, err := decodeContentParts(contentPartsJSON)
		if err != nil {
			return nil, err
		}
		toolCalls, err := decodeToolCalls(toolCallsJSON)
		if err != nil {
			return nil, err
		}
		toolMetadata, err := decodeToolMetadata(toolMetadataJSON)
		if err != nil {
			return nil, err
		}
		providerItems, err := decodeProviderItems(providerItemsJSON)
		if err != nil {
			return nil, err
		}
		trans.Append(model.Message{
			Role:             model.Role(role),
			Content:          content,
			Parts:            parts,
			ReasoningContent: reasoningContent,
			ToolCallID:       toolCallID,
			ToolCalls:        toolCalls,
			ToolMetadata:     toolMetadata,
			Usage:            usage,
			ProviderItems:    providerItems,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return trans, nil
}

// ListSessions returns sessions ordered by most recent update time.
func (s *Store) ListSessions(ctx context.Context, limit int) ([]Session, error) {
	page, err := s.ListSessionsPage(ctx, "", limit)
	return page.Sessions, err
}

// ListSessionsPage returns a paginated list of sessions, ordered by most recent update time.
func (s *Store) ListSessionsPage(ctx context.Context, cursor string, limit int) (ListPage, error) {
	return s.listSessionsPage(ctx, "", cursor, limit)
}

// ListSessionsForCWD returns sessions for the specified working directory, ordered by most recent update time.
func (s *Store) ListSessionsForCWD(ctx context.Context, cwd string, limit int) ([]Session, error) {
	page, err := s.ListSessionsForCWDPage(ctx, cwd, "", limit)
	return page.Sessions, err
}

// ListSessionsForCWDPage returns a paginated list of sessions for the specified working directory, ordered by most recent update time.
func (s *Store) ListSessionsForCWDPage(ctx context.Context, cwd, cursor string, limit int) (ListPage, error) {
	return s.listSessionsPage(ctx, cwd, cursor, limit)
}

// listSessionsPage executes the shared cursor pagination query for session lists.
func (s *Store) listSessionsPage(ctx context.Context, cwd, cursor string, limit int) (ListPage, error) {
	if limit <= 0 {
		limit = 20
	}
	queryLimit := limit + 1
	var args []any
	var conditions []string
	if cwd != "" {
		conditions = append(conditions, "cwd = ?")
		args = append(args, cwd)
	}
	if cursor != "" {
		updatedAt, id, err := decodeSessionCursor(cursor)
		if err != nil {
			return ListPage{}, err
		}
		conditions = append(conditions, "(updated_at < ? or (updated_at = ? and id > ?))")
		updatedAtText := updatedAt.UTC().Format(time.RFC3339Nano)
		args = append(args, updatedAtText, updatedAtText, id)
	}
	where := ""
	if len(conditions) > 0 {
		where = "where " + strings.Join(conditions, " and ")
	}
	args = append(args, queryLimit)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
select id, title, cwd, additional_directories_json, context_summary, compacted_message_count, compacted_input_tokens, last_input_tokens, last_output_tokens, last_total_tokens, memory_extracted_message_count, memory_extracted_input_tokens, memory_extracted_hash, memory_extracted_at, created_at, updated_at
from sessions
%s
order by updated_at desc, id
limit ?`, where), args...)
	if err != nil {
		return ListPage{}, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return ListPage{}, err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, err
	}
	page := ListPage{Sessions: sessions}
	if len(page.Sessions) > limit {
		page.Sessions = page.Sessions[:limit]
		last := page.Sessions[len(page.Sessions)-1]
		page.NextCursor = encodeSessionCursor(last.UpdatedAt, last.ID)
	}
	return page, nil
}

// GetSession returns the metadata for the specified session.
func (s *Store) GetSession(ctx context.Context, sessionID string) (Session, error) {
	if err := ValidateID(sessionID); err != nil {
		return Session{}, err
	}
	row := s.db.QueryRowContext(ctx, `
select id, title, cwd, additional_directories_json, context_summary, compacted_message_count, compacted_input_tokens, last_input_tokens, last_output_tokens, last_total_tokens, memory_extracted_message_count, memory_extracted_input_tokens, memory_extracted_hash, memory_extracted_at, created_at, updated_at
from sessions
where id = ?`, sessionID)
	session, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, fmt.Errorf("session %q not found", sessionID)
	}
	return session, err
}

// DeleteSession deletes the specified session and its messages.
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	if err := ValidateID(sessionID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `delete from sessions where id = ?`, sessionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	if _, err := tx.ExecContext(ctx, `delete from messages where session_id = ?`, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveCompaction saves the context compaction summary and boundary for the specified session.
func (s *Store) SaveCompaction(ctx context.Context, sessionID string, summary string, compactedMessageCount int, compactedInputTokens int) error {
	if err := ValidateID(sessionID); err != nil {
		return err
	}
	if compactedMessageCount < 0 {
		return fmt.Errorf("compacted message count must be non-negative")
	}
	if compactedInputTokens < 0 {
		return fmt.Errorf("compacted input tokens must be non-negative")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
update sessions
set context_summary = ?,
	compacted_message_count = ?,
	compacted_input_tokens = ?,
	updated_at = ?
where id = ?`, summary, compactedMessageCount, compactedInputTokens, now, sessionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// SaveMemoryExtraction saves the processing boundary for long-term memory background extraction.
func (s *Store) SaveMemoryExtraction(ctx context.Context, sessionID string, messageCount int, inputTokens int, inputHash string) error {
	if err := ValidateID(sessionID); err != nil {
		return err
	}
	if messageCount < 0 {
		return fmt.Errorf("memory extracted message count must be non-negative")
	}
	if inputTokens < 0 {
		return fmt.Errorf("memory extracted input tokens must be non-negative")
	}
	inputHash = strings.TrimSpace(inputHash)
	if inputHash == "" {
		return fmt.Errorf("memory extracted hash is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
update sessions
set memory_extracted_message_count = ?,
	memory_extracted_input_tokens = ?,
	memory_extracted_hash = ?,
	memory_extracted_at = ?
where id = ?
	and memory_extracted_message_count <= ?`, messageCount, inputTokens, inputHash, now, sessionID, messageCount)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 0 {
		return nil
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `select count(*) from sessions where id = ?`, sessionID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// SaveSessionRoots saves the additional working directory roots for the specified session.
func (s *Store) SaveSessionRoots(ctx context.Context, sessionID string, additionalDirectories []string) error {
	if err := ValidateID(sessionID); err != nil {
		return err
	}
	additionalDirectoriesJSON, err := encodeAdditionalDirectories(additionalDirectories)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
update sessions
set additional_directories_json = ?
where id = ?`, additionalDirectoriesJSON, sessionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// SaveTranscript overwrites the specified session with the given message snapshot.
func (s *Store) SaveTranscript(ctx context.Context, sessionID, cwd string, messages []model.Message) error {
	return s.SaveTranscriptWithOptions(ctx, sessionID, cwd, messages, SaveTranscriptOptions{})
}

// SaveTranscriptWithOptions overwrites the specified session with the given message snapshot and metadata.
func (s *Store) SaveTranscriptWithOptions(ctx context.Context, sessionID, cwd string, messages []model.Message, opts SaveTranscriptOptions) error {
	if err := ValidateID(sessionID); err != nil {
		return err
	}
	additionalDirectoriesSQL := "coalesce((select additional_directories_json from sessions where id = ?), '')"
	additionalDirectoriesArgs := []any{sessionID}
	if opts.AdditionalDirectoriesSet {
		additionalDirectoriesJSON, err := encodeAdditionalDirectories(opts.AdditionalDirectories)
		if err != nil {
			return err
		}
		additionalDirectoriesSQL = "?"
		additionalDirectoriesArgs = []any{additionalDirectoriesJSON}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	title := titleFromMessages(messages)
	usage := lastUsageFromMessages(messages)
	args := []any{sessionID, title, cwd}
	args = append(args, additionalDirectoriesArgs...)
	args = append(args, usage.InputTokens, usage.OutputTokens, usage.TotalTokens, now, now)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
insert into sessions(id, title, cwd, additional_directories_json, last_input_tokens, last_output_tokens, last_total_tokens, created_at, updated_at)
values(?, ?, ?, %s, ?, ?, ?, ?, ?)
on conflict(id) do update set
	title = excluded.title,
	cwd = excluded.cwd,
	additional_directories_json = excluded.additional_directories_json,
	last_input_tokens = excluded.last_input_tokens,
	last_output_tokens = excluded.last_output_tokens,
	last_total_tokens = excluded.last_total_tokens,
	updated_at = excluded.updated_at`, additionalDirectoriesSQL), args...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from messages where session_id = ?`, sessionID); err != nil {
		return err
	}
	if err := insertMessages(ctx, tx, sessionID, messages, now); err != nil {
		return err
	}
	return tx.Commit()
}

// AppendMessagesWithOptions atomically appends messages and refreshes session metadata without rewriting history.
func (s *Store) AppendMessagesWithOptions(ctx context.Context, sessionID, cwd string, messages []model.Message, opts SaveTranscriptOptions) error {
	if err := ValidateID(sessionID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	additionalDirectoriesSQL := "coalesce((select additional_directories_json from sessions where id = ?), '')"
	additionalDirectoriesArgs := []any{sessionID}
	if opts.AdditionalDirectoriesSet {
		additionalDirectoriesJSON, err := encodeAdditionalDirectories(opts.AdditionalDirectories)
		if err != nil {
			return err
		}
		additionalDirectoriesSQL = "?"
		additionalDirectoriesArgs = []any{additionalDirectoriesJSON}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	title := titleFromMessages(messages)
	usage := lastUsageFromMessages(messages)
	args := []any{sessionID, title, cwd}
	args = append(args, additionalDirectoriesArgs...)
	args = append(args, usage.InputTokens, usage.OutputTokens, usage.TotalTokens, now, now)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
insert into sessions(id, title, cwd, additional_directories_json, last_input_tokens, last_output_tokens, last_total_tokens, created_at, updated_at)
values(?, ?, ?, %s, ?, ?, ?, ?, ?)
on conflict(id) do update set
	title = case when sessions.title = '' then excluded.title else sessions.title end,
	cwd = excluded.cwd,
	additional_directories_json = excluded.additional_directories_json,
	last_input_tokens = case when excluded.last_input_tokens != 0 or excluded.last_output_tokens != 0 or excluded.last_total_tokens != 0 then excluded.last_input_tokens else sessions.last_input_tokens end,
	last_output_tokens = case when excluded.last_input_tokens != 0 or excluded.last_output_tokens != 0 or excluded.last_total_tokens != 0 then excluded.last_output_tokens else sessions.last_output_tokens end,
	last_total_tokens = case when excluded.last_input_tokens != 0 or excluded.last_output_tokens != 0 or excluded.last_total_tokens != 0 then excluded.last_total_tokens else sessions.last_total_tokens end,
	updated_at = excluded.updated_at`, additionalDirectoriesSQL), args...); err != nil {
		return err
	}
	if err := insertMessages(ctx, tx, sessionID, messages, now); err != nil {
		return err
	}
	return tx.Commit()
}

func insertMessages(ctx context.Context, tx *sql.Tx, sessionID string, messages []model.Message, now string) error {
	for _, msg := range messages {
		toolCallsJSON, err := encodeToolCalls(msg.ToolCalls)
		if err != nil {
			return err
		}
		toolMetadataJSON, err := encodeToolMetadata(msg.ToolMetadata)
		if err != nil {
			return err
		}
		providerItemsJSON, err := encodeProviderItems(msg.ProviderItems)
		if err != nil {
			return err
		}
		contentPartsJSON, err := encodeContentParts(model.MessageParts(msg))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
insert into messages(session_id, role, content, content_parts_json, reasoning_content, tool_call_id, tool_calls_json, tool_metadata_json, provider_items_json, input_tokens, output_tokens, total_tokens, created_at)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, sessionID, string(msg.Role), msg.Content, contentPartsJSON, msg.ReasoningContent, msg.ToolCallID, toolCallsJSON, toolMetadataJSON, providerItemsJSON, msg.Usage.InputTokens, msg.Usage.OutputTokens, msg.Usage.TotalTokens, now); err != nil {
			return err
		}
	}
	return nil
}

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSession(scanner sessionScanner) (Session, error) {
	var session Session
	var createdAt, updatedAt, memoryExtractedAt string
	var additionalDirectoriesJSON string
	if err := scanner.Scan(&session.ID, &session.Title, &session.CWD, &additionalDirectoriesJSON, &session.ContextSummary, &session.CompactedMessageCount, &session.CompactedInputTokens, &session.LastInputTokens, &session.LastOutputTokens, &session.LastTotalTokens, &session.MemoryExtractedMessageCount, &session.MemoryExtractedInputTokens, &session.MemoryExtractedHash, &memoryExtractedAt, &createdAt, &updatedAt); err != nil {
		return Session{}, err
	}
	additionalDirectories, err := decodeAdditionalDirectories(additionalDirectoriesJSON)
	if err != nil {
		return Session{}, err
	}
	session.AdditionalDirectories = additionalDirectories
	session.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Session{}, err
	}
	session.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Session{}, err
	}
	if memoryExtractedAt != "" {
		session.MemoryExtractedAt, err = time.Parse(time.RFC3339Nano, memoryExtractedAt)
		if err != nil {
			return Session{}, err
		}
	}
	return session, nil
}

func encodeContentParts(parts []model.ContentPart) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}
	content, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func decodeContentParts(content string) ([]model.ContentPart, error) {
	if content == "" {
		return nil, nil
	}
	var parts []model.ContentPart
	if err := json.Unmarshal([]byte(content), &parts); err != nil {
		return nil, err
	}
	return parts, nil
}

func encodeToolCalls(calls []model.ToolCall) (string, error) {
	if len(calls) == 0 {
		return "", nil
	}
	content, err := json.Marshal(calls)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func decodeToolCalls(content string) ([]model.ToolCall, error) {
	if content == "" {
		return nil, nil
	}
	var calls []model.ToolCall
	if err := json.Unmarshal([]byte(content), &calls); err != nil {
		return nil, err
	}
	return calls, nil
}

func encodeToolMetadata(metadata model.ToolMetadata) (string, error) {
	if len(metadata.Locations) == 0 && len(metadata.Diffs) == 0 && metadata.Diff == nil && len(metadata.Todos) == 0 {
		return "", nil
	}
	content, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func decodeToolMetadata(content string) (model.ToolMetadata, error) {
	if content == "" {
		return model.ToolMetadata{}, nil
	}
	var metadata model.ToolMetadata
	if err := json.Unmarshal([]byte(content), &metadata); err != nil {
		return model.ToolMetadata{}, err
	}
	return metadata, nil
}

func encodeProviderItems(items []model.ProviderItem) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	content, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func decodeProviderItems(content string) ([]model.ProviderItem, error) {
	if content == "" {
		return nil, nil
	}
	var items []model.ProviderItem
	if err := json.Unmarshal([]byte(content), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// encodeAdditionalDirectories serializes additional working directory roots to JSON.
func encodeAdditionalDirectories(additionalDirectories []string) (string, error) {
	if len(additionalDirectories) == 0 {
		return "", nil
	}
	content, err := json.Marshal(additionalDirectories)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// decodeAdditionalDirectories parses the additional working directory roots JSON.
func decodeAdditionalDirectories(content string) ([]string, error) {
	if content == "" {
		return nil, nil
	}
	var additionalDirectories []string
	if err := json.Unmarshal([]byte(content), &additionalDirectories); err != nil {
		return nil, err
	}
	return additionalDirectories, nil
}

// encodeSessionCursor encodes a pagination cursor as an opaque string.
func encodeSessionCursor(updatedAt time.Time, id string) string {
	if updatedAt.IsZero() || id == "" {
		return ""
	}
	content := fmt.Sprintf("%s\x00%s", updatedAt.UTC().Format(time.RFC3339Nano), id)
	return base64.StdEncoding.EncodeToString([]byte(content))
}

// decodeSessionCursor decodes a pagination cursor.
func decodeSessionCursor(cursor string) (time.Time, string, error) {
	if cursor == "" {
		return time.Time{}, "", fmt.Errorf("session cursor is required")
	}
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(data), "\x00", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return time.Time{}, "", fmt.Errorf("invalid session cursor")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return updatedAt, parts[1], nil
}

func titleFromMessages(messages []model.Message) string {
	for _, msg := range messages {
		if msg.Role != model.RoleUser {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			content = strings.TrimSpace(model.TextFromParts(model.MessageParts(msg)))
		}
		if content != "" {
			return firstLine(content)
		}
	}
	return ""
}

func lastUsageFromMessages(messages []model.Message) model.Usage {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == model.RoleAssistant && (msg.Usage.InputTokens != 0 || msg.Usage.OutputTokens != 0 || msg.Usage.TotalTokens != 0) {
			return msg.Usage
		}
	}
	return model.Usage{}
}

func firstLine(content string) string {
	content = strings.TrimSpace(content)
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		content = content[:idx]
	}
	runes := []rune(content)
	if len(runes) > 80 {
		content = string(runes[:80])
	}
	return content
}
