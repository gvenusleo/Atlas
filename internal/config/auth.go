package config

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const authFile = "auth.json"

// APIKey 解析 provider 的凭据：auth.json（仅允许属主可读写）优先，
// 其次环境变量 <PROVIDER 大写>_API_KEY。全无时报错并列出两条路。
func APIKey(dir, provider string, getenv func(string) string) (string, error) {
	envName := strings.ToUpper(provider) + "_API_KEY"
	path := filepath.Join(dir, authFile)
	if err := checkPerm(path); err != nil {
		return "", err
	}
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
	if key := getenv(envName); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("未找到 provider %q 的 API key：可在 %s 中添加 %q 条目，或设置环境变量 %s",
		provider, path, provider, envName)
}

// checkPerm 拒绝属主之外有任何权限位的 auth.json（0600/0400 均可）。Windows 无 POSIX 权限位，跳过。
func checkPerm(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil // 文件不存在等情形交给后续读取处理。
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%s 权限过宽（%o），请执行: chmod 600 %s", path, perm, path)
	}
	return nil
}
