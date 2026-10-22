package gui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// outlineApp 是有 30 轮对话的界面，每轮的回复占好几行，停在末尾。
func outlineApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	a := newApp()
	for i := range 30 {
		a.current.add(itemUser, fmt.Sprintf("提问%02d", i)).seq = int64(2 + 2*i)
		a.current.add(itemAssistant, strings.Repeat("回答。", 60))
	}
	tt := ui.NewTester(a.view, 900, 600)
	tt.Advance(time.Second)
	return a, tt
}

// 点目录里的一条提问，对话滚到它，让它排在视野顶端。
func TestOutlineJumpsToPrompt(t *testing.T) {
	_, tt := outlineApp(t)
	if tt.HasText("提问22") {
		t.Fatalf("起点就能看到提问22，现有文本 %q", tt.Texts())
	}
	if err := tt.Click("目录"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("提问22"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	r, ok := tt.Find("提问22")
	if !ok || r.Y > 150 {
		t.Errorf("提问22 = %v（%v），期望在视野顶端", r, ok)
	}
	if tt.HasText("提问29") {
		t.Errorf("跳转后还看得到末尾的提问29，现有文本 %q", tt.Texts())
	}
}

// 当前这条也要能选：滚过它的开头之后，目录里高亮的还是它，回车选中它，回到它的开头。
func TestOutlineReselectsCurrent(t *testing.T) {
	a, tt := outlineApp(t)
	a.current.list.ScrollTo(44, ui.Start) // 提问22
	tt.Advance(time.Second)
	tt.Scroll(500, 300, 0, 100)
	tt.Advance(time.Second)
	if r, _ := tt.Find("提问22"); r.Y > 60 {
		t.Fatalf("滚动之后提问22 = %v，期望已滚过它的开头", r)
	}
	if err := tt.Click("目录"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)

	if r, ok := tt.Find("提问22"); !ok || r.Y < 60 {
		t.Errorf("提问22 = %v（%v），期望回到视野顶端", r, ok)
	}
}

// 要跳到的提问是按记录序号找的：它加载进来之后，下一帧滚过去，不再等。
func TestSeekScrollsOnceLoaded(t *testing.T) {
	a, tt := outlineApp(t)
	a.current.seekSeq = 2 + 2*10 // 提问10
	tt.Advance(time.Second)

	if a.current.seekSeq != 0 {
		t.Errorf("seekSeq = %d，期望滚过去之后清掉", a.current.seekSeq)
	}
	if r, ok := tt.Find("提问10"); !ok || r.Y > 150 {
		t.Errorf("提问10 = %v（%v），期望在视野顶端", r, ok)
	}
}
