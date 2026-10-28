package config

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const authFile = "auth.json"

// APIKey 读取 provider 在 auth.json 里的凭据，没有时报错并说明怎么加。
// 凭据只从这个文件来，不读环境变量，行为不随应用从终端还是 Dock 启动而变。
func APIKey(dir, provider string) (string, error) {
	path := filepath.Join(dir, authFile)
	data, err := os.ReadFile(path)
	if err == nil {
		var keys map[string]string
		if err := json.Unmarshal(data, &keys); err != nil {
			return "", fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		if key := keys[provider]; key != "" {
			return key, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	return "", fmt.Errorf("未找到 provider %q 的 API key：请在 %s 中添加 %q 条目",
		provider, path, provider)
}
