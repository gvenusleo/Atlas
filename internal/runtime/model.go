package runtime

import (
	"cmp"
	"context"
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

// ModelCatalog 是当前配置的模型清单，不含凭据。
// DefaultModelError 非空表示默认模型引用无法解析；其余模型仍可正常列出。
type ModelCatalog struct {
	DefaultModel      string
	DefaultModelError string
	Models            []ModelInfo
}

// ModelInfo 描述模型元数据与当前 runtime 的协议支持情况。
type ModelInfo struct {
	Ref               string
	Provider          string
	ID                string
	Name              string
	API               config.WireAPI
	ContextWindow     int64
	MaxOutput         int64
	Modalities        config.Modalities
	Cost              config.Cost
	Tools             bool
	ThinkingLevels    []agent.ThinkingLevel
	Supported         bool
	UnsupportedReason string
}

// Models 读取当前配置中的所有模型，不加载凭据或发起网络请求。
func (m *Manager) Models(ctx context.Context) (ModelCatalog, error) {
	ctx, done, err := m.begin(ctx)
	if err != nil {
		return ModelCatalog{}, err
	}
	defer done()

	settings, err := config.Load()
	if err != nil {
		return ModelCatalog{}, err
	}
	catalog := ModelCatalog{DefaultModel: settings.Model}
	for providerID, provider := range settings.Providers {
		for modelID, metadata := range provider.Models {
			catalog.Models = append(catalog.Models, modelInfo(settings, providerID, modelID, metadata))
		}
	}
	if settings.Model != "" && !slices.ContainsFunc(catalog.Models, func(model ModelInfo) bool {
		return model.Ref == settings.Model
	}) {
		if providerID, modelID, err := config.ParseModel(settings.Model); err != nil {
			catalog.DefaultModelError = err.Error()
		} else {
			catalog.Models = append(catalog.Models, modelInfo(settings, providerID, modelID, config.Model{}))
		}
	}
	slices.SortStableFunc(catalog.Models, func(a, b ModelInfo) int {
		return cmp.Compare(a.Ref, b.Ref)
	})
	if err := ctx.Err(); err != nil {
		return ModelCatalog{}, err
	}
	return catalog, nil
}

func modelInfo(settings *config.Settings, providerID, modelID string, metadata config.Model) ModelInfo {
	provider := settings.Providers[providerID]
	info := ModelInfo{
		Ref:           providerID + "/" + modelID,
		Provider:      providerID,
		ID:            modelID,
		Name:          cmp.Or(metadata.Name, modelID),
		API:           provider.WireAPI,
		ContextWindow: metadata.ContextWindow,
		MaxOutput:     metadata.MaxOutput,
		Modalities: config.Modalities{
			Input:  slices.Clone(metadata.Modalities.Input),
			Output: slices.Clone(metadata.Modalities.Output),
		},
		Cost:           metadata.Cost,
		Tools:          metadata.Tools == nil || *metadata.Tools,
		ThinkingLevels: usableThinkingLevels(metadata),
	}
	if _, err := settings.Provider(providerID); err != nil {
		info.UnsupportedReason = err.Error()
	} else if provider.WireAPI != config.WireAPIOpenAIResponses {
		info.UnsupportedReason = fmt.Sprintf("provider %q 的协议 %q 尚未实现", providerID, provider.WireAPI)
	} else if !info.Tools {
		info.UnsupportedReason = fmt.Sprintf("模型 %q 不支持工具调用，无法用于当前 Agent", info.Ref)
	} else {
		info.Supported = true
	}
	return info
}

// usableThinkingLevels 保留配置中的有效 effort 档位顺序并去重。
func usableThinkingLevels(metadata config.Model) []agent.ThinkingLevel {
	if !metadata.Reasoning {
		return nil
	}
	var levels []agent.ThinkingLevel
	for _, option := range metadata.ReasoningOptions {
		if option.Type != config.ReasoningTypeEffort {
			continue
		}
		for _, value := range option.Values {
			level := agent.ThinkingLevel(value)
			switch level {
			case agent.ThinkingOff, agent.ThinkingMinimal, agent.ThinkingLow, agent.ThinkingMedium,
				agent.ThinkingHigh, agent.ThinkingXHigh, agent.ThinkingMax:
				if !slices.Contains(levels, level) {
					levels = append(levels, level)
				}
			}
		}
	}
	return levels
}

func defaultThinkingLevel(metadata config.Model) agent.ThinkingLevel {
	if levels := usableThinkingLevels(metadata); len(levels) > 0 {
		return levels[0]
	}
	return ""
}

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
	if metadata.Tools != nil && !*metadata.Tools {
		return "", nil, fmt.Errorf("模型 %q 不支持工具调用，无法用于当前 Agent", modelRef)
	}
	if thinkingLevel != "" {
		if !metadata.Reasoning {
			return "", nil, fmt.Errorf("模型 %q 未声明思考能力", modelRef)
		}
		if !slices.Contains(usableThinkingLevels(metadata), thinkingLevel) {
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
