package tools

import (
	"encoding/json/jsontext"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteKeepsFileOpenWithoutDeleteSharing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	_, err = Write(dir).Execute(t.Context(), "write", jsontext.Value(`{"path":"file.txt","content":"new"}`))
	if err == nil {
		t.Fatal("不允许删除共享的目标应拒绝替换")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatalf("替换失败后未保留原件：%q，%v", data, err)
	}
}
