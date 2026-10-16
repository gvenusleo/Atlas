package tools

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// read 的结果附带实际返回的窗口（起止行与总行数），界面靠它显示真实行号；
// 默认值与读取窗口都要计入，而不是照请求的参数。
func TestReadDetails(t *testing.T) {
	dir := t.TempDir()
	lines := strings.Repeat("x\n", 10)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	for params, want := range map[string]ReadDetails{
		`{"path":"a.txt"}`:                       {Start: 1, End: 10, Total: 10},
		`{"path":"a.txt","offset":3,"limit":4}`:  {Start: 3, End: 6, Total: 10},
		`{"path":"a.txt","offset":9,"limit":50}`: {Start: 9, End: 10, Total: 10},
	} {
		result, err := readFile(t.Context(), dir, jsontext.Value(params))
		if err != nil {
			t.Fatal(err)
		}
		var got ReadDetails
		if err := json.Unmarshal(result.Details, &got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s: 窗口 = %+v，期望 %+v", params, got, want)
		}
	}
}
