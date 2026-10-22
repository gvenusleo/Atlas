package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
)

// 追加只认调用方知道的最后序号：别的写入者先追加了，这次就冲突，什么也不写。
// 历史完整保留；会话目录只取某条之前的用户消息。
func TestAppendAndRead(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now()
	user := func(s string) agent.UserMessage {
		return agent.UserMessage{Timestamp: now,
			Content: []agent.Content{agent.TextContent{Text: s}}}
	}
	reply := agent.AssistantMessage{Model: "p/m", StopReason: agent.StopReasonStop, Timestamp: now}
	id := uuid.NewV7().String()
	info, err := db.Create(ctx, Info{ID: id, Cwd: "/", Model: "p/m"}, agent.SystemMessage{
		Type: agent.SystemMessageTypePrompt, Content: "提示词", Timestamp: now,
	}, user("u1"))
	if err != nil || info.LastSeq != 2 {
		t.Fatalf("Create = %+v（%v），期望 LastSeq 为 2", info, err)
	}
	seq, err := db.Append(ctx, id, info.LastSeq, reply)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Append(ctx, id, info.LastSeq, user("迟到的")); !errors.Is(err, ErrConflict) {
		t.Fatalf("用过时的序号追加 = %v，期望 ErrConflict", err)
	}
	if _, err := db.Append(ctx, id, seq, user("u2")); err != nil {
		t.Fatal(err)
	}

	_, records, err := db.Log(ctx, id)
	if err != nil || len(records) != 4 || records[3].Seq != 4 {
		t.Fatalf("Log = %d 条（%v），期望 4 条且序号连续", len(records), err)
	}
	page, err := db.Page(ctx, id, 4, 2)
	if err != nil || len(page) != 2 || page[0].Seq != 2 || page[1].Seq != 3 {
		t.Errorf("Page = %v（%v），期望第 2、3 条，正序", page, err)
	}
	prompts, err := db.Prompts(ctx, id, 4)
	if err != nil || len(prompts) != 1 || prompts[0].Seq != 2 {
		t.Errorf("Prompts = %v（%v），期望只有第 2 条", prompts, err)
	}
}
