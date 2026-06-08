package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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

// Store 读写 Atlas 的本地会话数据库。
type Store struct {
	db *sql.DB
}

// DefaultPath 返回用户主目录下的默认会话数据库路径。
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".atlas", defaultDBFileName), nil
}

// Open 打开 SQLite 会话数据库。
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
	return &Store{db: db}, nil
}

// Close 关闭底层数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// EnsureSchema 创建第一版会话表结构。
func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
create table if not exists sessions (
	id text primary key,
	title text not null default '',
	cwd text not null,
	created_at text not null,
	updated_at text not null
);

create table if not exists messages (
	id integer primary key autoincrement,
	session_id text not null,
	role text not null,
	content text not null,
	tool_call_id text not null default '',
	tool_calls_json text not null default '',
	created_at text not null,
	foreign key(session_id) references sessions(id) on delete cascade
);`)
	return err
}

// NewID 生成适合命令行展示和恢复的 session ID。
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

// ValidateID 校验用户提供或自动生成的 session ID。
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("session id is required")
	}
	if !sessionIDPattern.MatchString(id) {
		return fmt.Errorf("session id %q contains invalid characters", id)
	}
	return nil
}

// LoadTranscript 读取指定 session 的 transcript。不存在时返回空 transcript。
func (s *Store) LoadTranscript(ctx context.Context, sessionID string) (*transcript.Transcript, error) {
	if err := ValidateID(sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
select role, content, tool_call_id, tool_calls_json
from messages
where session_id = ?
order by id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trans := transcript.New()
	for rows.Next() {
		var role, content, toolCallID, toolCallsJSON string
		if err := rows.Scan(&role, &content, &toolCallID, &toolCallsJSON); err != nil {
			return nil, err
		}
		toolCalls, err := decodeToolCalls(toolCallsJSON)
		if err != nil {
			return nil, err
		}
		trans.Append(model.Message{
			Role:       model.Role(role),
			Content:    content,
			ToolCallID: toolCallID,
			ToolCalls:  toolCalls,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return trans, nil
}

// SaveTranscript 用给定消息快照覆盖保存指定 session。
func (s *Store) SaveTranscript(ctx context.Context, sessionID, cwd string, messages []model.Message) error {
	if err := ValidateID(sessionID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	title := titleFromMessages(messages)
	if _, err := tx.ExecContext(ctx, `
insert into sessions(id, title, cwd, created_at, updated_at)
values(?, ?, ?, ?, ?)
on conflict(id) do update set
	title = excluded.title,
	cwd = excluded.cwd,
	updated_at = excluded.updated_at`, sessionID, title, cwd, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from messages where session_id = ?`, sessionID); err != nil {
		return err
	}
	for _, msg := range messages {
		toolCallsJSON, err := encodeToolCalls(msg.ToolCalls)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
insert into messages(session_id, role, content, tool_call_id, tool_calls_json, created_at)
values(?, ?, ?, ?, ?, ?)`, sessionID, string(msg.Role), msg.Content, msg.ToolCallID, toolCallsJSON, now); err != nil {
			return err
		}
	}
	return tx.Commit()
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

func titleFromMessages(messages []model.Message) string {
	for _, msg := range messages {
		if msg.Role == model.RoleUser && strings.TrimSpace(msg.Content) != "" {
			return firstLine(msg.Content)
		}
	}
	return ""
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
