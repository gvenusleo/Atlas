package config

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirName      = ".atlas"
	settingsFile = "settings.json"
)

// Dir 返回配置目录 ~/.atlas。
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("定位主目录失败: %w", err)
	}
	return filepath.Join(home, dirName), nil
}

// Load 读取 ~/.atlas/settings.json，等价于 LoadFrom(Dir())。
func Load() (*Settings, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	return LoadFrom(dir)
}

// LoadFrom 读取 dir 下的 settings.json 并校验。文件缺失视为零配置；
// 解析与校验错误必须带文件路径。
func LoadFrom(dir string) (*Settings, error) {
	path := filepath.Join(dir, settingsFile)
	s := &Settings{}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Validate 校验格式类问题：model 两段式、api 取值合法。
func (s *Settings) Validate() error {
	if s.Model != "" {
		if _, _, err := ParseModel(s.Model); err != nil {
			return err
		}
	}
	for name, p := range s.Providers {
		switch p.WireAPI {
		case "", WireAPIOpenAIResponses, WireAPIOpenAICompletions:
		default:
			return fmt.Errorf("providers.%s.api = %q 无效，仅支持 %s / %s", name, p.WireAPI, WireAPIOpenAIResponses, WireAPIOpenAICompletions)
		}
	}
	return nil
}

// ParseModel 把 "provider/model-id" 拆为两段；模型 ID 允许再含 "/"（如 org/name）。
func ParseModel(ref string) (provider, id string, err error) {
	provider, id, ok := strings.Cut(ref, "/")
	if !ok || provider == "" || id == "" {
		return "", "", fmt.Errorf("模型引用 %q 必须是 provider/model-id 形式", ref)
	}
	return provider, id, nil
}

// Provider 返回 id 对应的接入参数。api 与 base_url 均须显式配置，缺失时报错指出位置。
func (s *Settings) Provider(id string) (Provider, error) {
	p := s.Providers[id]
	var missing []string
	if p.WireAPI == "" {
		missing = append(missing, "api")
	}
	if p.BaseURL == "" {
		missing = append(missing, "base_url")
	}
	if len(missing) > 0 {
		return Provider{}, fmt.Errorf("provider %q 缺少 %s，请在 settings.json 的 providers.%s 中配置",
			id, strings.Join(missing, " 与 "), id)
	}
	return p, nil
}

// ModelRef 把 "provider/model-id" 解析为接入参数、模型 ID 与其元数据。
// 元数据查 providers.<id>.models；查不到时返回零值 Model（不报错）。
func (s *Settings) ModelRef(ref string) (Provider, string, Model, error) {
	provider, id, err := ParseModel(ref)
	if err != nil {
		return Provider{}, "", Model{}, err
	}
	p, err := s.Provider(provider)
	if err != nil {
		return Provider{}, "", Model{}, err
	}
	return p, id, p.Models[id], nil
}
