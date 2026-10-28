package core

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/llm"
	"github.com/gvenusleo/atlas/internal/protocol"
)

// modelInfo 是配置里声明的一个模型。可用（Supported）是指协议已实现、支持工具调用。
type modelInfo struct {
	Ref            string
	Name           string
	Images         bool
	ThinkingLevels []agent.ThinkingLevel
	Supported      bool
}

// listModels 列出 providers 里声明的全部模型，按显示名排序，不分大小写，显示名相同的再按引用排。
func listModels(settings *config.Settings) []modelInfo {
	var models []modelInfo
	for providerID, provider := range settings.Providers {
		_, err := settings.Provider(providerID)
		for modelID, metadata := range provider.Models {
			models = append(models, modelInfo{
				Ref:            providerID + "/" + modelID,
				Name:           cmp.Or(metadata.Name, modelID),
				Images:         metadata.Modalities.AcceptsImages(),
				ThinkingLevels: usableThinkingLevels(metadata),
				Supported:      err == nil && llm.Supports(string(provider.WireAPI)),
			})
		}
	}
	slices.SortFunc(models, func(a, b modelInfo) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.Name, b.Name), cmp.Compare(a.Ref, b.Ref))
	})
	return models
}

// pickModel 为会话选模型：preferred（上次用的）可用就用它，否则用配置的默认模型，再否则用清单里
// 第一个可用的。思考档位 thinking 新选的模型支持就保留，否则用它的默认档。一个可用的模型都没有时
// 返回错误。
func pickModel(
	settings *config.Settings, preferred string, thinking agent.ThinkingLevel,
) (string, agent.ThinkingLevel, error) {
	models := listModels(settings)
	for _, ref := range []string{preferred, settings.Model} {
		i := slices.IndexFunc(models, func(m modelInfo) bool { return m.Supported && m.Ref == ref })
		if i >= 0 {
			return ref, fitLevel(models[i], thinking), nil
		}
	}
	if i := slices.IndexFunc(models, func(m modelInfo) bool { return m.Supported }); i >= 0 {
		return models[i].Ref, fitLevel(models[i], thinking), nil
	}
	return "", "", errors.New(
		"没有可用的模型：请在 ~/.atlas/settings.json 的 providers 里声明模型，并用 model 指定默认模型")
}

// chooseModel 选出会话该用的模型并配好（见 pickModel）。一个可用的模型都没有时 ref 为空，配不起来
// （缺凭据等）时 model 只有引用，都不算失败：会话照常打开，要运行时再试一次并报原因。
func chooseModel(
	settings *config.Settings, preferred string, thinking agent.ThinkingLevel,
) (agent.ThinkingLevel, agent.Model) {
	ref, level, err := pickModel(settings, preferred, thinking)
	if err != nil {
		return thinking, agent.Model{Ref: preferred}
	}
	model, err := modelConfig(settings, ref, level)
	if err != nil {
		return level, agent.Model{Ref: ref}
	}
	return level, model
}

// fitLevel 返回 model 支持的思考档位：thinking 在其中就是它，否则是 model 的默认档，没有档位时为空。
func fitLevel(model modelInfo, thinking agent.ThinkingLevel) agent.ThinkingLevel {
	switch {
	case slices.Contains(model.ThinkingLevels, thinking):
		return thinking
	case len(model.ThinkingLevels) > 0:
		return model.ThinkingLevels[0]
	}
	return ""
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

// configOptions 是会话的配置项：模型，以及当前模型支持思考时的思考深度。
func configOptions(
	settings *config.Settings, ref string, thinking agent.ThinkingLevel,
) []protocol.ConfigOption {
	model := protocol.ConfigOption{ID: protocol.ConfigModel, Name: "模型", CurrentValue: ref}
	var levels []agent.ThinkingLevel
	for _, m := range listModels(settings) {
		if !m.Supported {
			continue
		}
		model.Options = append(model.Options, protocol.ConfigValue{
			Value: m.Ref, Name: m.Name, Images: m.Images,
		})
		if m.Ref == ref {
			levels = m.ThinkingLevels
		}
	}
	options := []protocol.ConfigOption{model}
	if len(levels) > 0 {
		level := protocol.ConfigOption{ID: protocol.ConfigThinking, Name: "思考深度",
			CurrentValue: string(thinking)}
		for _, l := range levels {
			name := strings.ToUpper(string(l[:1])) + string(l[1:])
			level.Options = append(level.Options,
				protocol.ConfigValue{Value: string(l), Name: name})
		}
		options = append(options, level)
	}
	return options
}

// modelConfig 校验协议和思考档位、加载凭据并创建客户端，不发起模型请求。
func modelConfig(
	settings *config.Settings, modelRef string, thinkingLevel agent.ThinkingLevel,
) (agent.Model, error) {
	if modelRef == "" {
		return agent.Model{}, fmt.Errorf("请在 ~/.atlas/settings.json 中配置 model")
	}
	provider, modelID, metadata, err := settings.ModelRef(modelRef)
	if err != nil {
		return agent.Model{}, err
	}
	if !llm.Supports(string(provider.WireAPI)) {
		return agent.Model{}, fmt.Errorf("协议 %q 尚未实现", provider.WireAPI)
	}
	if thinkingLevel != "" && !slices.Contains(usableThinkingLevels(metadata), thinkingLevel) {
		return agent.Model{}, fmt.Errorf("模型 %q 未声明支持思考档位 %q", modelRef, thinkingLevel)
	}
	providerID, _, _ := config.ParseModel(modelRef)
	dir, err := config.Dir()
	if err != nil {
		return agent.Model{}, err
	}
	key, err := config.APIKey(dir, providerID)
	if err != nil {
		return agent.Model{}, err
	}
	stream, err := llm.NewStream(string(provider.WireAPI), provider.BaseURL, key, modelID,
		metadata.Modalities.AcceptsImages(), metadata.MaxOutput)
	if err != nil {
		return agent.Model{}, err
	}
	return agent.Model{Ref: modelRef, ContextWindow: metadata.ContextWindow, Stream: stream}, nil
}

// checkCwd 要求工作目录是仍然存在的绝对目录。
func checkCwd(cwd string) error {
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

// setConfig 换模型或思考深度：配好新模型，保存，再采用；返回完整的配置项，并推送给所有订阅者。
func (s *session) setConfig(
	ctx context.Context, id, value string,
) ([]protocol.ConfigOption, error) {
	end, err := s.begin(nil)
	if err != nil {
		return nil, err
	}
	defer end()
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	ref, thinking, saved := s.model.Ref, s.thinking, s.saved
	s.mu.Unlock()
	switch id {
	case protocol.ConfigModel:
		ref = value
		models := listModels(settings)
		if i := slices.IndexFunc(models, func(m modelInfo) bool { return m.Ref == ref }); i >= 0 {
			thinking = fitLevel(models[i], thinking)
		}
	case protocol.ConfigThinking:
		thinking = agent.ThinkingLevel(value)
	default:
		return nil, errors.New("未知的配置项 " + id)
	}
	model, err := modelConfig(settings, ref, thinking)
	if err != nil {
		return nil, err
	}
	if saved {
		if err := s.core.db.SetModel(ctx, s.id, ref, string(thinking)); err != nil {
			return nil, mapStoreError(err)
		}
	}
	s.mu.Lock()
	s.model, s.thinking = model, thinking
	usage := s.usage()
	s.mu.Unlock()
	options := configOptions(settings, ref, thinking)
	s.broadcast(protocol.ConfigOptionUpdate{ConfigOptions: options},
		protocol.UsageUpdate{Usage: usage})
	return options, nil
}

// options 读设置，给出会话现在的配置项。
func (s *session) options() ([]protocol.ConfigOption, error) {
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return configOptions(settings, s.model.Ref, s.thinking), nil
}

// ensureModel 在会话没有可用的模型时再选一次（设置可能已经改好了）；仍然选不出就返回原因。
func (s *session) ensureModel(ctx context.Context) error {
	s.mu.Lock()
	ref, thinking, ok, saved := s.model.Ref, s.thinking, s.model.Stream != nil, s.saved
	s.mu.Unlock()
	if ok {
		return nil
	}
	settings, err := config.Load()
	if err != nil {
		return err
	}
	if ref, thinking, err = pickModel(settings, ref, thinking); err != nil {
		return err
	}
	model, err := modelConfig(settings, ref, thinking)
	if err != nil {
		return err
	}
	if saved {
		if err := s.core.db.SetModel(ctx, s.id, ref, string(thinking)); err != nil {
			return mapStoreError(err)
		}
	}
	s.mu.Lock()
	s.model, s.thinking = model, thinking
	s.mu.Unlock()
	s.broadcast(protocol.ConfigOptionUpdate{ConfigOptions: configOptions(settings, ref, thinking)})
	return nil
}
