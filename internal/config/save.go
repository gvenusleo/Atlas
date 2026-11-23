package config

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// KeyChange 描述凭据条目的前后状态，nil 表示条目不存在。
type KeyChange struct {
	Name          string
	Before, After *string
}

// SaveSettings 检查设置和要改的凭据，再按新增凭据、设置、清理旧凭据的顺序写入。
// 返回非空快照表示设置已保存，即使清理旧凭据失败。
func SaveSettings(dir string, s *Settings, original []byte, changes ...KeyChange) ([]byte, error) {
	path := filepath.Join(dir, SettingsFile)
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := checkSnapshot(path, original); err != nil {
		return nil, err
	}
	auth := filepath.Join(dir, authFile)
	var install, remove []KeyChange
	if len(changes) != 0 {
		keys, _, err := readKeys(auth)
		if err != nil {
			return nil, err
		}
		if err := checkKeys(keys, changes); err != nil {
			return nil, err
		}
		for _, change := range changes {
			if change.After == nil {
				remove = append(remove, change)
			} else {
				install = append(install, change)
			}
		}
	}
	if err := updateKeys(auth, install); err != nil {
		return nil, err
	}
	perm := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	data, err := writeJSON(path, s, perm, original)
	if err != nil {
		for i := range install {
			install[i].Before, install[i].After = install[i].After, install[i].Before
		}
		if rollback := updateKeys(auth, install); rollback != nil {
			return nil, errors.Join(err, fmt.Errorf("恢复原 API key 失败: %w", rollback))
		}
		return nil, err
	}
	if err := updateKeys(auth, remove); err != nil {
		return data, fmt.Errorf("设置已保存，清理旧 API key 失败: %w", err)
	}
	return data, nil
}

func checkKeys(keys map[string]string, changes []KeyChange) error {
	for _, change := range changes {
		key, exists := keys[change.Name]
		if exists != (change.Before != nil) || exists && key != *change.Before {
			return fmt.Errorf("服务 %q 的 API key 在磁盘上已变更，请重新打开设置页", change.Name)
		}
	}
	return nil
}

func updateKeys(path string, changes []KeyChange) error {
	if len(changes) == 0 {
		return nil
	}
	keys, original, err := readKeys(path)
	if err != nil {
		return err
	}
	if err := checkKeys(keys, changes); err != nil {
		return err
	}
	changed := false
	for _, change := range changes {
		if change.After == nil {
			if change.Before != nil {
				delete(keys, change.Name)
				changed = true
			}
		} else {
			keys[change.Name] = *change.After
			changed = true
		}
	}
	if !changed {
		return nil
	}
	_, err = writeJSON(path, keys, 0o600, original)
	return err
}

func checkSnapshot(path string, original []byte) error {
	data, err := readOptional(path)
	if err != nil {
		return err
	}
	if (data == nil) != (original == nil) || !bytes.Equal(data, original) {
		return fmt.Errorf("%s 在磁盘上已变更，请重新打开设置页", filepath.Base(path))
	}
	return nil
}

// writeJSON 在同目录写临时文件，再核对原始内容并替换。有效的符号链接保留。
func writeJSON(path string, v any, perm os.FileMode, original []byte) ([]byte, error) {
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = real
	}
	data, err := json.Marshal(v, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return nil, fmt.Errorf("编码 %s 失败: %w", path, err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, ".atlas-config-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(perm)
	}
	if err = errors.Join(err, tmp.Close()); err != nil {
		return nil, err
	}
	if err := checkSnapshot(path, original); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nil, err
	}
	return data, nil
}
