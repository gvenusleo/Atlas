package store

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaVersion 是当前的结构版本，记录在 SQLite 的 user_version 里。1 是正式发布的第一版。
// 改表结构必须同时做三件事：升这个版本、在 migrations 末尾追加把上一版升上来的迁移、
// 更新 schemaSQL（全新建库用的当前结构）。
const schemaVersion = 1

// schemaSQL 是当前版本的完整结构，用来初始化全新的库；旧版本的库由 migrations 升上来，
// 两条路径得到的结构必须一致。
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
`

// migrations 把旧版本的库逐版本升级：migrations[n-1] 把版本 n 升到 n+1，在调用方的事务里
// 执行，失败时整个升级回滚，库保持原样。长度必须是 schemaVersion-1。
var migrations []func(ctx context.Context, tx *sql.Tx) error

// initSchema 在事务中让库达到当前版本：全新的库直接建当前结构，旧版本的库按顺序迁移，
// 比程序支持的更新的版本报错。
func initSchema(ctx context.Context, db *sql.DB) error {
	if len(migrations) != schemaVersion-1 {
		return fmt.Errorf("迁移有 %d 个，期望 %d 个", len(migrations), schemaVersion-1)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	switch {
	case version == 0:
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
	case version < schemaVersion:
		for v := version; v < schemaVersion; v++ {
			if err := migrations[v-1](ctx, tx); err != nil {
				return fmt.Errorf("数据库从版本 %d 升到 %d 失败: %w", v, v+1, err)
			}
		}
	case version > schemaVersion:
		return fmt.Errorf("数据库版本 %d 比当前程序支持的 %d 新：请升级 Atlas",
			version, schemaVersion)
	}
	if version != schemaVersion {
		// PRAGMA 不能带参数；schemaVersion 是常量，直接拼进去。
		pragma := fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)
		if _, err := tx.ExecContext(ctx, pragma); err != nil {
			return err
		}
	}
	return tx.Commit()
}
