package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/session"
)

// 选模型的回退链：上次用的可用就用它，否则默认模型，再否则第一个可用的；
// 思考档位新模型支持就保留，否则用它的默认档；没有声明或不支持的模型都算不可用。
func TestPickModel(t *testing.T) {
	noTools := false
	effort := []config.ReasoningOption{{Type: config.ReasoningTypeEffort, Values: []string{"low", "high"}}}
	settings := &config.Settings{
		Model: "p/default",
		Providers: map[string]config.Provider{"p": {
			WireAPI: config.WireAPIOpenAIResponses, BaseURL: "http://x",
			Models: map[string]config.Model{
				"default": {Reasoning: true, ReasoningOptions: effort},
				"plain":   {},
				"first":   {Reasoning: true, ReasoningOptions: effort},
				"notools": {Tools: &noTools},
			},
		}},
	}
	for _, c := range []struct {
		name, preferred string
		thinking        agent.ThinkingLevel
		wantRef         string
		wantThinking    agent.ThinkingLevel
	}{
		{"上次用的可用", "p/plain", "high", "p/plain", ""},
		{"上次用的已被删除，退到默认", "p/gone", "high", "p/default", "high"},
		{"上次用的不支持工具，退到默认", "p/notools", "", "p/default", "low"},
		{"没有上次用的，用默认", "", "", "p/default", "low"},
	} {
		ref, thinking, err := pickModel(settings, c.preferred, c.thinking)
		if err != nil || ref != c.wantRef || thinking != c.wantThinking {
			t.Errorf("%s：得到 %q %q（%v），期望 %q %q",
				c.name, ref, thinking, err, c.wantRef, c.wantThinking)
		}
	}

	settings.Model = "p/gone"
	if ref, _, err := pickModel(settings, "p/gone", ""); err != nil || ref != "p/default" {
		t.Errorf("默认模型也没有声明：得到 %q（%v），期望清单里第一个可用的 p/default", ref, err)
	}
	settings.Providers = nil
	if _, _, err := pickModel(settings, "p/gone", ""); err == nil {
		t.Error("没有任何可用的模型，应当报错")
	}
}

// openWith 把 HOME 指到临时目录，写入 settings.json 与 p 的 API key，返回打开在其上的宿主。
func openWith(t *testing.T, settings string) *Manager {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".atlas")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"settings.json": settings, "auth.json": `{"p":"key"}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Open(t.Context(), filepath.Join(home, "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

// stored 在库里建一个会话，它上次用的是 ref 与 high。
func stored(t *testing.T, m *Manager, ref string) string {
	t.Helper()
	id := uuid.NewV7().String()
	_, err := m.store.Create(t.Context(), session.CreateOptions{
		ID: id, Cwd: t.TempDir(), ModelRef: ref, ThinkingLevel: agent.ThinkingHigh,
		SystemPrompt: "提示词",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// 从 settings.json 里删掉会话上次用的模型之后，会话照常恢复，退到默认模型并存下来，
// 用户可以接着用或者换模型；原先是恢复时就用旧模型校验，一失败整个会话都打不开。
func TestResumeFallsBackWhenModelIsGone(t *testing.T) {
	m := openWith(t, `{"model": "p/a", "providers": {"p": {"api": "openai-responses",
		"base_url": "http://x", "models": {"a": {"reasoning": true,
		"reasoning_options": [{"type": "effort", "values": ["low", "high"]}]}}}}}`)
	id := stored(t, m, "p/gone")

	s, err := m.Resume(t.Context(), id)
	if err != nil {
		t.Fatalf("Resume = %v，期望模型被删也能恢复", err)
	}
	if state := s.State(); state.ModelRef != "p/a" || state.ThinkingLevel != agent.ThinkingHigh {
		t.Errorf("State = %q %q，期望退到默认模型 p/a，保留 high", state.ModelRef, state.ThinkingLevel)
	}
	if info, err := m.Get(t.Context(), id); err != nil || info.ModelRef != "p/a" {
		t.Errorf("会话表里的模型 = %q（%v），期望存下退到的 p/a", info.ModelRef, err)
	}
}

// 一个可用的模型都没有时，会话也照常恢复，运行时才报原因，并且不使会话失效。
func TestResumeWithoutUsableModel(t *testing.T) {
	m := openWith(t, `{}`)
	s, err := m.Resume(t.Context(), stored(t, m, "p/gone"))
	if err != nil {
		t.Fatalf("Resume = %v，期望没有可用模型也能恢复", err)
	}
	_, err = s.Run(t.Context(), user(), nil)
	if err == nil || !strings.Contains(err.Error(), "没有可用的模型") {
		t.Fatalf("Run = %v，期望报没有可用的模型", err)
	}
	if s.State().NeedsReload {
		t.Error("没有可用模型不应使会话失效")
	}
}

// 新会话先只在内存里，库里没有它；第一条消息保存时才写进库，连同之前换的模型；
// 写进库以后不能再换工作目录。没发过消息的新会话不在库里留下空记录。
func TestNewSessionIsSavedWithFirstMessage(t *testing.T) {
	m := openWith(t, `{"model": "p/a", "providers": {"p": {"api": "openai-responses",
		"base_url": "http://x", "models": {"a": {}, "b": {}}}}}`)
	s, err := m.New(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if state := s.State(); !state.Draft || state.ModelRef != "p/a" {
		t.Fatalf("State = %+v，期望是模型 p/a 的草稿", state)
	}
	if _, err := m.Get(t.Context(), s.id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("发消息之前 Get = %v，期望库里没有", err)
	}
	dir := t.TempDir()
	if err := s.SetCwd(t.Context(), dir); err != nil || s.State().Cwd != dir {
		t.Fatalf("草稿换工作目录：%v，Cwd = %q，期望 %q", err, s.State().Cwd, dir)
	}
	if err := s.SetModel(t.Context(), "p/b", ""); err != nil {
		t.Fatal(err)
	}

	s.loop.Model.Stream = reply
	if _, err := s.Run(t.Context(), user(), nil); err != nil {
		t.Fatal(err)
	}
	info, err := m.Get(t.Context(), s.id)
	if err != nil || info.ModelRef != "p/b" || info.Cwd != dir {
		t.Errorf("发消息之后 Get = %+v（%v），期望存下 p/b 与换过的工作目录", info, err)
	}
	if records, err := m.History(t.Context(), s.id, 0, 10); err != nil || len(records) != 3 {
		t.Errorf("历史 = %d 条（%v），期望提示词快照、用户消息、助手消息", len(records), err)
	}
	if err := s.SetCwd(t.Context(), dir); !errors.Is(err, ErrNotDraft) {
		t.Errorf("保存之后换工作目录 = %v，期望 ErrNotDraft", err)
	}
}
