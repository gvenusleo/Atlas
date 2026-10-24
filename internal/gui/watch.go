package gui

import (
	"context"
	"path"
	"strings"

	"github.com/egoist/mygo"
)

// watchFiles 开始监视 ch 的工作目录：文件面板里读过的目录有变化就重读，打开的文件被别处
// 改动时同步到标签，更改面板开着时重新读取更改。只要“某个目录里有变化”的提示
// （HintsOnly）：不给整棵树做快照，工作目录是家目录这样的大目录时也不费事，而我们本来就只是
// 重读读过的目录、核对所有标签。目录太大监视不了时不监视，只在打开面板与切换标签时核对。
func (a *App) watchFiles(ch *chat) {
	cwd := ch.cwd
	ch.stopWatch()
	ctx, cancel := context.WithCancel(context.Background())
	ch.unwatch = cancel
	go func() {
		opts := mygo.FileWatchOptions{Recursive: true, HintsOnly: true, Ignore: watchSkips}
		w, err := mygo.WatchFiles(ctx, cwd, opts)
		if err != nil {
			return
		}
		defer w.Close()
		for {
			events, err := w.Next(ctx)
			if err != nil {
				return
			}
			a.win.Update(func() { a.filesChanged(ch, cwd, events) })
		}
	}()
}

// syncWatch 让监视与面板一致：文件或更改面板开着就监视工作目录，都关了就停止。
func (a *App) syncWatch(ch *chat) {
	if !ch.filesOpen() && !ch.panelOpen(changesPanel) {
		ch.stopWatch()
		return
	}
	if ch.unwatch == nil {
		a.watchFiles(ch)
	}
}

// stopWatch 停止监视工作目录。
func (ch *chat) stopWatch() {
	if ch.unwatch != nil {
		ch.unwatch()
		ch.unwatch = nil
	}
}

// watchSkips 是监视时不看的目录：.git 里的变化太碎，node_modules 太大。
func watchSkips(p string, isDir bool) bool {
	name := path.Base(p)
	return isDir && (name == ".git" || name == "node_modules")
}

// filesChanged 处理工作目录里的一批变化：更改面板开着就重新读取更改，文件面板里读过的目录
// 有变化的重新读，打开的文件核对一遍。事件的 Path 是“其中或其下有变化”的目录，"." 是整个
// 工作目录。
func (a *App) filesChanged(ch *chat, cwd string, events []mygo.FileEvent) {
	if cp := ch.changes; cp != nil && ch.panelOpen(changesPanel) && cp.cwd == cwd {
		a.refreshChanges(ch)
	}
	fp := ch.files
	if fp == nil || !ch.filesOpen() || fp.cwd != cwd {
		return
	}
	for dir := range fp.kids {
		for _, ev := range events {
			if ev.Path == "." || dir == ev.Path || strings.HasPrefix(dir, ev.Path+"/") {
				a.readDir(ch, dir)
				break
			}
		}
	}
	for _, tb := range fp.tabs {
		a.syncTab(tb)
	}
}
