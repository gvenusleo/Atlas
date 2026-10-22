package core

import (
	"testing"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
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
