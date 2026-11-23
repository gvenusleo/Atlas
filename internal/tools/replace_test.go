package tools

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFailedReplacementKeepsFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing")
	if err := os.WriteFile(path, []byte("原文"), 0o640); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("模拟写入失败")
	for _, name := range []string{"partial write", "cancelled", "external change"} {
		t.Run(name, func(t *testing.T) {
			target, err := inspectTarget(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			expected := "原文"
			err = target.replace(ctx, func(file *os.File) error {
				if _, err := file.WriteString("只写了一半"); err != nil {
					return err
				}
				switch name {
				case "cancelled":
					cancel()
				case "external change":
					expected = "其他编辑器保存的内容"
					return os.WriteFile(path, []byte(expected), 0o640)
				default:
					return failure
				}
				return nil
			})
			if err == nil {
				t.Fatal("替换应当失败")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != expected {
				t.Fatalf("原文件没有保留：%q，%v", data, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("临时文件没有清理：%v，%v", entries, err)
			}
		})
	}
}

func TestWriteAndEditReplaceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	_, err := Write(dir).Execute(t.Context(), "write", jsontext.Value(`{"path":"file.txt","content":"old"}`))
	if err != nil {
		t.Fatal(err)
	}
	name := "file.txt"
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
		name = "link.txt"
		if err := os.Symlink("file.txt", filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	params := `{"path":"` + name + `","edits":[{"oldText":"old","newText":"updated"}]}`
	if _, err := Edit(dir).Execute(t.Context(), "edit", jsontext.Value(params)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "updated" {
		t.Fatalf("替换未写入目标：%q，%v", data, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Lstat(filepath.Join(dir, name)); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("符号链接没有保留：%v，%v", info, err)
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("权限没有保留：%v，%v", info, err)
		}
	}
	if err := os.Link(path, filepath.Join(dir, "hard.txt")); err != nil {
		t.Fatal(err)
	}
	_, err = Write(dir).Execute(t.Context(), "write", jsontext.Value(`{"path":"file.txt","content":"overwrite"}`))
	if err == nil || !strings.Contains(err.Error(), "硬链接") {
		t.Fatalf("替换应当保留硬链接关系：%v", err)
	}
}
