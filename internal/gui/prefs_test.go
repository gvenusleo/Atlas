package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

// 偏好文件缺失、损坏时用默认值，主题不认识时只重置主题；写进去再读出来是同样的。
func TestPrefsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, prefsFile)
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if got := loadPrefs(dir); got != (prefs{}) {
		t.Errorf("文件缺失时 = %+v，期望默认值", got)
	}
	write("{坏的")
	if got := loadPrefs(dir); got != (prefs{}) {
		t.Errorf("文件损坏时 = %+v，期望默认值", got)
	}
	write(`{"theme": "sepia", "code_font": "Menlo"}`)
	if got := loadPrefs(dir); got != (prefs{CodeFont: "Menlo"}) {
		t.Errorf("主题不认识时 = %+v，期望只有代码字体", got)
	}

	want := prefs{Theme: mygo.ThemeDark, UIFont: "Inter", CodeFont: "Menlo", HideTray: true}
	if err := savePrefs(dir, want); err != nil {
		t.Fatal(err)
	}
	if got := loadPrefs(dir); got != want {
		t.Errorf("读回 = %+v，期望 %+v", got, want)
	}
}
