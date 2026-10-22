package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
)

// Record 是历史里的一条消息；Seq 从 1 起连续，也是分页游标。
type Record struct {
	Seq     int64
	Message agent.Message
}

// encode 把消息编成 JSON，拒绝还没定稿的助手消息。
func encode(m agent.Message) (string, error) {
	if a, ok := m.(agent.AssistantMessage); ok && a.StopReason == agent.StopReasonPending {
		return "", errors.New("不能保存未定稿的助手消息")
	}
	data, err := agent.MarshalMessage(m)
	if err != nil {
		return "", fmt.Errorf("编码消息失败: %w", err)
	}
	return string(data), nil
}

// encodeAll 在事务外编好全部消息。
func encodeAll(messages []agent.Message) ([]string, error) {
	bodies := make([]string, len(messages))
	for i, m := range messages {
		body, err := encode(m)
		if err != nil {
			return nil, err
		}
		bodies[i] = body
	}
	return bodies, nil
}

// insert 在调用方的事务里插入一条消息；序号与会话表由调用方维护。
func insert(ctx context.Context, tx *sql.Tx, id string, seq int64, role agent.Role, body string) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO messages (session_id, seq, role, body) VALUES (?, ?, ?, ?)", id, seq, role, body)
	return err
}

// Append 追加一条消息并返回它的序号。lastSeq 是调用方知道的最后序号；库里的不是它时说明
// 别的写入者追加过，返回 ErrConflict，什么也不写。
func (d *DB) Append(ctx context.Context, id string, lastSeq int64, m agent.Message) (int64, error) {
	body, err := encode(m)
	if err != nil {
		return 0, err
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var seq int64
	err = tx.QueryRowContext(ctx, `UPDATE sessions
        SET last_seq = last_seq + 1, updated_at = max(updated_at, ?)
        WHERE id = ? AND last_seq = ?
        RETURNING last_seq`,
		time.Now().UnixMilli(), id, lastSeq).Scan(&seq)
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
	if err := insert(ctx, tx, id, seq, m.Role(), body); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return seq, nil
}

// Log 读出会话的全部历史，正序；序号必须从 1 起连续，到元信息的 LastSeq 为止。
func (d *DB) Log(ctx context.Context, id string) (Info, []Record, error) {
	info, err := d.Get(ctx, id)
	if err != nil {
		return Info{}, nil, err
	}
	records, err := d.query(ctx, id, "seq <= ? ORDER BY seq", info.LastSeq)
	if err != nil {
		return Info{}, nil, err
	}
	for i, r := range records {
		if r.Seq != int64(i+1) {
			return Info{}, nil, fmt.Errorf("会话 %s 在消息 %d 前缺少记录", id, r.Seq)
		}
	}
	if int64(len(records)) != info.LastSeq {
		return Info{}, nil, fmt.Errorf("会话 %s 的消息数量与元信息不一致", id)
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

// Prompts 取 before 之前的全部用户消息，正序，供会话目录使用。
func (d *DB) Prompts(ctx context.Context, id string, before int64) ([]Record, error) {
	return d.query(ctx, id, "role = 'user' AND seq < ? ORDER BY seq", before)
}

// query 按条件读出一个会话的消息。
func (d *DB) query(ctx context.Context, id, where string, args ...any) ([]Record, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT seq, role, body FROM messages WHERE session_id = ? AND "+where,
		append([]any{id}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		var r Record
		var role agent.Role
		var body string
		if err := rows.Scan(&r.Seq, &role, &body); err != nil {
			return nil, err
		}
		if r.Message, err = agent.UnmarshalMessage([]byte(body)); err != nil {
			return nil, fmt.Errorf("会话 %s 的消息 %d 解码失败: %w", id, r.Seq, err)
		}
		if r.Message.Role() != role {
			return nil, fmt.Errorf("会话 %s 的消息 %d 角色与正文不一致", id, r.Seq)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
