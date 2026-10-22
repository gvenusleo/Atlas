package host

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/llm"
)

// ModelCatalog 是当前配置的模型清单，不含凭据。DefaultModel 是新会话用的有效默认模型：
// 配置的默认模型可用就是它，否则是清单里第一个可用的；一个可用的都没有时为空。
type ModelCatalog struct {
	DefaultModel string
	Models       []ModelInfo
}

// ModelInfo 描述模型元数据与当前 host 的协议支持情况。
type ModelInfo struct {
	Ref               string
	Provider          string
	ID                string
	Name              string
	API               config.WireAPI
	ContextWindow     int64
	MaxOutput         int64
	Modalities        config.Modalities
	Tools             bool
	ThinkingLevels    []agent.ThinkingLevel
	Supported         bool
	UnsupportedReason string
}

// Models 读取当前配置中的所有模型，不加载凭据或发起网络请求。清单按显示名排序，
// 不分大小写，显示名相同的再按引用排。
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
	catalog := ModelCatalog{Models: listModels(settings)}
	catalog.DefaultModel, _, _ = pickModel(settings, "", "") // 没有可用的模型时为空
	if err := ctx.Err(); err != nil {
		return ModelCatalog{}, err
	}
	return catalog, nil
}

// listModels 列出 providers 里声明的全部模型，按显示名排序，不分大小写，显示名相同的再按引用排。
func listModels(settings *config.Settings) []ModelInfo {
	var models []ModelInfo
	for providerID, provider := range settings.Providers {
		for modelID, metadata := range provider.Models {
			models = append(models, modelInfo(settings, providerID, modelID, metadata))
		}
	}
	slices.SortFunc(models, func(a, b ModelInfo) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.Name, b.Name), cmp.Compare(a.Ref, b.Ref))
	})
	return models
}

// pickModel 为会话选模型：preferred（上次用的）可用就用它，否则用配置的默认模型，再否则用清单里
// 第一个可用的。可用是指已在 providers.*.models 里声明，协议与工具调用都支持。思考档位 thinking
// 新选的模型支持就保留，否则用它的默认档。一个可用的模型都没有时返回错误。
func pickModel(
	settings *config.Settings, preferred string, thinking agent.ThinkingLevel,
) (string, agent.ThinkingLevel, error) {
	models := listModels(settings)
	for _, ref := range []string{preferred, settings.Model} {
		i := slices.IndexFunc(models, func(m ModelInfo) bool { return m.Supported && m.Ref == ref })
		if i >= 0 {
			return ref, fitLevel(models[i], thinking), nil
		}
	}
	if i := slices.IndexFunc(models, func(m ModelInfo) bool { return m.Supported }); i >= 0 {
		return models[i].Ref, fitLevel(models[i], thinking), nil
	}
	return "", "", errors.New(
		"没有可用的模型：请在 ~/.atlas/settings.json 的 providers 里声明模型，并用 model 指定默认模型")
}

// fitLevel 返回 model 支持的思考档位：thinking 在其中就是它，否则是 model 的默认档，没有档位时为空。
func fitLevel(model ModelInfo, thinking agent.ThinkingLevel) agent.ThinkingLevel {
	switch {
	case slices.Contains(model.ThinkingLevels, thinking):
		return thinking
	case len(model.ThinkingLevels) > 0:
		return model.ThinkingLevels[0]
	}
	return ""
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
		Tools:          metadata.Tools == nil || *metadata.Tools,
		ThinkingLevels: usableThinkingLevels(metadata),
	}
	if _, err := settings.Provider(providerID); err != nil {
		info.UnsupportedReason = err.Error()
	} else if provider.WireAPI != config.WireAPIOpenAIResponses {
		info.UnsupportedReason = fmt.Sprintf(
			"provider %q 的协议 %q 尚未实现", providerID, provider.WireAPI)
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
			if level.Valid() && !slices.Contains(levels, level) {
				levels = append(levels, level)
			}
		}
	}
	return levels
}

// modelSetup 是一个模型在会话里用到的全部：流函数与上下文窗口。模型本身以完整引用
// provider/model-id 记在会话的 LoopContext 里。
type modelSetup struct {
	stream agent.StreamFn
	window int64 // 上下文窗口（token），配置里没有则为 0
}

// withModelID 让流函数发请求时用模型 ID：循环交来的是完整引用，provider 已体现在客户端上。
func withModelID(stream agent.StreamFn, id string) agent.StreamFn {
	return func(
		ctx context.Context, req agent.StreamRequest, emit func(agent.Delta),
	) agent.AssistantMessage {
		req.Model = id
		return stream(ctx, req, emit)
	}
}

// modelConfig 校验协议和思考档位、加载凭据并创建客户端，不发起模型请求。
func modelConfig(
	settings *config.Settings, modelRef string, thinkingLevel agent.ThinkingLevel,
) (modelSetup, error) {
	if modelRef == "" {
		return modelSetup{}, fmt.Errorf("请在 ~/.atlas/settings.json 中配置 model")
	}
	provider, modelID, metadata, err := settings.ModelRef(modelRef)
	if err != nil {
		return modelSetup{}, err
	}
	if provider.WireAPI != config.WireAPIOpenAIResponses {
		return modelSetup{}, fmt.Errorf("协议 %q 尚未实现", provider.WireAPI)
	}
	if metadata.Tools != nil && !*metadata.Tools {
		return modelSetup{}, fmt.Errorf("模型 %q 不支持工具调用，无法用于当前 Agent", modelRef)
	}
	if thinkingLevel != "" {
		if !metadata.Reasoning {
			return modelSetup{}, fmt.Errorf("模型 %q 未声明思考能力", modelRef)
		}
		if !slices.Contains(usableThinkingLevels(metadata), thinkingLevel) {
			return modelSetup{}, fmt.Errorf(
				"模型 %q 未声明支持思考档位 %q", modelRef, thinkingLevel)
		}
	}
	providerID, _, _ := config.ParseModel(modelRef)
	dir, err := config.Dir()
	if err != nil {
		return modelSetup{}, err
	}
	key, err := config.APIKey(dir, providerID)
	if err != nil {
		return modelSetup{}, err
	}
	client := openai.NewClient(
		option.WithAPIKey(key),
		option.WithBaseURL(provider.BaseURL),
		option.WithMaxRetries(0),
	)
	stream := llm.OpenAIResponsesStreamFn(client)
	if !metadata.Modalities.AcceptsImages() {
		stream = llm.WithoutImages(stream)
	}
	return modelSetup{stream: withModelID(stream, modelID), window: metadata.ContextWindow}, nil
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
