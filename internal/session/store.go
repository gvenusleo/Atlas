// Package session 使用 SQLite 保存会话与完整消息；同一会话的运行由调用方串行管理。
package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("会话不存在")
	// ErrConflict 表示会话已被其他写入者追加消息，调用方持有的历史已过期。
	ErrConflict = errors.New("会话已被其他写入者修改")
)

// Store 可供多个会话并发使用；Close 前须等待全部运行和数据库操作结束。
type Store struct {
	db *sql.DB
}

// Info 是会话元信息，时间统一以 UTC 返回。
type Info struct {
	ID            string
	Name          string
	Cwd           string
	ModelRef      string
	ThinkingLevel agent.ThinkingLevel
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastSeq       int64
}

// CreateOptions 保存模型设置与初始系统提示词快照；后续快照通过 AppendMessage 追加。
type CreateOptions struct {
	ID            string // 由宿主层预先保留，避免提交后产生重复运行实例。
	Cwd           string
	ModelRef      string
	ThinkingLevel agent.ThinkingLevel
	SystemPrompt  string
}

// Open 打开数据库；path 为空时使用 ~/.atlas/atlas.db。
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		dir, err := config.Dir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(dir, "atlas.db")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("数据库路径解析失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开 %s 失败: %w", path, err)
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if err := errors.Join(statErr, closeErr); err != nil {
		return nil, fmt.Errorf("检查 %s 失败: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s 不是普通文件", path)
	}

	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	q := url.Values{
		"_journal_mode": {"WAL"},
		"_foreign_keys": {"on"},
		"_synchronous":  {"FULL"},
		"_busy_timeout": {"5000"},
		"_txlock":       {"immediate"},
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("打开 %s 失败: %w", path, err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if err := initSchema(ctx, db); err != nil {
		return nil, errors.Join(fmt.Errorf("初始化 %s 失败: %w", path, err), db.Close())
	}
	return &Store{db: db}, nil
}

// Close 关闭连接池；调用方须先结束全部会话运行。
func (s *Store) Close() error {
	return s.db.Close()
}

// validateSettings 检查模型引用和档位词表，具体模型能力由宿主层校验。
func validateSettings(modelRef string, thinkingLevel agent.ThinkingLevel) error {
	if _, _, err := config.ParseModel(modelRef); err != nil {
		return err
	}
	if thinkingLevel != "" && !thinkingLevel.Valid() {
		return fmt.Errorf("未知思考档位 %q", thinkingLevel)
	}
	return nil
}

// Create 在同一事务中创建会话和首条系统消息。
func (s *Store) Create(ctx context.Context, opts CreateOptions) (Info, error) {
	if err := validateSettings(opts.ModelRef, opts.ThinkingLevel); err != nil {
		return Info{}, err
	}
	cwd, err := filepath.Abs(opts.Cwd)
	if err != nil {
		return Info{}, err
	}
	dir, err := os.Stat(cwd)
	if err != nil {
		return Info{}, fmt.Errorf("检查 %s 失败: %w", cwd, err)
	}
	if !dir.IsDir() {
		return Info{}, fmt.Errorf("%s 不是目录", cwd)
	}
	if strings.TrimSpace(opts.SystemPrompt) == "" {
		return Info{}, errors.New("系统提示词不能为空")
	}
	id, err := uuid.Parse(opts.ID)
	if err != nil || id.String() != opts.ID {
		return Info{}, errors.New("会话 ID 必须是规范 UUID")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	info := Info{ID: opts.ID, Cwd: cwd, ModelRef: opts.ModelRef,
		ThinkingLevel: opts.ThinkingLevel, CreatedAt: now, UpdatedAt: now, LastSeq: 1}
	encoded, err := encodeMessage(agent.SystemMessage{
		Type: agent.SystemMessageTypePrompt, Content: opts.SystemPrompt, Timestamp: now,
	})
	if err != nil {
		return Info{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Info{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions
        (id, cwd, model_ref, thinking, created_at, updated_at, last_seq)
        VALUES (?, ?, ?, ?, ?, ?, 1)`,
		info.ID, info.Cwd, info.ModelRef, info.ThinkingLevel,
		now.UnixMilli(), now.UnixMilli()); err != nil {
		return Info{}, err
	}
	if err := insertMessage(ctx, tx, info.ID, 1, "", "", encoded); err != nil {
		return Info{}, err
	}
	if err := tx.Commit(); err != nil {
		return Info{}, err
	}
	return info, nil
}

const infoColumns = "id, name, cwd, model_ref, thinking, created_at, updated_at, last_seq"

type scanner interface {
	Scan(...any) error
}

// scanInfo 将数据库元信息转为 UTC 时间，查无记录时返回 ErrNotFound。
func scanInfo(row scanner) (Info, error) {
	var info Info
	var created, updated int64
	err := row.Scan(&info.ID, &info.Name, &info.Cwd, &info.ModelRef, &info.ThinkingLevel,
		&created, &updated, &info.LastSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return Info{}, ErrNotFound
	}
	if err != nil {
		return Info{}, err
	}
	info.CreatedAt = time.UnixMilli(created).UTC()
	info.UpdatedAt = time.UnixMilli(updated).UTC()
	return info, nil
}

// Get 查询会话元信息，不加载消息历史。
func (s *Store) Get(ctx context.Context, id string) (Info, error) {
	return scanInfo(s.db.QueryRowContext(ctx, "SELECT "+infoColumns+" FROM sessions WHERE id = ?", id))
}

// ListCursor 与列表最后一项对应；列表在并发更新下是实时视图，不提供跨页快照。
type ListCursor struct {
	UpdatedAt time.Time
	ID        string
}

// List 按更新时间倒序分页；cwd 为空时列出所有工作区。
func (s *Store) List(ctx context.Context, cwd string, before *ListCursor, limit int) ([]Info, error) {
	limit, err := pageSize(limit)
	if err != nil {
		return nil, err
	}
	query := "SELECT " + infoColumns + " FROM sessions WHERE 1 = 1"
	var args []any
	if cwd != "" {
		cwd, err = filepath.Abs(cwd)
		if err != nil {
			return nil, err
		}
		query += " AND cwd = ?"
		args = append(args, cwd)
	}
	if before != nil {
		query += " AND (updated_at, id) < (?, ?)"
		args = append(args, before.UpdatedAt.UnixMilli(), before.ID)
	}
	query += " ORDER BY updated_at DESC, id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var infos []Info
	for rows.Next() {
		info, err := scanInfo(rows)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, rows.Err()
}

// pageSize 将零值解释为默认 50 条，拒绝超出 1 到 200 范围的显式值。
func pageSize(limit int) (int, error) {
	if limit == 0 {
		return 50, nil
	}
	if limit < 1 || limit > 200 {
		return 0, errors.New("分页条数必须在 1 到 200 之间")
	}
	return limit, nil
}

// UpdateSettings 调用方须确保会话空闲，并同步更新内存中的模型客户端和配置。
func (s *Store) UpdateSettings(
	ctx context.Context, id, modelRef string, thinkingLevel agent.ThinkingLevel,
) error {
	if err := validateSettings(modelRef, thinkingLevel); err != nil {
		return err
	}
	return s.change(ctx, `UPDATE sessions SET model_ref = ?, thinking = ?,
        updated_at = max(updated_at, ?) WHERE id = ?`,
		modelRef, thinkingLevel, time.Now().UnixMilli(), id)
}

// Rename 更新名称和活动时间，空名称表示清除名称。
func (s *Store) Rename(ctx context.Context, id, name string) error {
	return s.change(ctx, "UPDATE sessions SET name = ?, updated_at = max(updated_at, ?) WHERE id = ?",
		name, time.Now().UnixMilli(), id)
}

// Delete 调用方须确保没有运行或恢复中的任务，消息由外键级联删除。
func (s *Store) Delete(ctx context.Context, id string) error {
	return s.change(ctx, "DELETE FROM sessions WHERE id = ?", id)
}

// change 执行单会话更新或删除，未匹配任何会话时返回 ErrNotFound。
func (s *Store) change(ctx context.Context, query string, args ...any) error {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
