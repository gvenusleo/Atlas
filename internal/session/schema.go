package session

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 1

const schemaSQL = `
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    cwd TEXT NOT NULL,
    model_ref TEXT NOT NULL,
    thinking TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_seq INTEGER NOT NULL DEFAULT 0 CHECK (last_seq >= 0)
) STRICT;
CREATE INDEX sessions_updated ON sessions(updated_at DESC, id DESC);
CREATE INDEX sessions_cwd_updated ON sessions(cwd, updated_at DESC, id DESC);
CREATE TABLE messages (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL CHECK (seq > 0),
    role TEXT NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool_result')),
    stop_reason TEXT NOT NULL DEFAULT '',
    is_error INTEGER NOT NULL DEFAULT 0 CHECK (is_error IN (0, 1)),
    model_ref TEXT NOT NULL DEFAULT '',
    thinking TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    message_json TEXT NOT NULL CHECK (json_valid(message_json) AND json_type(message_json) = 'object'),
    usage TEXT CHECK (usage IS NULL OR (json_valid(usage) AND json_type(usage) = 'object')),
    PRIMARY KEY (session_id, seq),
    CHECK ((role = 'assistant' AND usage IS NOT NULL) OR (role != 'assistant' AND usage IS NULL))
) STRICT;
PRAGMA user_version = 1;
`

// initSchema 在事务中初始化空数据库，已有数据库只接受当前版本，不执行迁移。
func initSchema(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	switch version {
	case 0:
		// 不在未知的已有数据库中擅自建表。
		var tables int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema
            WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return fmt.Errorf("数据库没有 Atlas 版本标记，但已有数据表")
		}
		if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
			return err
		}
	case schemaVersion:
	default:
		return fmt.Errorf("不支持数据库版本 %d，当前支持 %d", version, schemaVersion)
	}
	return tx.Commit()
}
