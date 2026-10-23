// Package gui 是 Atlas 的桌面入口，也是核心的一个客户端：它只通过 protocol.Service 使用核心，
// 会话的状态一律来自核心的响应与更新（见 chat），自己只存界面状态。
//
// 界面状态只在主线程读写：后台 goroutine 调用核心，结果与更新经 Window.Update 回到主线程。
// 多个会话可以同时运行，每个会话有独立的 chat 状态，也有自己的终端标签：它们随会话保留，
// 在会话被删除、丢弃或应用退出时关闭。
package gui

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/shellenv"
)

const (
	// historyPage 是每次从历史中加载的记录条数：打开会话时取最近一页，向上滚时再往前取。
	historyPage = 200
	// olderNear 是列表顶部离可见区域还有几行时开始加载更早的记录。
	olderNear = 5
	// closeTimeout 限制退出时等待运行收尾的时间。
	closeTimeout = 5 * time.Second
)

// App 持有桌面端的全部界面状态。
type App struct {
	service       protocol.Service
	win           *mygo.Window
	infos         []protocol.SessionInfo // 侧栏会话，按更新时间倒序
	chats         map[string]*chat       // 已打开的会话，按会话 ID
	current       *chat
	cwd           string // 新会话默认的工作目录
	keys          int
	sidebarWidth  float32
	shown         float32 // 侧栏展开程度：1 完全展开，0 完全收起，动画中取中间值
	md            markdown.Options
	mdNote        markdown.Options // 面板里的小号 Markdown，如压缩的摘要
	mdFile        markdown.Options // 文件面板里的 Markdown 预览
	keyNav        bool             // 侧栏正在用键盘操作：切换会话时输入框不抢走焦点
	query         string
	sections      map[string]*bool // 各目录分组是否展开，缺省为展开
	more          map[string]bool  // 已点“显示更多”的目录分组
	sideWidth     float32          // 右侧栏的宽度，终端与文件面板共用
	termHeight    float32          // 终端与文件面板叠放时，终端卡片的高度；0 时各占一半
	shells        int              // 已创建的终端数，用作终端标签的标识
	sidebarHidden bool
	err           string
	toast         string // 待显示的一次性提示，下一帧弹出后清空

	// 重命名与删除对话框的状态。
	renameID, renameText string
	renameOpen           bool
	deleteID             string
	deleteOpen           bool

	unsaved   unsavedAsk // 关闭有未保存修改的文件标签时的确认
	discardOK bool       // 用户确认了丢弃未保存的修改，再退出一次就不再问
}

// Run 打开窗口，连接核心，阻塞到应用退出；退出时用 shutdown 关闭核心。
func Run(
	connect func(protocol.Subscriber) protocol.Service, shutdown func(context.Context) error,
) error {
	// 界面是中文的：控件自带的文字（搜索框、编辑菜单等）也用中文。
	mygo.App.SetLanguages("zh-Hans")
	// 从 Dock 启动时没有 shell 配置里的环境变量，后台向登录 shell 要一份给子进程用。
	shellenv.Warm()
	a := newApp()
	a.service = connect(a)
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) { a.confirmQuit(&e.Preventable) })
	mygo.App.OnQuit(func() {
		a.closeAllPanels()
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			log.Printf("关闭核心失败: %v", err)
		}
	})
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Atlas",
			Width:     1100,
			Height:    760,
			MinWidth:  800,
			MinHeight: 420,
			StateKey:  "main",
			// 隐藏标题栏并固定红绿灯位置：不带工具栏，标题栏区域低而紧凑，
			// c.TitleBar().Height 即红绿灯所在的带，开关按钮据此与它们居中对齐。
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 16, Y: 16},
			Content:              ui.View(a.view),
		})
		a.win.OnClose(func(e *mygo.CloseEvent) { a.confirmQuit(&e.Preventable) })
		a.refresh()
		a.start(a.current)
	})
	return mygo.App.Run()
}

// newApp 创建应用状态，打开一个空白的新会话。
func newApp() *App {
	a := &App{chats: map[string]*chat{}, sidebarWidth: sidebarDefault, shown: 1,
		sections: map[string]*bool{}, more: map[string]bool{}, sideWidth: sideDefault}
	// 按词显示以空格为界，中文没有空格，一段会整个冒出来；按字符才是匀速的。
	a.md.Reveal = &markdown.Reveal{Unit: markdown.ByCharacter}
	a.md.Theme = &markdown.Theme{
		FontSize:     bodySize,
		CodeFont:     monoFamily,
		CodeSize:     float32(codeSize) / bodySize,
		LineHeight:   1.6,
		BlockSpacing: 0.7,
		Headings: [6]markdown.HeadingStyle{
			{Size: 1.55, Weight: 600, Rule: true},
			{Size: 1.35, Weight: 600, Rule: true},
			{Size: 1.15, Weight: 600},
			{Size: 1.0, Weight: 600},
			{Size: 0.95, Weight: 600},
			{Size: 0.9, Weight: 600},
		},
	}
	// 面板里的 Markdown 正文与面板里的其他字一样大，行距与间距更紧凑。各处的代码都是
	// codeSize，所以 CodeSize 按各自的正文字号折算。
	note := markdown.Theme{
		FontSize:     textSize,
		CodeFont:     monoFamily,
		CodeSize:     float32(codeSize) / textSize,
		LineHeight:   1.5,
		BlockSpacing: 0.5,
		Headings: [6]markdown.HeadingStyle{
			{Size: 1.25, Weight: 600}, {Size: 1.15, Weight: 600}, {Size: 1.05, Weight: 600},
			{Size: 1, Weight: 600}, {Size: 1, Weight: 600}, {Size: 1, Weight: 600},
		},
	}
	a.mdNote.Theme = &note
	// 文件预览是整篇文档，标题用对话的；H5、H6 与 H4 同大，免得小于 smallSize。
	file := note
	file.Headings = a.md.Theme.Headings
	file.Headings[4], file.Headings[5] = file.Headings[3], file.Headings[3]
	a.mdFile.Theme = &file
	a.cwd, _ = os.UserHomeDir()
	a.newChat()
	return a
}

// Receive 接收核心推来的更新，交给主线程应用到对应的会话。
func (a *App) Receive(n protocol.Notification) {
	a.win.Update(func() {
		ch := a.chats[n.SessionID]
		if ch == nil {
			return
		}
		switch u := n.Update.(type) {
		case protocol.SessionInfoUpdate:
			ch.apply(u)
			a.refresh()
		case protocol.RunStateUpdate:
			ch.apply(u)
			if !u.Running {
				// 在别的会话时结束的运行，回到这个会话之前侧栏上标为未读。
				ch.unread = ch != a.current && ch.err == ""
				a.refresh()
			}
		default:
			ch.apply(u)
		}
	})
}

// newChat 切换到一个新会话：先是空白的，会话在后台建好才能发送。
func (a *App) newChat() {
	if !a.leave() {
		return
	}
	a.keys++
	a.current = &chat{key: "chat-" + strconv.Itoa(a.keys), cwd: a.cwd}
	a.start(a.current)
}

// toggleSidebar 显示或隐藏侧栏。
func (a *App) toggleSidebar() {
	a.sidebarHidden = !a.sidebarHidden
}
