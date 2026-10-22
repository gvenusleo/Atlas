package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
)

// Record 包含完整消息及该次响应的请求设置；Seq 是分页游标。
type Record struct {
	Seq           int64
	Message       agent.Message
	ModelRef      string
	ThinkingLevel agent.ThinkingLevel
}

// Loaded 提取最新提示词快照与原始对话；缺失工具结果在模型请求转换时补齐。
type Loaded struct {
	Info         Info
	SystemPrompt agent.SystemMessage
	PromptSeq    int64
	Messages     []agent.Message // 发给模型的对话：不含提示词快照，压缩记录之前的消息已折叠。
}

// insertMessage 在调用方事务中插入消息，顺序号和会话更新时间由调用方维护。
func insertMessage(ctx context.Context, tx *sql.Tx, id string, seq int64,
	modelRef string, thinkingLevel agent.ThinkingLevel, m encodedMessage) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO messages
        (session_id, seq, role, stop_reason, is_error, model_ref, thinking,
         created_at, message_json, usage)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, seq, m.role, m.stop, m.isError, modelRef, thinkingLevel,
		m.timestamp.UnixMilli(), m.body, m.usage)
	return err
}

// AppendMessage 编码在事务外完成；消息、顺序号和更新时间一次提交。
// lastSeq 是调用方已知的最后序号；会话已被其他写入者追加时返回 ErrConflict。
func (s *Store) AppendMessage(
	ctx context.Context, id string, lastSeq int64, message agent.Message,
) (int64, error) {
	m, err := encodeMessage(message)
	if err != nil {
		return 0, fmt.Errorf("编码消息失败: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var seq int64
	var modelRef string
	var thinkingLevel agent.ThinkingLevel
	err = tx.QueryRowContext(ctx, `UPDATE sessions
        SET last_seq = last_seq + 1, updated_at = max(updated_at, ?)
        WHERE id = ? AND last_seq = ?
        RETURNING last_seq, model_ref, thinking`,
		time.Now().UnixMilli(), id, lastSeq).Scan(&seq, &modelRef, &thinkingLevel)
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
	if assistant, ok := message.(agent.AssistantMessage); ok {
		_, modelID, err := config.ParseModel(modelRef)
		if err != nil {
			return 0, err
		}
		if assistant.Model != modelID {
			return 0, fmt.Errorf("响应模型 %q 与会话模型 %q 不一致", assistant.Model, modelID)
		}
	} else {
		modelRef, thinkingLevel = "", ""
	}
	if err := insertMessage(ctx, tx, id, seq, modelRef, thinkingLevel, m); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return seq, nil
}

const recordColumns = "seq, role, stop_reason, is_error, model_ref, thinking, message_json, usage"

// scanRecord 恢复历史记录并检查查询列与正文、模型归属的一致性。
func scanRecord(row scanner) (Record, error) {
	var record Record
	var role agent.Role
	var stop agent.StopReason
	var isError bool
	var body string
	var usage sql.NullString
	err := row.Scan(&record.Seq, &role, &stop, &isError,
		&record.ModelRef, &record.ThinkingLevel, &body, &usage)
	if err != nil {
		return Record{}, err
	}
	message, err := decodeMessage(body, usage)
	if err != nil {
		return Record{}, fmt.Errorf("消息 %d 解码失败: %w", record.Seq, err)
	}
	if message.Role() != role {
		return Record{}, fmt.Errorf("消息 %d 的角色列与正文不一致", record.Seq)
	}
	var expectedStop agent.StopReason
	var expectedError bool
	switch m := message.(type) {
	case agent.AssistantMessage:
		expectedStop, expectedError = m.StopReason, m.StopReason == agent.StopReasonError
		if err := validateSettings(record.ModelRef, record.ThinkingLevel); err != nil {
			return Record{}, fmt.Errorf("消息 %d 请求设置无效: %w", record.Seq, err)
		}
		_, modelID, _ := config.ParseModel(record.ModelRef)
		if m.Model != modelID {
			return Record{}, fmt.Errorf("消息 %d 的模型归属不一致", record.Seq)
		}
	case agent.ToolResultMessage:
		expectedError = m.IsError
	}
	if stop != expectedStop || isError != expectedError {
		return Record{}, fmt.Errorf("消息 %d 的状态列与正文不一致", record.Seq)
	}
	if role != agent.RoleAssistant && (record.ModelRef != "" || record.ThinkingLevel != "") {
		return Record{}, fmt.Errorf("消息 %d 含非助手消息的请求设置", record.Seq)
	}
	record.Message = message
	return record, nil
}

// Messages 取 beforeSeq 之前的一页完整原始记录，包含提示词快照，返回正序；beforeSeq 为 0 时取最新一页。
func (s *Store) Messages(ctx context.Context, id string, beforeSeq int64, limit int) ([]Record, error) {
	if beforeSeq < 0 {
		return nil, errors.New("消息分页游标不能为负数")
	}
	limit, err := pageSize(limit)
	if err != nil {
		return nil, err
	}
	info, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	query := "SELECT " + recordColumns + " FROM messages WHERE session_id = ? AND seq <= ?"
	args := []any{id, info.LastSeq}
	if beforeSeq > 0 {
		query += " AND seq < ?"
		args = append(args, beforeSeq)
	}
	query += " ORDER BY seq DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records, err := scanRecords(rows, id)
	if err != nil {
		return nil, err
	}
	slices.Reverse(records)
	return records, nil
}

// Prompts 取 beforeSeq 之前的全部用户消息，返回正序，供会话目录使用。
func (s *Store) Prompts(ctx context.Context, id string, beforeSeq int64) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordColumns+` FROM messages
		WHERE session_id = ? AND role = 'user' AND seq < ? ORDER BY seq`, id, beforeSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows, id)
}

// scanRecords 按查询结果的顺序读出全部记录。
func scanRecords(rows *sql.Rows, id string) ([]Record, error) {
	var records []Record
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("会话 %s: %w", id, err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// Load 以读取元信息时的 LastSeq 为边界，后续追加不会混入本次恢复。
func (s *Store) Load(ctx context.Context, id string) (Loaded, error) {
	info, err := s.Get(ctx, id)
	if err != nil {
		return Loaded{}, err
	}
	if err := validateSettings(info.ModelRef, info.ThinkingLevel); err != nil {
		return Loaded{}, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+recordColumns+
		" FROM messages WHERE session_id = ? AND seq <= ? ORDER BY seq", id, info.LastSeq)
	if err != nil {
		return Loaded{}, err
	}
	defer rows.Close()
	loaded := Loaded{Info: info}
	var last int64
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return Loaded{}, fmt.Errorf("会话 %s: %w", id, err)
		}
		if record.Seq != last+1 {
			return Loaded{}, fmt.Errorf("会话 %s 在消息 %d 前缺少记录", id, record.Seq)
		}
		last = record.Seq
		system, isSystem := record.Message.(agent.SystemMessage)
		isPrompt := isSystem && system.Type == agent.SystemMessageTypePrompt
		if last == 1 && !isPrompt {
			return Loaded{}, fmt.Errorf("会话 %s 缺少初始系统提示词快照", id)
		}
		switch {
		case isPrompt:
			loaded.SystemPrompt, loaded.PromptSeq = system, record.Seq
		case isSystem && system.Type == agent.SystemMessageTypeCompaction:
			// 与运行时的压缩同一条规则：摘要加上之前的最后 Kept 条消息，更早的不再发给模型。
			if system.Kept > len(loaded.Messages) {
				return Loaded{}, fmt.Errorf(
					"会话 %s 的压缩记录 %d 要保留 %d 条消息，此前只有 %d 条",
					id, record.Seq, system.Kept, len(loaded.Messages))
			}
			kept := loaded.Messages[len(loaded.Messages)-system.Kept:]
			loaded.Messages = append([]agent.Message{system}, kept...)
		default:
			loaded.Messages = append(loaded.Messages, record.Message)
		}
	}
	if err := rows.Err(); err != nil {
		return Loaded{}, err
	}
	if last != info.LastSeq || last == 0 {
		return Loaded{}, fmt.Errorf("会话 %s 的消息数量与元信息不一致", id)
	}
	return loaded, nil
}

// Usage 聚合独立的小 JSON 字段，不读取消息正文；它表示累计消耗而非上下文大小。
func (s *Store) Usage(ctx context.Context, id string) (agent.Usage, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return agent.Usage{}, err
	}
	var usage agent.Usage
	err := s.db.QueryRowContext(ctx, `SELECT
        coalesce(sum(json_extract(usage, '$.input_tokens')), 0),
        coalesce(sum(json_extract(usage, '$.output_tokens')), 0),
        coalesce(sum(json_extract(usage, '$.cache_read_tokens')), 0),
        coalesce(sum(json_extract(usage, '$.cache_write_tokens')), 0),
        coalesce(sum(json_extract(usage, '$.total_tokens')), 0)
        FROM messages WHERE session_id = ? AND role = 'assistant'`, id).
		Scan(&usage.InputTokens, &usage.OutputTokens,
			&usage.CacheReadTokens, &usage.CacheWriteTokens, &usage.TotalTokens)
	if err != nil {
		return agent.Usage{}, err
	}
	if err := usage.Validate(); err != nil {
		return agent.Usage{}, err
	}
	return usage, nil
}
