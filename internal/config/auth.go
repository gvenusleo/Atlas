package config

import (
	"encoding/json/v2"
	"fmt"
	"path/filepath"
)

const authFile = "auth.json"

// APIKey 读取 provider 在 auth.json 里的凭据，没有时报错并说明怎么加。
// 凭据只从这个文件来，不读环境变量，行为不随应用从终端还是 Dock 启动而变。
func APIKey(dir, provider string) (string, error) {
	keys, err := ReadAPIKeys(dir)
	if err != nil {
		return "", err
	}
	if key := keys[provider]; key != "" {
		return key, nil
	}
	return "", fmt.Errorf("未找到 provider %q 的 API key：请在 %s 中添加 %q 条目",
		provider, filepath.Join(dir, authFile), provider)
}

// ReadAPIKeys 读取配置目录中的凭据，文件缺失时返回空表。
func ReadAPIKeys(dir string) (map[string]string, error) {
	keys, _, err := readKeys(filepath.Join(dir, authFile))
	return keys, err
}

func readKeys(path string) (map[string]string, []byte, error) {
	data, err := readOptional(path)
	if err != nil {
		return nil, nil, err
	}
	keys := map[string]string{}
	if data != nil {
		if err := json.Unmarshal(data, &keys); err != nil {
			return nil, nil, fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		if keys == nil {
			keys = map[string]string{}
		}
	}
	return keys, data, nil
}
