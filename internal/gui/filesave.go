package gui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui/editor"
)

// errChanged 说明磁盘上的文件在打开之后变过，保存会盖掉别处的修改。
var errChanged = errors.New("文件在磁盘上已变更")

// saveTab 在后台保存标签；then 接收无法安全关闭标签的原因，为空时可以关闭。
// 磁盘上的文件已变更时不覆盖，保留标签上的冲突提示，交由用户处理。
func (a *App) saveTab(ch *chat, tb *fileTab, then func(string)) {
	if tb.ed == nil || tb.saving {
		if then != nil {
			then("文件无法保存或正在保存")
		}
		return
	}
	path := tb.path
	if a.worktreeRecoveries[ch.id].Root != "" || !a.beginFileWrite(path) {
		tb.err = "工作树不可用，请稍后再试或先恢复工作树"
		if then != nil {
			then(tb.err)
		}
		return
	}
	buf, stat, win := tb.ed.Buffer, tb.stat, a.win
	snap := buf.Snapshot()
	tb.saving, tb.err = true, ""
	go func() {
		st, err := writeFile(path, snap, stat)
		win.Update(func() {
			a.endFileWrite(path)
			message := tb.finishSave(buf, snap.Version(), st, err)
			if then != nil {
				then(message)
			}
		})
	}()
}

func (a *App) beginFileWrite(path string) bool {
	if a.worktreeBlocked(path) {
		return false
	}
	if a.fileWrites == nil {
		a.fileWrites = make(map[string]int)
	}
	a.fileWrites[path]++
	return true
}

func (a *App) endFileWrite(path string) {
	if a.fileWrites[path] <= 1 {
		delete(a.fileWrites, path)
		return
	}
	a.fileWrites[path]--
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

// finishSave 只有在写入的仍是当前缓冲区的当前版本时才标为已保存。
func (tb *fileTab) finishSave(buf *editor.Buffer, version uint64, st fileStat, err error) string {
	tb.saving = false
	switch {
	case errors.Is(err, errChanged):
		tb.changed = true
		return errChanged.Error()
	case err != nil:
		tb.err = "保存失败：" + err.Error()
		return tb.err
	default:
		tb.stat, tb.changed, tb.gone = st, false, false
		if tb.ed == nil || tb.ed.Buffer != buf || buf.Version() != version {
			tb.saveDiverged = true
			return "保存期间文件内容已变化"
		}
		tb.markSaved()
		return ""
	}
}

// syncTab 核对标签的文件在磁盘上有没有变过：没有未保存的修改就重新读入，有就提示冲突，
// 文件没了也提示。
func (a *App) syncTab(tb *fileTab) {
	if tb.reloading {
		tb.resync = true
		return
	}
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
	if tb.reloading {
		return
	}
	tb.reloading = true
	p := tb.path
	go func() {
		content := readTab(p)
		a.win.Update(func() {
			if !tb.reload(content, force) {
				a.reloadTab(tb, force)
			}
		})
	}()
}

// reload 应用重读结果；期间收到文件变化时保留现有内容与定位请求，返回 false 要求再次读取。
// 非强制重读期间发生编辑时保留用户修改，并提示冲突。
func (tb *fileTab) reload(content tabContent, force bool) bool {
	tb.reloading = false
	if tb.resync {
		tb.resync = false
		return false
	}
	switch {
	case tb.ed != nil && !force && tb.dirty():
		tb.changed = true
	case tb.ed != nil && content.note == "" && content.img == nil:
		tb.ed.Buffer.Reset(content.text)
		tb.saveDiverged = false
		tb.edited.forget()
		tb.mdImages.forgetLocalImages()
		tb.stat, tb.changed, tb.gone = content.stat, false, false
	default: // 以前不能预览现在可以，或现在不能预览了：重新建
		tb.load(content)
		tb.changed, tb.gone = false, false
	}
	if tb.position != nil {
		tb.showPosition(*tb.position)
	}
	return true
}

// keep 在用户选择保留自己的修改时，把磁盘上现在的文件当作基准，之后保存就会覆盖它。
func (tb *fileTab) keep() {
	if info, err := os.Stat(tb.path); err == nil {
		tb.stat = fileStat{info.ModTime(), info.Size()}
	}
	tb.changed = false
}
