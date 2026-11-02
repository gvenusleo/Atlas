package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// 重读期间的变化不能被丢弃，定位请求也不能被较早的读取结果消费。
func TestReloadKeepsLaterChangesAndPosition(t *testing.T) {
	p := filepath.Join(t.TempDir(), "example.txt")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("初始内容\n")
	tb := &fileTab{path: p}
	tb.load(readTab(p))
	write("较早的读取结果\n")
	tb.reloading = true
	stale := readTab(p)
	write("最后一次修改\n目标位置\n")
	latest := readTab(p)
	pos := editor.Pos{Line: 1, Col: 2}
	tb.showPosition(pos)
	a := &App{}
	a.syncTab(tb)
	tb.reload(stale, false)
	if tb.ed.Buffer.String() != "初始内容\n" || tb.position == nil {
		t.Fatal("较早的读取结果被应用，或提前消费了定位请求")
	}
	tb.reloading = true
	tb.reload(latest, false)
	if tb.ed.Buffer.String() != latest.text || tb.position != nil || tb.ed.CursorPos() != pos {
		t.Fatalf("后续修改或定位丢失：文本=%q，光标=%+v", tb.ed.Buffer.String(), tb.ed.CursorPos())
	}
}

// 重读期间继续编辑时，后续外部变化与定位都不能覆盖未保存的内容。
func TestReloadKeepsEditsAfterLaterChange(t *testing.T) {
	tb := &fileTab{path: "example.txt"}
	tb.load(tabContent{text: "初始内容\n"})
	tb.reloading = true
	tb.showPosition(editor.Pos{Line: 0, Col: 1})
	a := &App{}
	a.syncTab(tb)
	tb.ed.Insert("用户修改")
	if tb.reload(tabContent{text: "第一次外部修改\n"}, false) {
		t.Fatal("重读期间的后续变化未触发再次读取")
	}
	tb.reloading = true
	if !tb.reload(tabContent{text: "最终外部修改\n"}, false) {
		t.Fatal("最终重读未结束")
	}
	if tb.ed.Buffer.String() != "用户修改初始内容\n" || !tb.dirty() || !tb.changed {
		t.Fatal("未保存的修改丢失，或未提示外部冲突")
	}
	if tb.position != nil || tb.ed.CursorPos() != (editor.Pos{Line: 0, Col: 1}) {
		t.Fatal("保留用户修改后未执行定位")
	}
}
