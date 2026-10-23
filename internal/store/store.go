// Package store 用 SQLite 保存会话与它的历史。它只存取不透明的记录正文和二进制，
// 不理解消息内容，也不涉及上下文折叠与运行；
// 多个进程可以同时打开同一个库，同一会话的并发写入由 Append 的序号检查发现。
package store

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

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("会话不存在")
	// ErrConflict 表示会话已被别的写入者追加了消息，调用方知道的历史已经过时。
	ErrConflict = errors.New("会话已在别处更新")
)

// DB 可供多个会话并发使用；Close 前须等待全部数据库操作结束。
type DB struct {
	db *sql.DB
}

// Info 是会话的元信息，时间以 UTC 返回。
type Info struct {
	ID       string
	Title    string
	Cwd      string
	Model    string // 上次选的模型，provider/model-id
	Thinking string
	// LastSeq 是最后一条记录的序号，从 1 起；0 表示还没有记录。
	LastSeq int64
	// ContextFrom 是上下文的起点：读出上下文只需要从这个序号开始的记录。
	ContextFrom int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Open 打开 path 指定的数据库，不存在时创建。
func Open(ctx context.Context, path string) (*DB, error) {
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
	return &DB{db: db}, nil
}

// Close 关闭连接池；调用方须先结束全部数据库操作。
func (d *DB) Close() error {
	return d.db.Close()
}

// Create 建会话并在同一事务里写入最初的记录，返回带时间与 LastSeq 的元信息。
func (d *DB) Create(ctx context.Context, info Info, first ...Entry) (Info, error) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	info.CreatedAt, info.UpdatedAt = now, now
	info.LastSeq, info.ContextFrom = int64(len(first)), 1
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return Info{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions
        (id, title, cwd, model, thinking, last_seq, context_from, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		info.ID, info.Title, info.Cwd, info.Model, info.Thinking, info.LastSeq,
		info.ContextFrom, now.UnixMilli(), now.UnixMilli()); err != nil {
		return Info{}, err
	}
	for i, e := range first {
		if err := insert(ctx, tx, info.ID, int64(i+1), e); err != nil {
			return Info{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Info{}, err
	}
	return info, nil
}

const infoColumns = "id, title, cwd, model, thinking, last_seq, context_from, " +
	"created_at, updated_at"

type scanner interface {
	Scan(...any) error
}

// scanInfo 读出一行元信息，查无记录时返回 ErrNotFound。
func scanInfo(row scanner) (Info, error) {
	var info Info
	var created, updated int64
	err := row.Scan(&info.ID, &info.Title, &info.Cwd, &info.Model, &info.Thinking,
		&info.LastSeq, &info.ContextFrom, &created, &updated)
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

// Get 读会话的元信息，不读历史。
func (d *DB) Get(ctx context.Context, id string) (Info, error) {
	return scanInfo(d.db.QueryRowContext(ctx,
		"SELECT "+infoColumns+" FROM sessions WHERE id = ?", id))
}

// Cursor 是列表上一页最后一项的位置；列表是实时视图，不提供跨页快照。
type Cursor struct {
	UpdatedAt time.Time
	ID        string
}

// List 按更新时间倒序列出 limit 个会话；cwd 为空时列出所有工作目录的。
func (d *DB) List(ctx context.Context, cwd string, before *Cursor, limit int) ([]Info, error) {
	query := "SELECT " + infoColumns + " FROM sessions WHERE 1 = 1"
	var args []any
	if cwd != "" {
		query += " AND cwd = ?"
		args = append(args, cwd)
	}
	if before != nil {
		query += " AND (updated_at, id) < (?, ?)"
		args = append(args, before.UpdatedAt.UnixMilli(), before.ID)
	}
	query += " ORDER BY updated_at DESC, id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := d.db.QueryContext(ctx, query, args...)
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

// SetModel 记下会话选的模型与思考深度。
func (d *DB) SetModel(ctx context.Context, id, model, thinking string) error {
	return d.change(ctx, `UPDATE sessions SET model = ?, thinking = ?,
        updated_at = max(updated_at, ?) WHERE id = ?`,
		model, thinking, time.Now().UnixMilli(), id)
}

// Rename 改标题；空标题表示清除标题。
func (d *DB) Rename(ctx context.Context, id, title string) error {
	return d.change(ctx,
		"UPDATE sessions SET title = ?, updated_at = max(updated_at, ?) WHERE id = ?",
		title, time.Now().UnixMilli(), id)
}

// Delete 删除会话，消息由外键级联删除。
func (d *DB) Delete(ctx context.Context, id string) error {
	return d.change(ctx, "DELETE FROM sessions WHERE id = ?", id)
}

// change 执行单会话的更新或删除，没有匹配的会话时返回 ErrNotFound。
func (d *DB) change(ctx context.Context, query string, args ...any) error {
	result, err := d.db.ExecContext(ctx, query, args...)
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
