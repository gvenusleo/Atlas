package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Entry 是要追加到历史的一条记录。store 不解析 Body，Kind 由调用方给出。
type Entry struct {
	Kind  string
	Body  string // JSON 对象
	Blobs []Blob // Body 引用的二进制，与记录在同一个事务里写入
	// ContextFrom 不为 0 时，把会话的上下文起点设为这个序号（压缩记录用）。
	ContextFrom int64
}

// Record 是历史里的一条记录；Seq 从 1 起连续，也是分页游标。
type Record struct {
	Seq  int64
	Kind string
	Body string
}

// Blob 是消息引用的二进制数据，以内容哈希寻址，属于某个会话。
type Blob struct {
	Hash string
	MIME string
	Data []byte
}

// insert 在调用方的事务里插入一条记录和它引用的 blob；序号与会话表由调用方维护。
func insert(ctx context.Context, tx *sql.Tx, id string, seq int64, e Entry) error {
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO messages (session_id, seq, kind, body) VALUES (?, ?, ?, ?)",
		id, seq, e.Kind, e.Body); err != nil {
		return err
	}
	for _, b := range e.Blobs {
		if _, err := tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO blobs (session_id, hash, mime, data) VALUES (?, ?, ?, ?)",
			id, b.Hash, b.MIME, b.Data); err != nil {
			return err
		}
	}
	return nil
}

// Append 追加一条记录并返回它的序号。lastSeq 是调用方知道的最后序号；库里的不是它时说明
// 别的写入者追加过，返回 ErrConflict，什么也不写。
func (d *DB) Append(ctx context.Context, id string, lastSeq int64, e Entry) (int64, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var seq int64
	err = tx.QueryRowContext(ctx, `UPDATE sessions
        SET last_seq = last_seq + 1, updated_at = max(updated_at, ?),
            context_from = CASE WHEN ? > 0 THEN ? ELSE context_from END
        WHERE id = ? AND last_seq = ?
        RETURNING last_seq`,
		time.Now().UnixMilli(), e.ContextFrom, e.ContextFrom, id, lastSeq).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		if tx.QueryRowContext(ctx, "SELECT 1 FROM sessions WHERE id = ?", id).Scan(&exists) == nil {
			return 0, ErrConflict
		}
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if err := insert(ctx, tx, id, seq, e); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return seq, nil
}

// Context 读出会话的元信息，和从上下文起点到最后一条的全部记录，正序；
// 序号必须连续，到元信息的 LastSeq 为止。
func (d *DB) Context(ctx context.Context, id string) (Info, []Record, error) {
	info, err := d.Get(ctx, id)
	if err != nil {
		return Info{}, nil, err
	}
	records, err := d.query(ctx, id, "seq >= ? AND seq <= ? ORDER BY seq",
		info.ContextFrom, info.LastSeq)
	if err != nil {
		return Info{}, nil, err
	}
	for i, r := range records {
		if want := info.ContextFrom + int64(i); r.Seq != want {
			return Info{}, nil, fmt.Errorf("会话 %s 在记录 %d 前缺少记录", id, want)
		}
	}
	if want := info.LastSeq - info.ContextFrom + 1; int64(len(records)) != max(want, 0) {
		return Info{}, nil, fmt.Errorf("会话 %s 的记录数量与元信息不一致", id)
	}
	return info, records, nil
}

// Page 取 before 之前的 limit 条记录，正序；before 为 0 时取最新的一页。
func (d *DB) Page(ctx context.Context, id string, before int64, limit int) ([]Record, error) {
	if before == 0 {
		before = 1<<63 - 1
	}
	records, err := d.query(ctx, id, "seq < ? ORDER BY seq DESC LIMIT ?", before, limit)
	slices.Reverse(records)
	return records, err
}

// ListKind 取 before 之前某一种类的全部记录，正序；before 为 0 时不设上限。
func (d *DB) ListKind(ctx context.Context, id, kind string, before int64) ([]Record, error) {
	if before == 0 {
		before = 1<<63 - 1
	}
	return d.query(ctx, id, "kind = ? AND seq < ? ORDER BY seq", kind, before)
}

// Latest 取某一种类最新的一条记录，没有时返回 ErrNotFound。
func (d *DB) Latest(ctx context.Context, id, kind string) (Record, error) {
	records, err := d.query(ctx, id, "kind = ? ORDER BY seq DESC LIMIT 1", kind)
	if err != nil {
		return Record{}, err
	}
	if len(records) == 0 {
		return Record{}, ErrNotFound
	}
	return records[0], nil
}

// Blob 读出会话引用的二进制数据，没有时返回 ErrNotFound。
func (d *DB) Blob(ctx context.Context, id, hash string) (Blob, error) {
	b := Blob{Hash: hash}
	err := d.db.QueryRowContext(ctx,
		"SELECT mime, data FROM blobs WHERE session_id = ? AND hash = ?", id, hash).
		Scan(&b.MIME, &b.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return Blob{}, ErrNotFound
	}
	return b, err
}

// query 按条件读出一个会话的记录。
func (d *DB) query(ctx context.Context, id, where string, args ...any) ([]Record, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT seq, kind, body FROM messages WHERE session_id = ? AND "+where,
		append([]any{id}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.Seq, &r.Kind, &r.Body); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
