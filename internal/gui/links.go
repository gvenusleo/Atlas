package gui

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

// Markdown 里的链接：#标题 滚到本篇的标题，文件路径在文件面板里打开，网址只把 ui.SafeToOpen
// 认可的交给系统。设了 OnLink 之后 MyGo 不再自己处理任何链接，所以都在这里接。

// 链接里指向文件中某一行的两种写法，模型与 GitHub 都常用。
var (
	lineFragment = regexp.MustCompile(`^L(\d+)`)          // a.go#L12、a.go#L12-L20
	lineSuffix   = regexp.MustCompile(`:(\d+)(?::\d+)?$`) // a.go:12、a.go:12:3
)

// useLinks 在构建视图前填入链接处理。锚点与点击在构建时读取，右键菜单在打开时读取。
// 会话中的视图共用基准目录，文件面板只构建当前标签的预览，因此右键菜单可用最后填入的回调。
// base 是相对路径链接的基准目录：对话里是工作目录，文件预览里是文件所在的目录。
// anchor 是这个视图点过、还没滚过去的 #标题。它只填一次就清空：MyGo 在文档换了内容或重新
// 显示时会再滚到 Anchor，留着它，文件被改或预览切回来时就会跳回那个标题。
func (a *App) useLinks(o *markdown.Options, anchor *string, base string) *markdown.Options {
	o.OnLink = func(dest string) { a.followLink(anchor, base, dest) }
	o.LinkMenu = func(m *ui.Menu, dest string) { a.linkMenu(m, base, dest) }
	o.Anchor, *anchor = *anchor, ""
	return o
}

// followLink 处理点击。路径不交给系统程序打开，网址也只打开 ui.SafeToOpen 认可的，免得
// 模型写的链接运行了程序。
func (a *App) followLink(anchor *string, base, dest string) {
	if dest == "" {
		return // [文字]() 这样的空链接，按路径解析会落到 base 目录本身
	}
	if strings.HasPrefix(dest, "#") {
		*anchor = dest
		return
	}
	if p, line, ok := linkTarget(base, dest); ok {
		a.openLinkPath(p, line)
		return
	}
	if !ui.SafeToOpen(dest) {
		a.toast = "不支持打开这类链接：" + dest
		return
	}
	if err := mygo.Shell.OpenExternal(dest); err != nil {
		a.toast = "打开链接失败：" + err.Error()
	}
}

// openLinkPath 在文件面板打开链接指向的文件，有行号时定位到该行。目录在文件管理器中显示。
func (a *App) openLinkPath(p string, line int) {
	info, err := os.Stat(p)
	if err != nil {
		a.toast = "文件不存在：" + p
		return
	}
	if info.IsDir() {
		mygo.Shell.ShowItemInFolder(p)
		return
	}
	ch := a.current
	a.showFile(ch, p)
	if line > 0 {
		ch.files.tabs[ch.files.active].showPosition(editor.Pos{Line: line - 1})
	}
}

// linkMenu 是链接的右键菜单：文件路径用 pathItems，其中的相对路径与文件树一样相对工作目录；
// 其余是网址的菜单项。链接的菜单取代了外面的那个，所以末尾补上选中文字的复制与全选。
func (a *App) linkMenu(m *ui.Menu, base, dest string) {
	if p, _, ok := linkTarget(base, dest); ok {
		cwd := a.current.cwd
		rel := relOrAbs(cwd, p)
		if filepath.IsAbs(rel) {
			cwd = "" // 工作目录之外的路径没有相对路径，各项都用绝对路径
		}
		_, err := os.Stat(p)
		pathItems(m, cwd, rel, err == nil)
	} else {
		a.urlItems(m, dest)
	}
	m.Separator()
	m.EditItems()
}

// urlItems 是网址链接的菜单项：在浏览器中打开（不支持的协议不可选）、复制。
func (a *App) urlItems(m *ui.Menu, dest string) {
	if m.Item("打开链接").Disabled(!ui.SafeToOpen(dest)).Chosen() {
		if err := mygo.Shell.OpenExternal(dest); err != nil {
			a.toast = "打开链接失败：" + err.Error()
		}
	}
	if m.Item("复制链接").Chosen() {
		mygo.Clipboard.WriteText(dest)
	}
}

// linkTarget 解析指向文件的链接：绝对路径，以及行号（从 1 起，0 为没有）。相对路径以 base
// 为基准；没有协议或是 file: 的按网址解码（%20 等），解析不了的原样当路径。带其他协议的
// 不是文件，ok 为假。
func linkTarget(base, dest string) (p string, line int, ok bool) {
	p, frag, _ := strings.Cut(dest, "#")
	if m := lineFragment.FindStringSubmatch(frag); m != nil {
		line, _ = strconv.Atoi(m[1])
	}
	if m := lineSuffix.FindStringSubmatchIndex(p); m != nil {
		line, _ = strconv.Atoi(p[m[2]:m[3]])
		p = p[:m[0]]
	}
	if u, err := url.Parse(p); err == nil {
		if u.Scheme != "" && u.Scheme != "file" {
			return "", 0, false
		}
		p = u.Path
	}
	if !filepath.IsAbs(p) && base != "" {
		if !os.IsPathSeparator(base[len(base)-1]) {
			base += string(os.PathSeparator)
		}
		p = base + p
	}
	// 符号链接后面的 .. 取决于链接的目标，须先按文件系统解析。
	if filepath.Clean(p) != p {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
	}
	return p, line, true
}
