// Package memory manages Atlas's long-term memory entries and background tasks.
package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/model"
	_ "modernc.org/sqlite"
)

const (
	// ScopeGlobal indicates user preferences and long-term instructions that apply across projects.
	ScopeGlobal = "global"
	// ScopeProject indicates facts and workflows that apply only to the current project.
	ScopeProject = "project"

	// TypeInstruction indicates a user long-term preference or constraint.
	TypeInstruction = "instruction"
	// TypeFact indicates a project fact.
	TypeFact = "fact"
	// TypeWorkflow indicates a reusable project workflow.
	TypeWorkflow = "workflow"

	statusActive    = "active"
	statusArchived  = "archived"
	statusPending   = "pending"
	statusRunning   = "running"
	statusSucceeded = "succeeded"
	statusFailed    = "failed"

	defaultPromptEntryLimit = 12
)

// Store reads and writes the Atlas long-term memory database.
type Store struct {
	db *sql.DB
}

// Entry is a single long-term memory entry.
type Entry struct {
	ID              int64
	Scope           string
	ProjectKey      string
	ProjectPath     string
	Type            string
	Content         string
	SourceNote      string
	Confidence      int
	Fingerprint     string
	Status          string
	SourceSessionID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastUsedAt      time.Time
	UseCount        int
}

// Job describes a single background memory job.
type Job struct {
	Key       string
	SessionID string
	Model     string
	Status    string
	Attempts  int
	WorkerID  string
	LastError string
}

// Counts summarizes background memory job counts.
type Counts struct {
	Entries int
	Pending int
	Failed  int
}

// Open opens the SQLite memory database. The caller owns the connection lifecycle.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("memory db path is required")
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

// OpenDB creates a memory Store from an existing *sql.DB. The caller owns the DB lifecycle;
// do not call Store.Close when using this constructor.
func OpenDB(db *sql.DB) *Store {
	return &Store{db: db}
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for testing and diagnostics.
func (s *Store) DB() *sql.DB {
	return s.db
}

// EnsureSchema creates the long-term memory table schema.
func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
create table if not exists memory_entries (
	id integer primary key autoincrement,
	scope text not null,
	project_key text not null default '',
	project_path text not null default '',
	type text not null,
	content text not null,
	source_note text not null default '',
	confidence integer not null default 3,
	fingerprint text not null unique,
	status text not null default 'active',
	source_session_id text not null default '',
	created_at text not null,
	updated_at text not null,
	last_used_at text not null default '',
	use_count integer not null default 0
);

create index if not exists memory_entries_lookup_idx
	on memory_entries(status, scope, project_key, type, updated_at);

create table if not exists memory_jobs (
	job_key text primary key,
	session_id text not null default '',
	model text not null default '',
	status text not null,
	attempts integer not null default 0,
	retry_after text not null default '',
	lease_until text not null default '',
	worker_id text not null default '',
	last_error text not null default '',
	created_at text not null,
	updated_at text not null,
	finished_at text not null default ''
);

create index if not exists memory_jobs_ready_idx
	on memory_jobs(status, retry_after, lease_until, updated_at);
`)
	return err
}

// ProjectIdentity returns a stable project identifier for the working directory.
func ProjectIdentity(cwd string) (string, string) {
	if cwd == "" {
		cwd = "."
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	projectPath := filepath.Clean(abs)
	sum := sha256.Sum256([]byte(projectPath))
	return hex.EncodeToString(sum[:])[:16], projectPath
}

// TranscriptHash returns a stable hash of the transcript content.
func TranscriptHash(messages []model.Message) string {
	var builder strings.Builder
	for _, msg := range messages {
		builder.WriteString(string(msg.Role))
		builder.WriteByte('\x00')
		builder.WriteString(msg.Content)
		builder.WriteByte('\x00')
		for _, part := range model.MessageParts(msg) {
			builder.WriteString(string(part.Type))
			builder.WriteByte('\x00')
			builder.WriteString(part.Text)
			builder.WriteByte('\x00')
			builder.WriteString(part.MimeType)
			builder.WriteByte('\x00')
			builder.WriteString(part.DataURL)
			builder.WriteByte('\x00')
			builder.WriteString(part.URI)
			builder.WriteByte('\x00')
			builder.WriteString(string(part.Detail))
			builder.WriteByte('\x00')
		}
		builder.WriteString(msg.ReasoningContent)
		builder.WriteByte('\x00')
		for _, call := range msg.ToolCalls {
			builder.WriteString(call.ID)
			builder.WriteByte('\x00')
			builder.WriteString(call.Name)
			builder.WriteByte('\x00')
			builder.WriteString(call.Arguments)
			builder.WriteByte('\x00')
		}
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

// EntryFingerprint returns the deduplication fingerprint for a memory entry.
func EntryFingerprint(scope, projectKey, entryType, content string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(content), " "))
	sum := sha256.Sum256([]byte(scope + "\x00" + projectKey + "\x00" + entryType + "\x00" + normalized))
	return hex.EncodeToString(sum[:])
}

// UpsertEntry inserts or updates a single active memory entry.
func (s *Store) UpsertEntry(ctx context.Context, entry Entry) (Entry, error) {
	if err := validateScope(entry.Scope); err != nil {
		return Entry{}, err
	}
	if err := validateType(entry.Type); err != nil {
		return Entry{}, err
	}
	entry.Content = strings.TrimSpace(entry.Content)
	if entry.Content == "" {
		return Entry{}, fmt.Errorf("memory content is required")
	}
	if entry.Scope == ScopeGlobal {
		entry.ProjectKey = ""
		entry.ProjectPath = ""
	} else if entry.ProjectKey == "" {
		return Entry{}, fmt.Errorf("project memory requires project key")
	}
	entry.Confidence = clampConfidence(entry.Confidence)
	entry.Fingerprint = EntryFingerprint(entry.Scope, entry.ProjectKey, entry.Type, entry.Content)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
insert into memory_entries(scope, project_key, project_path, type, content, source_note, confidence, fingerprint, status, source_session_id, created_at, updated_at)
values(?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)
on conflict(fingerprint) do update set
	status = 'active',
	source_note = case when excluded.source_note != '' then excluded.source_note else memory_entries.source_note end,
	confidence = max(memory_entries.confidence, excluded.confidence),
	source_session_id = case when excluded.source_session_id != '' then excluded.source_session_id else memory_entries.source_session_id end,
	updated_at = excluded.updated_at`,
		entry.Scope, entry.ProjectKey, entry.ProjectPath, entry.Type, entry.Content, strings.TrimSpace(entry.SourceNote), entry.Confidence, entry.Fingerprint, entry.SourceSessionID, now, now)
	if err != nil {
		return Entry{}, err
	}
	return s.GetEntryByFingerprint(ctx, entry.Fingerprint)
}

// GetEntryByFingerprint reads a memory entry by fingerprint.
func (s *Store) GetEntryByFingerprint(ctx context.Context, fingerprint string) (Entry, error) {
	row := s.db.QueryRowContext(ctx, `
select id, scope, project_key, project_path, type, content, source_note, confidence, fingerprint, status, source_session_id, created_at, updated_at, last_used_at, use_count
from memory_entries
where fingerprint = ?`, fingerprint)
	return scanEntry(row)
}

// ArchiveFingerprints marks memories with the specified fingerprints as archived.
func (s *Store) ArchiveFingerprints(ctx context.Context, fingerprints []string) error {
	if len(fingerprints) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, fingerprint := range fingerprints {
		if strings.TrimSpace(fingerprint) == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `
update memory_entries
set status = ?, updated_at = ?
where fingerprint = ?`, statusArchived, now, strings.TrimSpace(fingerprint)); err != nil {
			return err
		}
	}
	return nil
}

// DecayConfidence applies discrete confidence decay to active memories by memory type.
// decayDays specifies how many days per 1-point decay by type; unspecified types do not decay.
// confidence never goes below 1.
func (s *Store) DecayConfidence(ctx context.Context, decayDays map[string]int) error {
	if len(decayDays) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for entryType, days := range decayDays {
		if days <= 0 {
			continue
		}
		rows, err := s.db.QueryContext(ctx, `
select id, confidence, updated_at from memory_entries
where status = 'active' and type = ?`, entryType)
		if err != nil {
			return err
		}
		var ids []int64
		var newConfidences []int
		for rows.Next() {
			var id int64
			var confidence int
			var updatedAtStr string
			if err := rows.Scan(&id, &confidence, &updatedAtStr); err != nil {
				rows.Close()
				return err
			}
			updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtStr)
			if err != nil {
				continue
			}
			ageDays := int(now.Sub(updatedAt).Hours() / 24)
			decay := ageDays / days
			newConf := confidence - decay
			if newConf < 1 {
				newConf = 1
			}
			if newConf != confidence {
				ids = append(ids, id)
				newConfidences = append(newConfidences, newConf)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i, id := range ids {
			if _, err := s.db.ExecContext(ctx, `
update memory_entries set confidence = ? where id = ?`, newConfidences[i], id); err != nil {
				return err
			}
		}
	}
	return nil
}

// PruneStaleEntries archives active memories that have been unused for a long time and have low confidence.
// Condition: confidence <= 1 and use_count == 0 and last_used_at exceeds maxUnusedDays.
// Returns the number of archived entries. Entries with empty last_used_at are calculated by updated_at.
func (s *Store) PruneStaleEntries(ctx context.Context, maxUnusedDays int) (int, error) {
	if maxUnusedDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -maxUnusedDays).Format(time.RFC3339Nano)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `
	update memory_entries
	set status = ?, updated_at = ?
	where status = 'active'
		and confidence <= 1
		and use_count = 0
		and (last_used_at = '' or last_used_at < ?)
		and updated_at < ?`, statusArchived, now, cutoff, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Search returns active memories matching the query using case-insensitive substring matching.
// The query is split by whitespace into terms; an entry matches if any term appears as a
// substring in its content, source_note, type, or project_path. Matches are ranked by
// confidence, use_count, and recency. An empty query returns no results.
func (s *Store) Search(ctx context.Context, cwd, query string, limit int) ([]Entry, error) {
	terms := splitSearchTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	projectKey, _ := ProjectIdentity(cwd)
	if limit <= 0 {
		limit = defaultPromptEntryLimit
	}
	rows, err := s.db.QueryContext(ctx, `
select id, scope, project_key, project_path, type, content, source_note, confidence, fingerprint, status, source_session_id, created_at, updated_at, last_used_at, use_count
from memory_entries
where status = 'active'
	and (scope = 'global' or (scope = 'project' and project_key = ?))
order by confidence desc, use_count desc, updated_at desc, id desc`, projectKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all, err := scanEntries(rows)
	if err != nil {
		return nil, err
	}
	matched := make([]Entry, 0, limit)
	for _, entry := range all {
		if entryMatchesAnyTerm(entry, terms) {
			matched = append(matched, entry)
			if len(matched) >= limit {
				break
			}
		}
	}
	if len(matched) == 0 {
		return matched, nil
	}
	if err := s.markUsed(ctx, matched); err != nil {
		return nil, err
	}
	return matched, nil
}

// ListEntries returns active memories under the specified scope.
func (s *Store) ListEntries(ctx context.Context, scope, projectKey string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
select id, scope, project_key, project_path, type, content, source_note, confidence, fingerprint, status, source_session_id, created_at, updated_at, last_used_at, use_count
from memory_entries
where status = 'active' and scope = ? and project_key = ?
order by type, updated_at desc, id desc`, scope, projectKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// EnqueueSessionExtract schedules memory extraction from the specified session.
func (s *Store) EnqueueSessionExtract(ctx context.Context, sessionID, inputHash, model string) error {
	if sessionID == "" || inputHash == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
insert into memory_jobs(job_key, session_id, model, status, attempts, retry_after, lease_until, worker_id, last_error, created_at, updated_at, finished_at)
values(?, ?, ?, ?, 0, '', '', '', '', ?, ?, '')
on conflict(job_key) do update set
	model = excluded.model,
	status = excluded.status,
	attempts = 0,
	retry_after = '',
	lease_until = '',
	worker_id = '',
	last_error = '',
	updated_at = excluded.updated_at,
	finished_at = ''`,
		sessionID, sessionID, strings.TrimSpace(model), statusPending, now, now)
	return err
}

// ClaimNextJob fetches an executable background job and sets its lease.
func (s *Store) ClaimNextJob(ctx context.Context, workerID string, lease time.Duration) (Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	row := tx.QueryRowContext(ctx, `
select job_key, session_id, model, status, attempts, worker_id, last_error
from memory_jobs
where (
	status = 'pending'
	or (status = 'failed' and (retry_after = '' or retry_after <= ?))
	or (status = 'running' and lease_until != '' and lease_until <= ?)
)
and (lease_until = '' or lease_until <= ?)
order by updated_at, job_key
limit 1`, nowText, nowText, nowText)
	job, err := scanJob(row)
	if errorsIsNoRows(err) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	leaseUntil := now.Add(lease).Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
update memory_jobs
set status = 'running',
	attempts = attempts + 1,
	lease_until = ?,
	worker_id = ?,
	updated_at = ?
where job_key = ?
	and (
		status = 'pending'
		or (status = 'failed' and (retry_after = '' or retry_after <= ?))
		or (status = 'running' and lease_until != '' and lease_until <= ?)
	)
	and (lease_until = '' or lease_until <= ?)`, leaseUntil, workerID, nowText, job.Key, nowText, nowText, nowText)
	if err != nil {
		return Job{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Job{}, false, err
	}
	if rows == 0 {
		return Job{}, false, tx.Commit()
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	job.Status = statusRunning
	job.Attempts++
	job.WorkerID = workerID
	return job, true, nil
}

// CompleteJob marks a background job as succeeded.
func (s *Store) CompleteJob(ctx context.Context, job Job) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
update memory_jobs
set status = 'succeeded',
	lease_until = '',
	worker_id = '',
	last_error = '',
	updated_at = ?,
	finished_at = ?
where job_key = ?
	and status = 'running'
	and worker_id = ?
	and attempts = ?`, now, now, job.Key, job.WorkerID, job.Attempts)
	return err
}

// FailJob marks a background job as failed and sets the retry time.
func (s *Store) FailJob(ctx context.Context, job Job, jobErr error) error {
	delay := time.Duration(min(max(job.Attempts, 1), 5)) * time.Minute
	now := time.Now().UTC()
	message := ""
	if jobErr != nil {
		message = jobErr.Error()
	}
	_, err := s.db.ExecContext(ctx, `
update memory_jobs
set status = 'failed',
	retry_after = ?,
	lease_until = '',
	worker_id = '',
	last_error = ?,
	updated_at = ?
where job_key = ?
	and status = 'running'
	and worker_id = ?
	and attempts = ?`, now.Add(delay).Format(time.RFC3339Nano), message, now.Format(time.RFC3339Nano), job.Key, job.WorkerID, job.Attempts)
	return err
}

// Counts returns memory statistics for doctor diagnostics.
func (s *Store) Counts(ctx context.Context) (Counts, error) {
	var counts Counts
	if err := s.db.QueryRowContext(ctx, `select count(*) from memory_entries where status = 'active'`).Scan(&counts.Entries); err != nil {
		return Counts{}, err
	}
	if err := s.db.QueryRowContext(ctx, `select count(*) from memory_jobs where status = 'pending' or status = 'running'`).Scan(&counts.Pending); err != nil {
		return Counts{}, err
	}
	if err := s.db.QueryRowContext(ctx, `select count(*) from memory_jobs where status = 'failed'`).Scan(&counts.Failed); err != nil {
		return Counts{}, err
	}
	return counts, nil
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var entries []Entry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEntry(row scanner) (Entry, error) {
	var entry Entry
	var createdAt, updatedAt, lastUsedAt string
	if err := row.Scan(&entry.ID, &entry.Scope, &entry.ProjectKey, &entry.ProjectPath, &entry.Type, &entry.Content, &entry.SourceNote, &entry.Confidence, &entry.Fingerprint, &entry.Status, &entry.SourceSessionID, &createdAt, &updatedAt, &lastUsedAt, &entry.UseCount); err != nil {
		return Entry{}, err
	}
	var err error
	entry.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Entry{}, err
	}
	entry.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Entry{}, err
	}
	if lastUsedAt != "" {
		entry.LastUsedAt, err = time.Parse(time.RFC3339Nano, lastUsedAt)
		if err != nil {
			return Entry{}, err
		}
	}
	return entry, nil
}

func scanJob(row scanner) (Job, error) {
	var job Job
	if err := row.Scan(&job.Key, &job.SessionID, &job.Model, &job.Status, &job.Attempts, &job.WorkerID, &job.LastError); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Store) markUsed(ctx context.Context, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, entry := range entries {
		if _, err := s.db.ExecContext(ctx, `
update memory_entries
set last_used_at = ?, use_count = use_count + 1
where id = ?`, now, entry.ID); err != nil {
			return err
		}
	}
	return nil
}

// splitSearchTerms splits a query string into lowercase search terms by whitespace.
func splitSearchTerms(query string) []string {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return nil
	}
	return strings.Fields(query)
}

// entryMatchesAnyTerm returns true if any term appears as a case-insensitive substring
// in the entry's content, source_note, type, or project_path.
func entryMatchesAnyTerm(entry Entry, terms []string) bool {
	haystack := strings.ToLower(entry.Content + "\n" + entry.SourceNote + "\n" + entry.Type + "\n" + entry.ProjectPath)
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			return true
		}
	}
	return false
}

func validateScope(scope string) error {
	if scope != ScopeGlobal && scope != ScopeProject {
		return fmt.Errorf("invalid memory scope %q", scope)
	}
	return nil
}

func validateType(entryType string) error {
	if entryType != TypeInstruction && entryType != TypeFact && entryType != TypeWorkflow {
		return fmt.Errorf("invalid memory type %q", entryType)
	}
	return nil
}

func clampConfidence(confidence int) int {
	if confidence <= 0 {
		return 3
	}
	return min(max(confidence, 1), 5)
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}
