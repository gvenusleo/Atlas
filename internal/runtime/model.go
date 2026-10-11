package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/llm"
)

// modelConfig 校验协议和思考档位、加载凭据并创建客户端，不发起模型请求。
func modelConfig(settings *config.Settings, modelRef string, thinkingLevel agent.ThinkingLevel) (string, agent.StreamFn, error) {
	if modelRef == "" {
		return "", nil, fmt.Errorf("请在 ~/.atlas/settings.json 中配置 model")
	}
	provider, modelID, metadata, err := settings.ModelRef(modelRef)
	if err != nil {
		return "", nil, err
	}
	if provider.WireAPI != config.WireAPIOpenAIResponses {
		return "", nil, fmt.Errorf("协议 %q 尚未实现", provider.WireAPI)
	}
	if thinkingLevel != "" {
		if !metadata.Reasoning {
			return "", nil, fmt.Errorf("模型 %q 未声明思考能力", modelRef)
		}
		supported := slices.ContainsFunc(metadata.ReasoningOptions, func(option config.ReasoningOption) bool {
			return option.Type == config.ReasoningTypeEffort && slices.Contains(option.Values, string(thinkingLevel))
		})
		if !supported {
			return "", nil, fmt.Errorf("模型 %q 未声明支持思考档位 %q", modelRef, thinkingLevel)
		}
	}
	providerID, _, _ := config.ParseModel(modelRef)
	dir, err := config.Dir()
	if err != nil {
		return "", nil, err
	}
	key, err := config.APIKey(dir, providerID, os.Getenv)
	if err != nil {
		return "", nil, err
	}
	client := openai.NewClient(option.WithAPIKey(key), option.WithBaseURL(provider.BaseURL), option.WithMaxRetries(0))
	return modelID, llm.OpenAIResponsesStreamFn(client), nil
}

// checkWorkdir 要求恢复的工作目录是仍然存在的绝对目录。
func checkWorkdir(cwd string) error {
	if !filepath.IsAbs(cwd) {
		return fmt.Errorf("会话工作目录 %q 不是绝对路径", cwd)
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return fmt.Errorf("检查会话工作目录 %s 失败: %w", cwd, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("会话工作目录 %s 不是目录", cwd)
	}
	return nil
}
