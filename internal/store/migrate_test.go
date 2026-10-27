package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// v1Schema 是版本 1 的结构，用来造一个旧库。
const v1Schema = `
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

// 版本 1 的库升级之后，结构与全新建的库一致，旧会话照常可读，都是普通会话、没有属主。
func TestMigrationMatchesFreshSchema(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	oldPath := filepath.Join(dir, "old.db")
	raw, err := sql.Open("sqlite", oldPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		v1Schema,
		`INSERT INTO sessions (id, title, cwd, model, created_at, updated_at)
         VALUES ('s1', '旧会话', '/', 'p/m', 1, 1)`,
		"PRAGMA user_version = 1",
	} {
		if _, err := raw.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := Open(ctx, oldPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrated.Close() })
	fresh, err := Open(ctx, filepath.Join(dir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })

	if got, want := shape(t, migrated), shape(t, fresh); got != want {
		t.Errorf("升级后的结构与全新建库不一致：\n升级的：\n%s\n全新的：\n%s", got, want)
	}
	info, err := migrated.Get(ctx, "s1")
	if err != nil || info.Title != "旧会话" || info.Type != "interactive" || info.OwnerID != "" {
		t.Errorf("旧会话 = %+v（%v），期望原样保留、类型是 interactive、没有属主", info, err)
	}
}

// shape 列出库里每张表的列（名称、类型、非空、默认值、主键）与每个索引的列，用来比较结构。
func shape(t *testing.T, d *DB) string {
	t.Helper()
	var lines []string
	for _, query := range []string{
		`SELECT m.name, p.name, p.type, p."notnull", coalesce(p.dflt_value, ''), p.pk
         FROM sqlite_schema m, pragma_table_info(m.name) p
         WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%' ORDER BY m.name, p.cid`,
		`SELECT m.name, i.name, x.seqno, x.name, x.desc
         FROM sqlite_schema m, pragma_index_list(m.name) i, pragma_index_xinfo(i.name) x
         WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%' AND x.key = 1
         ORDER BY m.name, i.name, x.seqno`,
	} {
		rows, err := d.db.QueryContext(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			for i := range values {
				values[i] = new(any)
			}
			if err := rows.Scan(values...); err != nil {
				t.Fatal(err)
			}
			var line []string
			for _, v := range values {
				line = append(line, fmt.Sprint(*v.(*any)))
			}
			lines = append(lines, strings.Join(line, " "))
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return strings.Join(lines, "\n")
}
