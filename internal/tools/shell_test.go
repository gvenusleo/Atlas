package tools

import (
	"encoding/json/jsontext"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
)

func TestShellTimeoutAndLogRetention(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LocalAppData", dir)
	_, timeout, err := parseShellArgs(jsontext.Value(`{"command":"test"}`))
	if err != nil || timeout != 1800 {
		t.Fatalf("默认命令时限 = %d，%v", timeout, err)
	}
	script := `{"command":"yes done | head -n 3000; sleep 30","timeout":1}`
	if runtime.GOOS == "windows" {
		script = `{"command":"1..3000 | ForEach-Object { 'done' }; Start-Sleep 30","timeout":2}`
	}
	result, err := Shell(dir).Execute(t.Context(), "run", jsontext.Value(script))
	if err != nil || !result.IsError {
		t.Fatalf("命令未因超时报告错误：%v", err)
	}
	text := agent.TextOf(result.Content)
	_, after, ok := strings.Cut(text, "完整输出: ")
	path, _, _ := strings.Cut(after, "\n")
	if !ok || !strings.Contains(text, "已终止") {
		t.Fatalf("没有返回超时或完整日志：%q", text)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "done") != 3000 {
		t.Fatalf("完整日志长度错误：%d 字节，%v", len(data), err)
	}
	if err := expireShellLogs(t, path); err != nil {
		t.Fatal(err)
	}
}

func expireShellLogs(t *testing.T, path string) error {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	old := now.Add(-shellLogRetention)
	if err := os.Chtimes(path, old, old); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	for name, modified := range map[string]time.Time{
		"atlas-shell-active.running": old,
		"atlas-shell-recent.log":     old.Add(time.Second),
	} {
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte("输出"), 0o600); err != nil {
			return err
		}
		if err := os.Chtimes(file, modified, modified); err != nil {
			return err
		}
	}
	if err := pruneShellLogs(dir, now); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 || entries[0].Name() != "atlas-shell-active.running" ||
		entries[1].Name() != "atlas-shell-recent.log" {
		t.Fatalf("日志清理范围错误：%v，%v", entries, err)
	}
	return nil
}
