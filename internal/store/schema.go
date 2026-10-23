package store

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 4

const schemaSQL = `
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    cwd TEXT NOT NULL,
    model TEXT NOT NULL,
    thinking TEXT NOT NULL DEFAULT '',
    last_seq INTEGER NOT NULL DEFAULT 0 CHECK (last_seq >= 0),
    context_from INTEGER NOT NULL DEFAULT 1 CHECK (context_from >= 1),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;
CREATE INDEX sessions_updated ON sessions(updated_at DESC, id DESC);
CREATE INDEX sessions_cwd_updated ON sessions(cwd, updated_at DESC, id DESC);
CREATE TABLE messages (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL CHECK (seq > 0),
    kind TEXT NOT NULL CHECK (kind <> ''),
    body TEXT NOT NULL CHECK (json_valid(body) AND json_type(body) = 'object'),
    PRIMARY KEY (session_id, seq)
) STRICT;
CREATE INDEX messages_kind ON messages(session_id, kind, seq);
CREATE TABLE blobs (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    hash TEXT NOT NULL,
    mime TEXT NOT NULL,
    data BLOB NOT NULL,
    PRIMARY KEY (session_id, hash)
) STRICT;
PRAGMA user_version = 4;
`

// initSchema 在事务中初始化空数据库，已有数据库只接受当前版本，不做迁移。
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
		return fmt.Errorf("不支持数据库版本 %d，当前是 %d：请删除旧的数据库文件后重新打开",
			version, schemaVersion)
	}
	return tx.Commit()
}
