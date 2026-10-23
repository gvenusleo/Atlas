package gui

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"
)

// errChanged 说明磁盘上的文件在打开之后变过，保存会盖掉别处的修改。
var errChanged = errors.New("文件在磁盘上已变更")

// saveTab 保存标签的文件，成功后调用 then（可为空）。写入在后台做；磁盘上的文件在打开或
// 上次保存之后变过时不覆盖，标签上提示冲突，由用户选择重新加载还是保留自己的修改。
func (a *App) saveTab(tb *fileTab, then func()) {
	if tb.ed == nil || tb.saving {
		return
	}
	snap, stat := tb.ed.Buffer.Snapshot(), tb.stat
	tb.saving, tb.err = true, ""
	go func() {
		st, err := writeFile(tb.path, snap, stat)
		a.win.Update(func() {
			tb.finishSave(snap.Version(), st, err)
			if err == nil && then != nil {
				then()
			}
		})
	}()
}

// writeFile 把 snap 写进 p：先写同目录的临时文件再改名，读者不会看到写了一半的文件，
// 权限保留，换行风格由 snap 带着；p 是符号链接时写它指向的文件。磁盘上的文件与 stat 不一样
// 就返回 errChanged、什么也不写；文件已经不在了就新建。返回写完之后文件的样子。
func writeFile(p string, snap editor.Snapshot, stat fileStat) (fileStat, error) {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	mode := fs.FileMode(0o644)
	switch info, err := os.Stat(p); {
	case err == nil && !stat.same(info):
		return fileStat{}, errChanged
	case err == nil:
		mode = info.Mode().Perm()
	case !errors.Is(err, fs.ErrNotExist):
		return fileStat{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".atlas-save-*")
	if err != nil {
		return fileStat{}, err
	}
	defer os.Remove(tmp.Name()) // 改名成功之后它已经不在了
	if _, err = snap.WriteTo(tmp); err == nil {
		err = tmp.Chmod(mode)
	}
	if err = errors.Join(err, tmp.Close()); err == nil {
		err = os.Rename(tmp.Name(), p)
	}
	if err != nil {
		return fileStat{}, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return fileStat{}, err
	}
	return fileStat{info.ModTime(), info.Size()}, nil
}

// finishSave 记下一次保存的结果；version 是保存的那一版缓冲区。
func (tb *fileTab) finishSave(version uint64, st fileStat, err error) {
	tb.saving = false
	switch {
	case errors.Is(err, errChanged):
		tb.changed = true
	case err != nil:
		tb.err = "保存失败：" + err.Error()
	default:
		tb.stat, tb.changed, tb.gone = st, false, false
		// 写的时候又改了的，仍然算没保存。
		if tb.ed.Buffer.Version() == version {
			tb.markSaved()
		}
	}
}

// syncTab 核对标签的文件在磁盘上有没有变过：没有未保存的修改就重新读入，有就提示冲突，
// 文件没了也提示。
func (a *App) syncTab(tb *fileTab) {
	if tb.loading || tb.saving {
		return
	}
	info, err := os.Stat(tb.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		tb.gone = true
	case err != nil: // 暂时看不了（权限、网络盘等）：等下一次核对
	case tb.stat.same(info):
		tb.gone = false
	case tb.dirty():
		tb.changed, tb.gone = true, false
	default:
		tb.gone = false
		a.reloadTab(tb, false)
	}
}

// reloadTab 在后台重新读入标签的文件；force 为真时丢掉未保存的修改。
func (a *App) reloadTab(tb *fileTab, force bool) {
	go func() {
		text, st, note := readTab(tb.path)
		a.win.Update(func() { tb.reload(text, st, note, force) })
	}()
}

// reload 用重新读到的内容换掉标签里的。force 为假时，读的时候用户又改了就不换，
// 改成提示冲突。
func (tb *fileTab) reload(text string, st fileStat, note string, force bool) {
	switch {
	case tb.ed != nil && !force && tb.dirty():
		tb.changed = true
	case tb.ed != nil && note == "":
		tb.ed.Buffer.Reset(text)
		tb.edited.forget()
		tb.stat, tb.changed, tb.gone = st, false, false
	default: // 以前不能预览现在可以，或现在不能预览了：重新建
		tb.load(text, st, note)
		tb.changed, tb.gone = false, false
	}
}

// keep 在用户选择保留自己的修改时，把磁盘上现在的文件当作基准，之后保存就会覆盖它。
func (tb *fileTab) keep() {
	if info, err := os.Stat(tb.path); err == nil {
		tb.stat = fileStat{info.ModTime(), info.Size()}
	}
	tb.changed = false
}

// watchFiles 开始监视 ch 的工作目录：目录里的变化刷新文件树，打开的文件被别处改动时同步到
// 标签。只要“某个目录里有变化”的提示（HintsOnly）：不给整棵树做快照，工作目录是家目录这样
// 的大目录时也不费事，而我们本来就只是重读读过的目录、核对所有标签。目录太大监视不了时不监视，
// 只在打开面板与切换标签时核对。
func (a *App) watchFiles(ch *chat) {
	fp, cwd := ch.files, ch.cwd
	fp.stopWatch()
	ctx, cancel := context.WithCancel(context.Background())
	fp.stop = cancel
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

// watchSkips 是监视时不看的目录：.git 里的变化太碎，node_modules 太大。
func watchSkips(p string, isDir bool) bool {
	name := path.Base(p)
	return isDir && (name == ".git" || name == "node_modules")
}

// filesChanged 处理工作目录里的一批变化：读过的目录里有变化的重新读，打开的文件核对一遍。
// 事件的 Path 是“其中或其下有变化”的目录，"." 是整个工作目录。
func (a *App) filesChanged(ch *chat, cwd string, events []mygo.FileEvent) {
	fp := ch.files
	if fp == nil || !fp.open || fp.cwd != cwd {
		return
	}
	for dir := range fp.dirs {
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
