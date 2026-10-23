package store

import (
	"errors"
	"path/filepath"
	"testing"
	"uuid"
)

// 追加只认调用方知道的最后序号：别的写入者先追加了，这次就冲突，什么也不写。
// 追加带上下文起点时，读出上下文只从起点开始；blob 与记录同属一个会话，按内容寻址。
func TestAppendAndRead(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	entry := func(kind string) Entry { return Entry{Kind: kind, Body: `{"k":"` + kind + `"}`} }
	id := uuid.NewV7().String()
	info, err := db.Create(ctx, Info{ID: id, Cwd: "/", Model: "p/m"},
		entry("system_prompt"), entry("user"))
	if err != nil || info.LastSeq != 2 || info.ContextFrom != 1 {
		t.Fatalf("Create = %+v（%v），期望 LastSeq 为 2、ContextFrom 为 1", info, err)
	}
	seq, err := db.Append(ctx, id, info.LastSeq, entry("assistant"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Append(ctx, id, info.LastSeq, entry("user")); !errors.Is(err, ErrConflict) {
		t.Fatalf("用过时的序号追加 = %v，期望 ErrConflict", err)
	}
	blob := Blob{Hash: "h", MIME: "image/png", Data: []byte{1, 2, 3}}
	compaction := Entry{
		Kind: "compaction", Body: `{"k":"compaction"}`, Blobs: []Blob{blob}, ContextFrom: 2,
	}
	if seq, err = db.Append(ctx, id, seq, compaction); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Append(ctx, id, seq, entry("user")); err != nil {
		t.Fatal(err)
	}

	got, records, err := db.Context(ctx, id)
	if err != nil || got.ContextFrom != 2 || len(records) != 4 || records[0].Seq != 2 {
		t.Fatalf("Context = %+v，%d 条（%v），期望从第 2 条起共 4 条", got, len(records), err)
	}
	page, err := db.Page(ctx, id, 4, 2)
	if err != nil || len(page) != 2 || page[0].Seq != 2 || page[1].Seq != 3 {
		t.Errorf("Page = %v（%v），期望第 2、3 条，正序", page, err)
	}
	users, err := db.ListKind(ctx, id, "user", 5)
	if err != nil || len(users) != 1 || users[0].Seq != 2 {
		t.Errorf("ListKind = %v（%v），期望只有第 2 条", users, err)
	}
	last, err := db.Latest(ctx, id, "compaction")
	if err != nil || last.Seq != 4 {
		t.Errorf("Latest = %v（%v），期望第 4 条", last, err)
	}
	if b, err := db.Blob(ctx, id, "h"); err != nil || string(b.Data) != "\x01\x02\x03" {
		t.Errorf("Blob = %v（%v），期望取回写入的数据", b, err)
	}
}
