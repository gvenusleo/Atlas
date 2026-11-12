package gui

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// fileCreateForm 固定开始新建时的目标，避免创建结果落到后来切换的工作目录。
type fileCreateForm struct {
	ch       *chat
	cwd      string
	gen      int
	parent   string
	dir      bool
	name     string
	err      string
	creating bool
}

func (f *fileCreateForm) matchesChat(ch *chat) bool {
	return ch == f.ch && ch.cwd == f.cwd && ch.files != nil && ch.files.gen == f.gen
}

func (a *App) beginFileCreate(ch *chat, parent string, dir bool) {
	if parent == "." {
		parent = ""
	}
	name := "未命名.txt"
	if dir {
		name = "未命名目录"
	}
	a.fileCreate = &fileCreateForm{
		ch: ch, cwd: ch.cwd, gen: ch.files.gen, parent: parent, dir: dir, name: name,
	}
}

func (a *App) createEntry(f *fileCreateForm) {
	if a.fileCreate != f || !f.matchesChat(a.current) || f.creating {
		return
	}
	name := strings.TrimSpace(f.name)
	switch {
	case name == "":
		f.err = "请输入名称"
		return
	case name == "." || !fs.ValidPath(name) || !filepath.IsLocal(filepath.FromSlash(name)) ||
		strings.ContainsAny(name, "\\\x00"):
		f.err = "请输入相对路径，各级名称之间使用 /，不能包含空名称、. 或 .."
		return
	}
	for part := range strings.SplitSeq(name, "/") {
		if skipped(part) {
			f.err = "文件树不显示路径中的名称，请使用其他名称"
			return
		}
	}
	rel := path.Join(f.parent, name)
	abs, dir := filepath.Join(f.cwd, rel), f.dir
	f.creating, f.err = true, ""
	go func() {
		err := createPath(abs, dir)
		a.win.Update(func() { a.finishFileCreate(f, rel, err) })
	}()
}

func createPath(abs string, dir bool) error {
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if dir {
		return os.Mkdir(abs, 0o755)
	}
	// 排他创建，不覆盖同名文件，也不跟随已存在的符号链接。
	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func (a *App) finishFileCreate(f *fileCreateForm, rel string, err error) {
	if a.fileCreate != f || !f.matchesChat(a.current) {
		return
	}
	f.creating = false
	if err != nil {
		f.err = "创建失败：" + err.Error()
		if errors.Is(err, fs.ErrExist) {
			f.err = "已存在同名文件或目录，请使用其他名称"
		}
		return
	}
	ch, fp := f.ch, f.ch.files
	fp.filter = ""
	fp.isDir[rel] = f.dir
	for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
		fp.isDir[dir] = true
		fp.tree.Open.Add(dir)
		a.readDir(ch, dir)
	}
	a.readDir(ch, "")
	a.loadPaths(ch)
	fp.shown = fp.activeRel(ch.cwd)
	fp.chosen.Clear()
	fp.chosen.Add(rel)
	fp.reveal = rel
	if !f.dir {
		a.openFile(ch, rel)
	}
	a.fileCreate = nil
}
