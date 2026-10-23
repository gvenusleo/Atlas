package gui

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/egoist/mygo"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// 这个文件是界面对核心的会话操作：打开、加载历史、发送、换配置、关闭。
// 都在后台调用核心，结果经 Window.Update 回到主线程；后台 goroutine 启动前先取出所需的值。

// open 切换到已有会话，首次打开时加载最近一页历史并让核心打开它。历史直接读库，
// 所以会话打不开（比如工作目录没了）也能看。
func (a *App) open(id string) {
	if !a.leave() {
		return
	}
	if ch := a.chats[id]; ch != nil {
		a.current, ch.unread = ch, false
		return
	}
	a.keys++
	ch := &chat{key: "chat-" + strconv.Itoa(a.keys), id: id}
	for _, info := range a.infos {
		if info.SessionID == id {
			ch.cwd, ch.title = info.Cwd, info.Title
		}
	}
	a.chats[id], a.current = ch, ch
	a.reload(ch)
	a.start(ch)
}

// reload 从头读最近一页历史，重建对话。
func (a *App) reload(ch *chat) {
	ch.loading = true
	id := ch.id
	go func() {
		history, err := a.service.ListHistory(context.Background(),
			protocol.ListHistoryRequest{SessionID: id, Limit: historyPage})
		a.win.Update(func() {
			ch.loading = false
			if err != nil {
				ch.err = err.Error()
				return
			}
			ch.load(history.Updates, history.Oldest)
		})
	}()
}

// start 让核心在后台打开会话：新会话新建，已有会话恢复。失败时历史照常显示，错误显示在
// 对话下方，发送时再试一次。窗口还没创建（启动时）先不做，创建窗口后再调用。
// 之后又调用过 start（比如又换了工作目录）时，这次的响应已经过时：丢掉它，新建的会话也关掉。
func (a *App) start(ch *chat) {
	if a.win == nil {
		return
	}
	ch.starts++
	gen, id, cwd := ch.starts, ch.id, ch.cwd
	go func() {
		ctx := context.Background()
		var options []protocol.ConfigOption
		var state protocol.SessionState
		var err error
		if id == "" {
			var resp protocol.NewSessionResponse
			resp, err = a.service.NewSession(ctx, protocol.NewSessionRequest{Cwd: cwd})
			id, options, state = resp.SessionID, resp.ConfigOptions, resp.State
		} else {
			var resp protocol.OpenSessionResponse
			resp, err = a.service.OpenSession(ctx, protocol.OpenSessionRequest{SessionID: id})
			options, state = resp.ConfigOptions, resp.State
		}
		a.win.Update(func() {
			switch {
			case gen != ch.starts:
				if err == nil && id != ch.id {
					a.closeSession(id)
				}
				return
			case err != nil:
				ch.err = err.Error()
				return
			}
			ch.id, ch.err = id, ""
			a.chats[id] = ch
			ch.attach(options, state)
			a.cwd = state.Cwd
		})
	}()
}

// loadOlder 在列表滚近顶部时再取前面一页记录；没有更早的记录或正在取时什么也不做。
// 大纲要跳到的提问还没加载时（ch.seekSeq），不论滚到哪里都取，最后一页恰好取到它为止。
// 列表的行有 Key，往前加入时当前看的位置不动。
func (a *App) loadOlder(ch *chat) {
	first, last := ch.list.Visible()
	if (last < first || first >= olderNear) && ch.seekSeq == 0 {
		return
	}
	if ch.loading || ch.paging || ch.oldest <= 1 {
		return
	}
	limit := historyPage
	if ch.seekSeq > 0 {
		limit = min(limit, int(ch.oldest-ch.seekSeq))
	}
	ch.paging = true
	id, before := ch.id, ch.oldest
	go func() {
		history, err := a.service.ListHistory(context.Background(),
			protocol.ListHistoryRequest{SessionID: id, Before: before, Limit: limit})
		a.win.Update(func() {
			ch.paging = false
			if err != nil {
				ch.err, ch.oldest, ch.seekSeq = err.Error(), 0, 0 // 不再重试
				return
			}
			ch.prepend(history.Updates, history.Oldest)
		})
	}()
}

// refresh 重新读取侧栏的会话列表。
func (a *App) refresh() {
	go func() {
		list, err := a.service.ListSessions(context.Background(), protocol.ListSessionsRequest{})
		a.win.Update(func() {
			if err != nil {
				a.err = err.Error()
				return
			}
			a.err, a.infos = "", list.Sessions
		})
	}()
}

// choose 让核心换会话的模型或思考深度；完成前选择器仍显示原来的，失败时显示原因。
func (a *App) choose(ch *chat, id, value string) {
	req := protocol.SetConfigOptionRequest{SessionID: ch.id, ConfigID: id, Value: value}
	go func() {
		resp, err := a.service.SetConfigOption(context.Background(), req)
		a.win.Update(func() {
			if err != nil {
				ch.err = err.Error()
				return
			}
			ch.options = resp.ConfigOptions
		})
	}()
}

// setCwd 给还没发过消息的新会话换工作目录：工作目录在建会话时就定了，所以丢掉原来的，
// 按新目录再建一个。
func (a *App) setCwd(ch *chat, cwd string) {
	if !ch.fresh() {
		return
	}
	if ch.id != "" {
		a.discard(ch)
	}
	ch.id, ch.ready, ch.err, ch.options, ch.commands = "", false, "", nil, nil
	ch.cwd, a.cwd = cwd, cwd
	a.start(ch)
}

// discard 让核心不再保留这个会话，并从已打开的会话里去掉它。
func (a *App) discard(ch *chat) {
	delete(a.chats, ch.id)
	a.closeSession(ch.id)
}

// closeSession 在后台通知核心，这个界面不再订阅会话 id。
func (a *App) closeSession(id string) {
	go func() {
		_ = a.service.CloseSession(context.Background(),
			protocol.CloseSessionRequest{SessionID: id})
	}()
}

// send 受理输入框里的文本与待发送的图片，在后台发给核心（/compact 与 /技能名 由核心处理）；
// 返回是否已受理。这一轮的结果经 run_state 更新送达。
func (a *App) send(ch *chat, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" && len(ch.attachments) == 0 || !ch.canSend() {
		return false
	}
	if len(ch.attachments) > 0 && !ch.acceptsImages() {
		a.toast = noImagesHint // 附上图片后又换了模型
		return false
	}
	prompt := draftPrompt(text, ch.attachments)
	ch.attachments, ch.err = nil, ""
	id, ready := ch.id, ch.ready
	go func() {
		ctx := context.Background()
		var err error
		if !ready { // 上次没打开（比如工作目录当时不在），再试一次
			var resp protocol.OpenSessionResponse
			resp, err = a.service.OpenSession(ctx, protocol.OpenSessionRequest{SessionID: id})
			if err == nil {
				a.win.Update(func() { ch.attach(resp.ConfigOptions, resp.State) })
			}
		}
		if err == nil {
			_, err = a.service.SendPrompt(ctx,
				protocol.SendPromptRequest{SessionID: id, Prompt: prompt})
		}
		a.win.Update(func() { a.finish(ch, err) })
	}()
	return true
}

// finish 显示核心没有受理这条消息的原因；受理之后的错误经 run_state 更新送达。
func (a *App) finish(ch *chat, err error) {
	switch {
	case errors.Is(err, protocol.ErrNothingToCompact):
		a.toast = err.Error()
	case err != nil:
		ch.err = err.Error()
	}
}

// pickDir 让用户为尚未保存的新会话选择工作目录。
func (a *App) pickDir(ch *chat) {
	options := mygo.OpenDialogOptions{
		Parent: a.win, Title: "选择工作目录", DefaultPath: ch.cwd, Directory: true,
	}
	go func() {
		paths, err := mygo.Dialog.Open(options)
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() { a.setCwd(ch, paths[0]) })
	}()
}
