package session

import (
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
)

// 助手消息记下产生它的模型的完整引用，记录据此盖上 model_ref，而不是会话表里的；
// 只有模型 ID、没有 provider 的消息拒绝保存。
func TestAppendMessageStampsModel(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	id := uuid.NewV7().String()
	info, err := store.Create(ctx, CreateOptions{
		ID: id, Cwd: t.TempDir(), ModelRef: "p/old", SystemPrompt: "提示词",
	})
	if err != nil {
		t.Fatal(err)
	}
	reply := agent.AssistantMessage{
		Model: "m", StopReason: agent.StopReasonStop, Timestamp: time.Now(),
	}
	if _, err := store.AppendMessage(ctx, id, info.LastSeq, reply, ""); err == nil {
		t.Fatal("模型不是 provider/model-id 的形式，应当拒绝")
	}
	reply.Model = "p/m"
	if _, err := store.AppendMessage(ctx, id, info.LastSeq, reply, agent.ThinkingHigh); err != nil {
		t.Fatal(err)
	}
	records, err := store.Messages(ctx, id, 0, 10)
	last := records[len(records)-1]
	if err != nil || last.ModelRef != "p/m" || last.ThinkingLevel != agent.ThinkingHigh {
		t.Errorf("记录 = %+v（%v），期望盖上 p/m 与 high，而不是会话表里的 p/old", last, err)
	}
}

// 压缩记录重放时与运行时的压缩同一条规则：摘要加上它之前的最后 Kept 条消息，
// 更早的消息只留在历史里；Kept 超过此前的消息数的记录是损坏的。
func TestLoadAppliesCompaction(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	id := uuid.NewV7().String()
	info, err := store.Create(ctx, CreateOptions{
		ID: id, Cwd: t.TempDir(), ModelRef: "p/m", SystemPrompt: "提示词",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	text := func(s string) []agent.Content { return []agent.Content{agent.TextContent{Text: s}} }
	seq := info.LastSeq
	append := func(m agent.Message) {
		t.Helper()
		if seq, err = store.AppendMessage(ctx, id, seq, m, ""); err != nil {
			t.Fatal(err)
		}
	}
	append(agent.UserMessage{Content: text("u1"), Timestamp: now})
	reply := func(s string) agent.AssistantMessage {
		return agent.AssistantMessage{
			Model: "p/m", StopReason: agent.StopReasonStop, Content: text(s), Timestamp: now,
		}
	}
	append(reply("a1"))
	append(agent.UserMessage{Content: text("u2"), Timestamp: now})
	append(reply("a2"))
	summary := agent.SystemMessage{
		Type: agent.SystemMessageTypeCompaction, Content: "摘要", Kept: 2, Timestamp: now,
	}
	append(summary)
	append(agent.UserMessage{Content: text("u3"), Timestamp: now})

	loaded, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := loaded.Messages[0].(agent.SystemMessage)
	if len(loaded.Messages) != 4 || first.Content != summary.Content || first.Kept != 2 ||
		loaded.Messages[1].(agent.UserMessage).Content[0].(agent.TextContent).Text != "u2" {
		t.Errorf("上下文 = %v，期望 [摘要, u2, a2, u3]", loaded.Messages)
	}
	records, err := store.Messages(ctx, id, 0, 50)
	if err != nil || len(records) != 7 {
		t.Errorf("历史 = %d 条（%v），期望全部 7 条仍在，供界面往上翻", len(records), err)
	}
	// 会话目录只取某条记录之前的用户消息，压缩记录之前的也在。
	prompts, err := store.Prompts(ctx, id, seq)
	if err != nil || len(prompts) != 2 || prompts[0].Seq >= prompts[1].Seq ||
		prompts[0].Message.(agent.UserMessage).Content[0].(agent.TextContent).Text != "u1" {
		t.Errorf("提问 = %v（%v），期望 [u1, u2]", prompts, err)
	}

	append(agent.SystemMessage{
		Type: agent.SystemMessageTypeCompaction, Content: "坏的", Kept: 99, Timestamp: now,
	})
	if _, err := store.Load(ctx, id); err == nil {
		t.Error("Kept 超过此前消息数的压缩记录应当报错")
	}
}
