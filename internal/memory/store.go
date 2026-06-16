// Package memory 管理 Atlas 的长期记忆条目、检索摘要和后台任务。
package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/model"
	_ "modernc.org/sqlite"
)

const (
	// ScopeGlobal 表示跨项目生效的用户偏好和长期指令。
	ScopeGlobal = "global"
	// ScopeProject 表示只对当前项目生效的事实和工作流。
	ScopeProject = "project"

	// TypeInstruction 表示用户长期偏好或约束。
	TypeInstruction = "instruction"
	// TypeFact 表示项目事实。
	TypeFact = "fact"
	// TypeWorkflow 表示可复用的项目操作流程。
	TypeWorkflow = "workflow"

	statusActive    = "active"
	statusArchived  = "archived"
	statusPending   = "pending"
	statusRunning   = "running"
	statusSucceeded = "succeeded"
	statusFailed    = "failed"

	// JobKindSessionExtract 表示从会话 transcript 中抽取记忆。
	JobKindSessionExtract = "session_extract"
	// JobKindScopeSummarize 表示重新生成某个记忆作用域的摘要。
	JobKindScopeSummarize = "scope_summarize"

	defaultPromptEntryLimit = 12
	maxPromptContextRunes   = 6000
)

var ftsTokenPattern = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// Store 读写 Atlas 长期记忆数据库。
type Store struct {
	db *sql.DB
}

// Entry 是一条长期记忆。
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

// Summary 是某个记忆作用域的模型摘要。
type Summary struct {
	Scope       string
	ProjectKey  string
	ProjectPath string
	Content     string
	EntryCount  int
	InputHash   string
	UpdatedAt   time.Time
}

// Job 描述一条后台记忆任务。
type Job struct {
	Key         string
	Kind        string
	Scope       string
	ProjectKey  string
	ProjectPath string
	SessionID   string
	InputHash   string
	Model       string
	Status      string
	Attempts    int
	WorkerID    string
	LastError   string
}

// Counts 汇总后台记忆任务数量。
type Counts struct {
	Entries int
	Pending int
	Failed  int
}

// Open 打开 SQLite 记忆数据库。
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
	return &Store{db: db}, nil
}

// Close 关闭底层数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// EnsureSchema 创建长期记忆相关表结构。
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
create index if not exists memory_entries_source_idx
	on memory_entries(source_session_id, updated_at);

create virtual table if not exists memory_entries_fts
	using fts5(body, content='memory_entries', content_rowid='id', tokenize='unicode61 remove_diacritics 1');

create trigger if not exists memory_entries_ai after insert on memory_entries begin
	insert into memory_entries_fts(rowid, body)
	values (new.id, new.type || char(10) || new.content || char(10) || new.source_note || char(10) || new.project_path);
end;

create trigger if not exists memory_entries_ad after delete on memory_entries begin
	insert into memory_entries_fts(memory_entries_fts, rowid, body)
	values ('delete', old.id, old.type || char(10) || old.content || char(10) || old.source_note || char(10) || old.project_path);
end;

create trigger if not exists memory_entries_au after update on memory_entries begin
	insert into memory_entries_fts(memory_entries_fts, rowid, body)
	values ('delete', old.id, old.type || char(10) || old.content || char(10) || old.source_note || char(10) || old.project_path);
	insert into memory_entries_fts(rowid, body)
	values (new.id, new.type || char(10) || new.content || char(10) || new.source_note || char(10) || new.project_path);
end;

create table if not exists memory_summaries (
	scope text not null,
	project_key text not null default '',
	project_path text not null default '',
	content text not null,
	entry_count integer not null default 0,
	input_hash text not null default '',
	updated_at text not null,
	primary key(scope, project_key)
);

create table if not exists memory_jobs (
	job_key text primary key,
	kind text not null,
	scope text not null default '',
	project_key text not null default '',
	project_path text not null default '',
	session_id text not null default '',
	input_hash text not null default '',
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

// ProjectIdentity 返回工作目录对应的稳定项目标识。
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

// TranscriptHash 返回 transcript 内容的稳定哈希。
func TranscriptHash(messages []model.Message) string {
	var builder strings.Builder
	for _, msg := range messages {
		builder.WriteString(string(msg.Role))
		builder.WriteByte('\x00')
		builder.WriteString(msg.Content)
		builder.WriteByte('\x00')
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

// EntryFingerprint 返回记忆条目的去重指纹。
func EntryFingerprint(scope, projectKey, entryType, content string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(content), " "))
	sum := sha256.Sum256([]byte(scope + "\x00" + projectKey + "\x00" + entryType + "\x00" + normalized))
	return hex.EncodeToString(sum[:])
}

// UpsertEntry 写入或更新一条 active 记忆。
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

// GetEntryByFingerprint 按指纹读取记忆条目。
func (s *Store) GetEntryByFingerprint(ctx context.Context, fingerprint string) (Entry, error) {
	row := s.db.QueryRowContext(ctx, `
select id, scope, project_key, project_path, type, content, source_note, confidence, fingerprint, status, source_session_id, created_at, updated_at, last_used_at, use_count
from memory_entries
where fingerprint = ?`, fingerprint)
	return scanEntry(row)
}

// ArchiveFingerprints 将指定指纹的记忆标记为 archived。
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

// Search 返回和 query 相关的 global/project active 记忆。
func (s *Store) Search(ctx context.Context, cwd, query string, limit int) ([]Entry, error) {
	projectKey, _ := ProjectIdentity(cwd)
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return s.ListPromptEntries(ctx, cwd, limit)
	}
	if limit <= 0 {
		limit = defaultPromptEntryLimit
	}
	rows, err := s.db.QueryContext(ctx, `
select e.id, e.scope, e.project_key, e.project_path, e.type, e.content, e.source_note, e.confidence, e.fingerprint, e.status, e.source_session_id, e.created_at, e.updated_at, e.last_used_at, e.use_count
from memory_entries_fts
join memory_entries e on e.id = memory_entries_fts.rowid
where memory_entries_fts match ?
	and e.status = 'active'
	and (e.scope = 'global' or (e.scope = 'project' and e.project_key = ?))
order by bm25(memory_entries_fts), e.updated_at desc
limit ?`, ftsQuery, projectKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries, err := scanEntries(rows)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return entries, nil
	}
	if err := s.markUsed(ctx, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ListPromptEntries 返回没有 query 时用于提示词注入的最近记忆。
func (s *Store) ListPromptEntries(ctx context.Context, cwd string, limit int) ([]Entry, error) {
	projectKey, _ := ProjectIdentity(cwd)
	if limit <= 0 {
		limit = defaultPromptEntryLimit
	}
	rows, err := s.db.QueryContext(ctx, `
select id, scope, project_key, project_path, type, content, source_note, confidence, fingerprint, status, source_session_id, created_at, updated_at, last_used_at, use_count
from memory_entries
where status = 'active'
	and (scope = 'global' or (scope = 'project' and project_key = ?))
order by updated_at desc, id desc
limit ?`, projectKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries, err := scanEntries(rows)
	if err != nil {
		return nil, err
	}
	if err := s.markUsed(ctx, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ListEntries 返回指定作用域下的 active 记忆。
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

// SaveSummary 保存指定作用域的记忆摘要。
func (s *Store) SaveSummary(ctx context.Context, summary Summary) error {
	if err := validateScope(summary.Scope); err != nil {
		return err
	}
	if summary.Scope == ScopeGlobal {
		summary.ProjectKey = ""
		summary.ProjectPath = ""
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
insert into memory_summaries(scope, project_key, project_path, content, entry_count, input_hash, updated_at)
values(?, ?, ?, ?, ?, ?, ?)
on conflict(scope, project_key) do update set
	project_path = excluded.project_path,
	content = excluded.content,
	entry_count = excluded.entry_count,
	input_hash = excluded.input_hash,
	updated_at = excluded.updated_at`,
		summary.Scope, summary.ProjectKey, summary.ProjectPath, strings.TrimSpace(summary.Content), summary.EntryCount, summary.InputHash, now)
	return err
}

// LoadSummaries 返回 global 和当前 project 的摘要。
func (s *Store) LoadSummaries(ctx context.Context, cwd string) ([]Summary, error) {
	projectKey, _ := ProjectIdentity(cwd)
	rows, err := s.db.QueryContext(ctx, `
select scope, project_key, project_path, content, entry_count, input_hash, updated_at
from memory_summaries
where scope = 'global' or (scope = 'project' and project_key = ?)
order by scope`, projectKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []Summary
	for rows.Next() {
		summary, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// PromptContext 构造系统提示词可直接注入的长期记忆上下文。
func (s *Store) PromptContext(ctx context.Context, cwd, query string) (string, error) {
	summaries, err := s.LoadSummaries(ctx, cwd)
	if err != nil {
		return "", err
	}
	entries, err := s.Search(ctx, cwd, query, defaultPromptEntryLimit)
	if err != nil {
		return "", err
	}
	if len(summaries) == 0 && len(entries) == 0 {
		return "", nil
	}
	var builder strings.Builder
	builder.WriteString("The following long-term memories were automatically retrieved from prior Atlas sessions. They may be stale; verify project facts with tools when precision matters.\n")
	writeSummaries(&builder, summaries)
	writeEntries(&builder, entries)
	return trimPromptContext(builder.String()), nil
}

// EnqueueSessionExtract 安排从指定 session 中抽取记忆。
func (s *Store) EnqueueSessionExtract(ctx context.Context, sessionID, cwd, inputHash, model string) error {
	if sessionID == "" || inputHash == "" {
		return nil
	}
	projectKey, projectPath := ProjectIdentity(cwd)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
insert into memory_jobs(job_key, kind, project_key, project_path, session_id, input_hash, model, status, attempts, retry_after, lease_until, worker_id, last_error, created_at, updated_at, finished_at)
values(?, ?, ?, ?, ?, ?, ?, ?, 0, '', '', '', '', ?, ?, '')
on conflict(job_key) do update set
	project_key = excluded.project_key,
	project_path = excluded.project_path,
	input_hash = excluded.input_hash,
	model = excluded.model,
	status = excluded.status,
	attempts = 0,
	retry_after = '',
	lease_until = '',
	worker_id = '',
	last_error = '',
	updated_at = excluded.updated_at,
	finished_at = ''`,
		JobKindSessionExtract+":"+sessionID, JobKindSessionExtract, projectKey, projectPath, sessionID, inputHash, strings.TrimSpace(model), statusPending, now, now)
	return err
}

// EnqueueSummarize 安排重新生成指定记忆作用域摘要。
func (s *Store) EnqueueSummarize(ctx context.Context, scope, projectKey, projectPath, model string) error {
	if err := validateScope(scope); err != nil {
		return err
	}
	if scope == ScopeGlobal {
		projectKey = ""
		projectPath = ""
	}
	jobKey := JobKindScopeSummarize + ":" + scope + ":" + projectKey
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
insert into memory_jobs(job_key, kind, scope, project_key, project_path, model, status, attempts, retry_after, lease_until, worker_id, last_error, created_at, updated_at, finished_at)
values(?, ?, ?, ?, ?, ?, ?, 0, '', '', '', '', ?, ?, '')
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
		jobKey, JobKindScopeSummarize, scope, projectKey, projectPath, strings.TrimSpace(model), statusPending, now, now)
	return err
}

// ClaimNextJob 获取一条可执行的后台任务并设置租约。
func (s *Store) ClaimNextJob(ctx context.Context, workerID string, lease time.Duration) (Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	row := tx.QueryRowContext(ctx, `
select job_key, kind, scope, project_key, project_path, session_id, input_hash, model, status, attempts, worker_id, last_error
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

// CompleteJob 标记后台任务成功。
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

// FailJob 标记后台任务失败并设置重试时间。
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

// Counts 返回 doctor 使用的记忆统计。
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

func scanSummary(row scanner) (Summary, error) {
	var summary Summary
	var updatedAt string
	if err := row.Scan(&summary.Scope, &summary.ProjectKey, &summary.ProjectPath, &summary.Content, &summary.EntryCount, &summary.InputHash, &updatedAt); err != nil {
		return Summary{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Summary{}, err
	}
	summary.UpdatedAt = updated
	return summary, nil
}

func scanJob(row scanner) (Job, error) {
	var job Job
	if err := row.Scan(&job.Key, &job.Kind, &job.Scope, &job.ProjectKey, &job.ProjectPath, &job.SessionID, &job.InputHash, &job.Model, &job.Status, &job.Attempts, &job.WorkerID, &job.LastError); err != nil {
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

func writeSummaries(builder *strings.Builder, summaries []Summary) {
	for _, summary := range summaries {
		if strings.TrimSpace(summary.Content) == "" {
			continue
		}
		builder.WriteString("\n### ")
		if summary.Scope == ScopeGlobal {
			builder.WriteString("Global summary\n")
		} else {
			builder.WriteString("Project summary")
			if summary.ProjectPath != "" {
				builder.WriteString(" (")
				builder.WriteString(filepath.ToSlash(summary.ProjectPath))
				builder.WriteString(")")
			}
			builder.WriteString("\n")
		}
		builder.WriteString(strings.TrimSpace(summary.Content))
		builder.WriteString("\n")
	}
}

func writeEntries(builder *strings.Builder, entries []Entry) {
	if len(entries) == 0 {
		return
	}
	builder.WriteString("\n### Retrieved entries\n")
	for _, entry := range entries {
		builder.WriteString("- [")
		builder.WriteString(entry.Scope)
		builder.WriteString("/")
		builder.WriteString(entry.Type)
		builder.WriteString("] ")
		builder.WriteString(strings.TrimSpace(entry.Content))
		builder.WriteString("\n")
	}
}

func trimPromptContext(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) <= maxPromptContextRunes {
		return string(runes)
	}
	return string(runes[:maxPromptContextRunes]) + "\n..."
}

func buildFTSQuery(query string) string {
	tokens := ftsTokenPattern.FindAllString(strings.ToLower(query), -1)
	if len(tokens) == 0 {
		return ""
	}
	if len(tokens) > 12 {
		tokens = tokens[:12]
	}
	uniq := slices.Compact(tokens)
	quoted := make([]string, 0, len(uniq))
	for _, token := range uniq {
		if token == "" {
			continue
		}
		quoted = append(quoted, `"`+token+`"`)
	}
	return strings.Join(quoted, " OR ")
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

// EntriesInputHash 返回一组记忆条目的摘要输入哈希。
func EntriesInputHash(entries []Entry) string {
	var builder strings.Builder
	for _, entry := range entries {
		builder.WriteString(entry.Fingerprint)
		builder.WriteByte('\x00')
		builder.WriteString(entry.UpdatedAt.UTC().Format(time.RFC3339Nano))
		builder.WriteByte('\x00')
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}
