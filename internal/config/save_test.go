package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveSettingsRestoresKeyWhenSettingsCannotBeWritten(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("需要可由权限阻止创建文件的目录")
	}
	dir, readOnly := t.TempDir(), t.TempDir()
	settings := filepath.Join(readOnly, SettingsFile)
	original := []byte("{}\n")
	if err := os.WriteFile(settings, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(settings, filepath.Join(dir, SettingsFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, authFile), []byte(`{"service":"fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })
	snapshot, err := SaveSettings(dir, &Settings{}, original, KeyChange{
		Name: "renamed", After: new("fixture"),
	}, KeyChange{Name: "service", Before: new("fixture")})
	if err == nil || snapshot != nil {
		t.Fatalf("不可写的设置被报告为已保存：%v", err)
	}
	keys, err := ReadAPIKeys(dir)
	if err != nil || len(keys) != 1 || keys["service"] != "fixture" {
		t.Fatalf("设置写入失败后没有恢复原凭据：%v", err)
	}
}

func TestSaveSettingsRejectsConflictsBeforeChangingKeys(t *testing.T) {
	for _, kind := range []string{"settings", "same key", "unrelated key"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			settings := filepath.Join(dir, SettingsFile)
			auth := filepath.Join(dir, authFile)
			original := []byte("{}\n")
			oldKey := "old-fixture"
			keyFile := `{"service":"old-fixture"}`
			fileOnDisk := original
			switch kind {
			case "settings":
				fileOnDisk = []byte(" {}") // 相同字节数也必须识别。
			case "same key":
				keyFile = `{"service":"external-fixture"}`
			case "unrelated key":
				keyFile = `{"service":"old-fixture","another":"external-fixture"}`
			}
			if err := os.WriteFile(settings, fileOnDisk, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(auth, []byte(keyFile), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := SaveSettings(dir, &Settings{}, original, KeyChange{
				Name: "service", Before: &oldKey, After: new("new-fixture"),
			})
			expectConflict := kind != "unrelated key"
			if expectConflict && (err == nil || !strings.Contains(err.Error(), "在磁盘上已变更")) ||
				!expectConflict && err != nil {
				t.Fatalf("设置保存结果错误：%v", err)
			}
			if expectConflict {
				got, err := os.ReadFile(auth)
				if err != nil || string(got) != keyFile {
					t.Fatalf("拒绝保存时改动了凭据：%v", err)
				}
				return
			}
			keys, err := ReadAPIKeys(dir)
			if err != nil || keys["service"] != "new-fixture" || keys["another"] != "external-fixture" {
				t.Fatalf("没有合并无关条目的改动：%v", err)
			}
		})
	}
}
