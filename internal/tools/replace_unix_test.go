//go:build darwin || linux

package tools

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestToolReplacementPreservesExtendedAttributes(t *testing.T) {
	const name = "user.atlas-test"
	value := []byte{0, 1, 255}
	for _, tool := range []string{"write", "edit"} {
		t.Run(tool, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "note.txt")
			if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
				t.Fatal(err)
			}
			if err := unix.Setxattr(path, name, value, 0); errors.Is(err, unix.ENOTSUP) {
				t.Skip("测试文件系统不支持扩展属性")
			} else if err != nil {
				t.Fatal(err)
			}
			if tool == "edit" {
				params := jsontext.Value(`{"path":"note.txt","edits":[{"oldText":"old","newText":"new"}]}`)
				if _, err := Edit(dir).Execute(t.Context(), "edit", params); err != nil {
					t.Fatal(err)
				}
			} else if _, err := Write(dir).Execute(t.Context(), "write",
				jsontext.Value(`{"path":"note.txt","content":"new"}`)); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, len(value))
			if n, err := unix.Getxattr(path, name, got); err != nil || !bytes.Equal(got[:n], value) {
				t.Fatalf("扩展属性没有保留：%v，%v", got, err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "new" {
				t.Fatalf("目标内容没有更新：%q，%v", got, err)
			}
		})
	}
}
