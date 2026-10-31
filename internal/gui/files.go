package gui

import (
	"cmp"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/gvenusleo/atlas/internal/tools"
)

// filePanel 是会话的文件面板：文件树、筛选，以及打开的文件标签。
type filePanel struct {
	treeHidden bool    // 文件树收起了，只显示文件内容
	treeWidth  float32 // 文件树与内容之间分隔条的位置
	prevWidth  float32 // 打开时右侧栏被拉宽过：拉宽之前的宽度，关闭时还原；0 为没拉宽

	// 文件树，见 tree.go。条目是相对工作目录的路径。
	cwd    string                  // kids 对应的工作目录
	gen    int                     // 换一次工作目录加一，读目录的结果据此判断是否过时
	kids   map[string][]string     // 读过的目录里的条目，键是目录的路径，根是 ""
	isDir  map[string]bool         // 条目是不是目录
	tree   ui.OutlineState[string] // 展开着的目录与滚动位置
	chosen ui.Selection[string]    // 选中的条目
	opened map[string]bool         // 上一帧展开着的目录，用来发现新展开的
	shown  string                  // 上一帧选中的当前标签的文件，它换了就重新选中

	filter string                  // 筛选框里的文字，非空时文件树换成筛选结果
	found  memo[rankKey, []string] // 筛选的结果

	tabs   []*fileTab
	active int // 当前标签
	reveal int // 需要滚进可见范围的标签的标识
	next   int // 已建的标签数，用作标签的标识
}

// fileEntry 是目录里的一项。
type fileEntry struct {
	name string
	dir  bool
}

// fileTab 是文件面板里打开的一个文件。
type fileTab struct {
	id      int
	path    string         // 绝对路径
	loading bool           // 还在读
	ed      *editor.State  // 文本的编辑器；不能预览、或是图片时为空
	img     ui.ImageSource // 图片；文本与不能预览时为空
	note    string         // 不能预览的原因

	onSave  func()   // 保存这个标签的文件，在编辑器里按 Cmd+S 时调用
	stat    fileStat // 打开或保存时磁盘上的文件，用来发现外部修改
	saving  bool     // 正在保存
	changed bool     // 文件在磁盘上改过，而这里有未保存的修改
	gone    bool     // 文件在磁盘上没了
	err     string   // 最近一次保存失败的原因

	render bool                   // 显示渲染后的样子（Markdown 与 SVG 的预览、CSV 表格），否则是源码
	edited memo[uint64, bool]     // 有没有未保存的修改，按缓冲区的 version 缓存
	md     memo[uint64, string]   // Markdown 预览用的文本，按缓冲区的 version 缓存
	svg    memo[uint64, svgImage] // SVG 解析出来的图，按缓冲区的 version 缓存
	csv    memo[uint64, csvTable] // 表格，按缓冲区的 version 缓存
	order  memo[orderKey, []int]  // 表格要显示的行号，按内容与排序缓存
	table  ui.ListState           // 表格视图的选中与滚动
	sort   ui.SortOrder           // 表格按哪一列排，零值是不排
	sorted ui.SortOrder           // 表格现在按哪个排序显示，用来发现排序变了
}

// fileStat 是磁盘上一个文件的修改时间与大小，一样就当作没有变过。
type fileStat struct {
	mtime time.Time
	size  int64
}

// orderKey 是显示顺序的缓存键：内容变了或者排序变了才重排。
type orderKey struct {
	version    uint64
	column     string
	descending bool
}

// tabContent 是读一个文件的结果：文本进编辑器，img 不为空时是图片，note 不为空时是
// 不能预览的原因；三者只有一项成立。
type tabContent struct {
	text string
	img  ui.ImageSource
	stat fileStat
	note string
}

// same 报告磁盘上的文件 info 与 s 一样。
func (s fileStat) same(info fs.FileInfo) bool {
	return info.ModTime().Equal(s.mtime) && info.Size() == s.size
}

// dirty 报告标签里有没保存的修改。逐帧要问，比较文本不便宜，所以按 version 缓存。
func (tb *fileTab) dirty() bool {
	return tb.ed != nil && tb.edited.get(tb.ed.Buffer.Version(), tb.ed.Buffer.Modified)
}

// markSaved 把缓冲区现在的文本当作已保存的。
func (tb *fileTab) markSaved() {
	tb.ed.Buffer.MarkSaved()
	tb.edited.forget()
}

// previewText 返回 Markdown 预览用的文本，缓冲区变了才重新取。
func (tb *fileTab) previewText() string {
	return tb.md.get(tb.ed.Buffer.Version(), tb.ed.Buffer.String)
}

// filesOpen 报告文件面板是否开着。
func (ch *chat) filesOpen() bool { return ch.panelOpen(filesPanel) }

// dirtyTabs 返回会话的文件面板里有未保存修改的标签数。
func (ch *chat) dirtyTabs() int {
	if ch.files == nil {
		return 0
	}
	n := 0
	for _, tb := range ch.files.tabs {
		if tb.dirty() {
			n++
		}
	}
	return n
}

// toggleFiles 显示或收起当前会话的文件面板；第一次打开时建面板，右侧栏不够宽时拉宽，
// 关闭时在没被再拖过的情况下还原，打开时核对已打开的文件。
func (a *App) toggleFiles() {
	ch := a.current
	if ch.files == nil {
		ch.files = &filePanel{treeWidth: treeDefault}
	}
	fp := ch.files
	a.togglePanel(filesPanel, func() {
		a.widenSide(&fp.prevWidth)
		for _, tb := range fp.tabs {
			a.syncTab(tb)
		}
	}, func() { a.restoreSide(&fp.prevWidth) })
}

// reset 在换了工作目录时清掉文件树。
func (fp *filePanel) reset(cwd string) {
	fp.cwd = cwd
	fp.gen++
	fp.kids, fp.isDir, fp.opened = map[string][]string{}, map[string]bool{}, map[string]bool{}
	fp.tree.Open.Clear()
	fp.chosen.Clear()
	fp.shown = ""
}

// readDir 在后台（重新）读取工作目录下相对路径 rel 的目录，读完之前先用旧的条目。
func (a *App) readDir(ch *chat, rel string) {
	fp, cwd, gen := ch.files, ch.cwd, ch.files.gen
	go func() {
		entries := readEntries(filepath.Join(cwd, rel))
		a.win.Update(func() {
			if fp.gen != gen { // 读的时候换了工作目录的，结果作废
				return
			}
			kids := make([]string, len(entries)) // 空目录也不是 nil：树据此知道它是目录
			for i, e := range entries {
				kids[i] = path.Join(rel, e.name)
				fp.isDir[kids[i]] = e.dir
			}
			fp.kids[rel] = kids
		})
	}()
}

// readEntries 读取一个目录：目录在前、文件在后，各自按名称不分大小写排序，不含 skipped 的名字。
// 读不了时当作空目录。
func readEntries(dir string) []fileEntry {
	items, _ := os.ReadDir(dir)
	entries := make([]fileEntry, 0, len(items))
	for _, it := range items {
		if skipped(it.Name()) {
			continue
		}
		isDir := it.IsDir()
		if it.Type()&fs.ModeSymlink != 0 { // 指向目录的符号链接当作目录
			if info, err := os.Stat(filepath.Join(dir, it.Name())); err == nil {
				isDir = info.IsDir()
			}
		}
		entries = append(entries, fileEntry{it.Name(), isDir})
	}
	slices.SortFunc(entries, func(x, y fileEntry) int {
		if x.dir != y.dir {
			if x.dir {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(strings.ToLower(x.name), strings.ToLower(y.name)),
			cmp.Compare(x.name, y.name))
	})
	return entries
}

// openFile 在文件面板里打开工作目录下相对路径 rel 的文件：已经开着就切过去，否则新开一个标签。
func (a *App) openFile(ch *chat, rel string) {
	fp := ch.files
	abs := filepath.Join(ch.cwd, rel)
	if i := slices.IndexFunc(fp.tabs, func(tb *fileTab) bool { return tb.path == abs }); i >= 0 {
		fp.active, fp.reveal = i, fp.tabs[i].id
		a.syncTab(fp.tabs[i])
		return
	}
	fp.next++
	tb := &fileTab{id: fp.next, path: abs, loading: true}
	tb.onSave = func() { a.saveTab(tb, nil) }
	fp.tabs = append(fp.tabs, tb)
	fp.active, fp.reveal = len(fp.tabs)-1, tb.id
	go func() {
		content := readTab(abs)
		a.win.Update(func() { tb.load(content) })
	}()
}

// showFile 打开文件面板（已开着的就不动），并在其中打开相对工作目录的文件 rel。
func (a *App) showFile(ch *chat, rel string) {
	if !ch.filesOpen() || ch.expanded != noPanel && ch.expanded != filesPanel {
		a.toggleFiles()
	}
	a.openFile(ch, rel)
}

// readTab 读取标签的文件，st 是读它之前它在磁盘上的样子。图片解码，文本读成 UTF-8；
// 不能预览（太大、不是普通文件、不是 UTF-8 文本）时 note 是原因。
func readTab(p string) tabContent {
	if isRaster(p) {
		return readImage(p)
	}
	text, info, err := tools.ReadText(context.Background(), p)
	content := tabContent{text: text}
	if info != nil {
		content.stat = fileStat{mtime: info.ModTime(), size: info.Size()}
	}
	if err != nil {
		content.text, content.note = "", noteOf(p, err)
	}
	return content
}

// isMarkdown 报告路径是不是 Markdown 文件。
func isMarkdown(p string) bool {
	ext := extOf(p)
	return ext == ".md" || ext == ".markdown"
}

// load 在文件读完后建出编辑器或记下图片；note 不空时不能预览。Markdown 与 CSV/TSV 先
// 显示渲染后的样子，Markdown 与纯文本自动换行。
func (tb *fileTab) load(content tabContent) {
	tb.loading, tb.note, tb.stat, tb.img = false, content.note, content.stat, content.img
	tb.edited.forget() // 新的缓冲区，旧的缓存不能用
	tb.md.forget()
	tb.svg.forget()
	tb.csv.forget()
	tb.order.forget()
	if content.note != "" || content.img != nil {
		tb.ed = nil
		return
	}
	buf := editor.NewBuffer(content.text)
	buf.SetLanguage(editor.LanguageFor(tb.path))
	tb.render = isMarkdown(tb.path) || isCSV(tb.path) || isSVG(tb.path)
	tb.ed = &editor.State{
		Buffer:   buf,
		SoftWrap: (tb.render && isMarkdown(tb.path)) || extOf(tb.path) == ".txt",
		Font:     codeFont, Label: filepath.Base(tb.path),
		OnInput: func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && ev.Mods == ui.Cmd && ev.Key == ui.KeyS {
				tb.onSave()
				return true
			}
			return false
		},
	}
}

// remove 关闭标签 tb。
func (fp *filePanel) remove(tb *fileTab) {
	i := slices.Index(fp.tabs, tb)
	if i < 0 {
		return
	}
	fp.tabs = slices.Delete(fp.tabs, i, i+1)
	if i < fp.active {
		fp.active--
	}
	fp.active = max(min(fp.active, len(fp.tabs)-1), 0)
}

// activePath 返回当前标签的文件的绝对路径，没有标签时为空。
func (fp *filePanel) activePath() string {
	if fp.active < len(fp.tabs) {
		return fp.tabs[fp.active].path
	}
	return ""
}
