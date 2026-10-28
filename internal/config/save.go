package config

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Save 校验 s 并写进 dir 下的 settings.json。键的顺序固定、带缩进，手工改过的文件重写后也不会
// 来回变动；结构里没有的键不会保留。文件已经存在时保持它的权限。
func Save(dir string, s *Settings) error {
	path := filepath.Join(dir, SettingsFile)
	if err := s.Validate(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	perm := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	return writeJSON(path, s, perm)
}

// SetAPIKey 把 provider 的凭据写进 dir 下的 auth.json，其余条目保持不变。文件不存在就新建；
// 内容解析不了时不改，免得盖掉用户的东西。写出的文件权限固定为 0600。
func SetAPIKey(dir, provider, key string) error {
	return editKeys(dir, func(keys map[string]string) { keys[provider] = key })
}

// RemoveAPIKey 从 auth.json 里删掉 provider 的凭据；没有这个条目或文件时什么也不做。
func RemoveAPIKey(dir, provider string) error {
	return editKeys(dir, func(keys map[string]string) { delete(keys, provider) })
}

// editKeys 读出 auth.json 的全部条目，交给 edit 改，再写回去。
func editKeys(dir string, edit func(map[string]string)) error {
	path := filepath.Join(dir, authFile)
	keys := map[string]string{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("读取 %s 失败: %w", path, err)
	default:
		if err := json.Unmarshal(data, &keys); err != nil {
			return fmt.Errorf("解析 %s 失败: %w", path, err)
		}
	}
	edit(keys)
	return writeJSON(path, keys, 0o600)
}

// writeJSON 把 v 写成带缩进的 JSON：先写同目录的临时文件再改名，读者不会看到写了一半的文件；
// path 是符号链接时写它指向的文件，链接本身保留。
func writeJSON(path string, v any, perm os.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	data, err := json.Marshal(v, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("编码 %s 失败: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".atlas-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // 改名成功之后它已经不在了
	_, err = tmp.Write(append(data, '\n'))
	if err == nil {
		err = tmp.Chmod(perm)
	}
	if err = errors.Join(err, tmp.Close()); err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	return err
}
