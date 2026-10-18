// Package gui 是 Atlas 的桌面入口：用 MyGo 原生界面展示 host 管理的会话。
//
// 界面状态只在主线程读写：后台 goroutine 调用 host，结果与事件经
// Window.Update 回到主线程。多个会话可以同时运行，每个会话有独立的 chat 状态，
// 也有自己的终端标签：它们随会话保留，在会话被删除、丢弃或应用退出时关闭。
package gui

import (
	"cmp"
	"context"
	"errors"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/host"
)

const (
	// historyPage 是每次从历史中加载的记录条数：打开会话时取最近一页，向上滚时再往前取。
	historyPage = 200
	// olderNear 是列表顶部离可见区域还有几行时开始加载更早的记录。
	olderNear = 5
	// listLimit 是侧栏显示的会话数量。
	listLimit = 100
	// closeTimeout 限制退出时等待运行收尾的时间。
	closeTimeout = 5 * time.Second
)

// App 持有桌面端的全部界面状态。
type App struct {
	host          *host.Manager
	win           *mygo.Window
	infos         []host.Info      // 侧栏会话，按更新时间倒序
	chats         map[string]*chat // 已打开的会话
	current       *chat
	cwd           string           // 新会话默认的工作目录
	models        []host.ModelInfo // 可用的模型，来自配置
	defaultModel  string
	keys          int
	sidebarWidth  float32
	shown         float32 // 侧栏展开程度：1 完全展开，0 完全收起，动画中取中间值
	md            markdown.Options
	mdNote        markdown.Options // 面板里的小号 Markdown，如压缩的摘要
	keyNav        bool             // 侧栏正在用键盘操作：切换会话时输入框不抢走焦点
	query         string
	sections      map[string]*bool // 各目录分组是否展开，缺省为展开
	more          map[string]bool  // 已点“显示更多”的目录分组
	termWidth     float32
	shells        int // 已创建的终端数，用作终端标签的标识
	sidebarHidden bool
	err           string
	toast         string // 待显示的一次性提示，下一帧弹出后清空

	// 重命名与删除对话框的状态。
	renameID, renameText string
	renameOpen           bool
	deleteID             string
	deleteOpen           bool
}

// Run 打开宿主与窗口，阻塞到应用退出。
func Run() error {
	// 界面是中文的：控件自带的文字（搜索框、编辑菜单等）也用中文。
	mygo.App.SetLanguages("zh-Hans")
	h, err := host.Open(context.Background(), "")
	if err != nil {
		return err
	}
	a := newApp(h)
	mygo.App.OnQuit(func() {
		a.closeAllTerminals()
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		if err := h.Close(ctx); err != nil {
			log.Printf("关闭宿主失败: %v", err)
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
		a.refresh()
		a.loadModels()
	})
	return mygo.App.Run()
}

// newApp 创建应用状态，打开一个空白的新会话。
func newApp(h *host.Manager) *App {
	a := &App{host: h, chats: map[string]*chat{}, sidebarWidth: sidebarDefault, shown: 1,
		sections: map[string]*bool{}, more: map[string]bool{}, termWidth: termDefault}
	// 按词显示以空格为界，中文没有空格，一段会整个冒出来；按字符才是匀速的。
	a.md.Reveal = &markdown.Reveal{Unit: markdown.ByCharacter}
	a.md.Theme = &markdown.Theme{
		FontSize:     bodySize,
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
	a.mdNote.Theme = &markdown.Theme{
		FontSize:     13,
		LineHeight:   1.5,
		BlockSpacing: 0.5,
		Headings: [6]markdown.HeadingStyle{
			{Size: 1.15, Weight: 600}, {Size: 1.1, Weight: 600}, {Size: 1.05, Weight: 600},
			{Size: 1, Weight: 600}, {Size: 1, Weight: 600}, {Size: 1, Weight: 600},
		},
	}
	a.cwd, _ = os.UserHomeDir()
	a.newChat()
	return a
}

// newChat 切换到一个尚未创建的新会话。
func (a *App) newChat() {
	a.leave()
	a.keys++
	a.current = &chat{key: "chat-" + strconv.Itoa(a.keys), cwd: a.cwd}
	a.useDefaultModel(a.current)
}

// toggleSidebar 显示或隐藏侧栏。
func (a *App) toggleSidebar() {
	a.sidebarHidden = !a.sidebarHidden
}

// open 切换到已有会话，首次打开时加载历史；查看历史不需要凭据。
func (a *App) open(id string) {
	a.leave()
	if ch := a.chats[id]; ch != nil {
		a.current, ch.unread = ch, false
		return
	}
	a.keys++
	ch := &chat{key: "chat-" + strconv.Itoa(a.keys), id: id, loading: true}
	for _, info := range a.infos {
		if info.ID == id {
			ch.cwd, ch.model, ch.thinking = info.Cwd, info.ModelRef, info.ThinkingLevel
		}
	}
	a.chats[id], a.current = ch, ch
	go func() {
		records, err := a.host.History(context.Background(), id, 0, historyPage)
		a.win.Update(func() {
			ch.loading = false
			if err != nil {
				ch.err = err.Error()
				return
			}
			ch.load(records)
		})
	}()
}

// loadOlder 在列表滚近顶部时再取前面一页记录；没有更早的记录或正在取时什么也不做。
// 列表的行有 Key，往前加入时当前看的位置不动。
func (a *App) loadOlder(ch *chat) {
	if first, last := ch.list.Visible(); last < first || first >= olderNear {
		return
	}
	if ch.loading || ch.paging || ch.oldest <= 1 {
		return
	}
	ch.paging = true
	before := ch.oldest
	go func() {
		records, err := a.host.History(context.Background(), ch.id, before, historyPage)
		a.win.Update(func() {
			ch.paging = false
			if err != nil {
				ch.err, ch.oldest = err.Error(), 0 // 不再重试
				return
			}
			ch.prepend(records)
		})
	}()
}

// refresh 重新读取侧栏的会话列表。
func (a *App) refresh() {
	go func() {
		infos, err := a.host.List(context.Background(), "", nil, listLimit)
		a.win.Update(func() {
			if err != nil {
				a.err = err.Error()
				return
			}
			a.err, a.infos = "", infos
		})
	}()
}

// loadModels 读取可用的模型清单，并为尚未选择模型的空白会话填入默认值。
func (a *App) loadModels() {
	go func() {
		catalog, err := a.host.Models(context.Background())
		a.win.Update(func() {
			if err != nil {
				a.err = err.Error()
				return
			}
			a.models = slices.DeleteFunc(catalog.Models, func(m host.ModelInfo) bool {
				return !m.Supported
			})
			a.defaultModel = catalog.DefaultModel
			a.useDefaultModel(a.current)
		})
	}()
}

// modelInfo 返回清单中的模型；清单未加载或模型不在其中时 ok 为 false。
func (a *App) modelInfo(ref string) (host.ModelInfo, bool) {
	i := slices.IndexFunc(a.models, func(m host.ModelInfo) bool { return m.Ref == ref })
	if i < 0 {
		return host.ModelInfo{}, false
	}
	return a.models[i], true
}

// useDefaultModel 让尚未创建且没有选择模型的会话采用默认模型。
func (a *App) useDefaultModel(ch *chat) {
	if ch.id == "" && ch.model == "" {
		ch.model = a.defaultModel
		a.fitThinking(ch)
	}
}

// fitThinking 保证思考深度属于当前模型支持的档位，否则改用该模型的默认档位。
func (a *App) fitThinking(ch *chat) {
	var levels []agent.ThinkingLevel
	if info, ok := a.modelInfo(ch.model); ok {
		levels = info.ThinkingLevels
	}
	if !slices.Contains(levels, ch.thinking) {
		ch.thinking = ""
		if len(levels) > 0 {
			ch.thinking = levels[0]
		}
	}
}

// job 是会话上的一项后台操作：一轮对话或一次压缩。
type job func(ctx context.Context, s *host.Session, emit func(agent.Event)) error

// say 返回让会话回复 message 的操作。
func say(message agent.UserMessage) job {
	return func(ctx context.Context, s *host.Session, emit func(agent.Event)) error {
		_, err := s.Run(ctx, message, emit)
		return err
	}
}

// compactSession 是压缩会话上下文的操作。
func compactSession(ctx context.Context, s *host.Session, emit func(agent.Event)) error {
	_, err := s.Compact(ctx, emit)
	return err
}

// send 受理输入框里的文本：压缩命令压缩当前会话，其余在后台运行一轮对话；返回是否已受理。
func (a *App) send(ch *chat, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || ch.running || ch.loading {
		return false
	}
	switch {
	case text != compactCommand:
		message := agent.UserMessage{Content: []agent.Content{agent.TextContent{Text: text}}}
		a.launch(ch, say(message))
	case ch.id == "":
		a.toast = host.ErrNothingToCompact.Error() // 新会话还没有历史，不为它创建会话
	default:
		a.launch(ch, compactSession)
	}
	return true
}

// launch 在后台获取会话、采用选定的模型并执行 run，结束后更新界面。
func (a *App) launch(ch *chat, run job) {
	ch.running, ch.err = true, ""
	session, id, cwd := ch.session, ch.id, ch.cwd
	model, thinking := ch.model, ch.thinking
	go func() {
		ctx := context.Background()
		s, err := acquire(ctx, a.host, session, id, cwd)
		if err == nil {
			a.win.Update(func() { a.attach(ch, s) })
			err = useModel(ctx, s, model, thinking)
		}
		if err == nil {
			err = run(ctx, s, func(e agent.Event) {
				a.win.Update(func() { ch.apply(e) })
			})
		}
		a.win.Update(func() { a.finish(ch, err) })
	}()
}

// acquire 返回可运行的会话：已有则复用，否则创建或恢复。
func acquire(
	ctx context.Context, h *host.Manager, s *host.Session, id, cwd string,
) (*host.Session, error) {
	switch {
	case s != nil:
		return s, nil
	case id == "":
		return h.Create(ctx, cwd)
	default:
		return h.Resume(ctx, id)
	}
}

// useModel 让会话采用界面上选定的模型与思考深度；未选择或没有变化时什么也不做。
func useModel(
	ctx context.Context, s *host.Session, model string, thinking agent.ThinkingLevel,
) error {
	state := s.State()
	if model == "" || state.ModelRef == model && state.ThinkingLevel == thinking {
		return nil
	}
	return s.SetModel(ctx, model, thinking)
}

// attach 记录会话句柄；新会话创建后登记到已打开的会话中。
func (a *App) attach(ch *chat, s *host.Session) {
	ch.session = s
	if ch.id == "" {
		ch.id = s.ID()
		a.chats[ch.id] = ch
	}
	state := s.State()
	ch.cwd, a.cwd = state.Cwd, state.Cwd
	if ch.model == "" {
		ch.model, ch.thinking = state.ModelRef, state.ThinkingLevel
	}
	a.refresh()
}

// finish 在一轮对话结束后更新状态；用户主动停止不算错误。
func (a *App) finish(ch *chat, err error) {
	ch.running, ch.retry = false, ""
	ch.dropCompacting() // 压缩中被取消时不会有结束事件，占位行在这里清掉
	switch {
	case errors.Is(err, host.ErrNothingToCompact):
		a.toast = err.Error()
	case err != nil && !errors.Is(err, context.Canceled):
		ch.err = err.Error()
	}
	ch.settle(cmp.Or(ch.err, "已中断"))
	if errors.Is(err, host.ErrNeedsReload) {
		ch.session = nil // 下次发送时重新恢复
	}
	// 在别的会话时结束的运行，回到这个会话之前侧栏上标为未读。
	ch.unread = ch != a.current && ch.err == ""
	a.refresh()
}

// pickDir 让用户为尚未创建的会话选择工作目录。
func (a *App) pickDir(ch *chat) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       "选择工作目录",
			DefaultPath: ch.cwd,
			Directory:   true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() { ch.cwd, a.cwd = paths[0], paths[0] })
	}()
}
