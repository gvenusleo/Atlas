package gui

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// iconFiles 是取用的图标，一个来源一个目录，各带自己的许可证，随二进制发布：
// lucide 是界面图标（lucide-static，ISC 许可），material 是文件类型图标
// （Material Icon Theme，MIT 许可）。
//
//go:embed icons
var iconFiles embed.FS

// icon 解析一个嵌入的图标，name 是 icons 下不带 .svg 的路径，如 lucide/plus。
// Lucide 图标是描边的，颜色取自文字色；material 的图标有自己的颜色。
func icon(name string) *ui.SVG {
	data, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

var (
	plusIcon      = icon("lucide/plus")
	arrowUpIcon   = icon("lucide/arrow-up")
	sendIcon      = arrowUpIcon
	stopIcon      = icon("lucide/square")
	sidebarIcon   = icon("lucide/panel-left")
	chevronIcon   = icon("lucide/chevron-right")
	closeIcon     = icon("lucide/x")
	folderIcon    = icon("lucide/folder")
	terminalIcon  = icon("lucide/terminal")
	diffIcon      = icon("lucide/file-diff")
	arrowDownIcon = icon("lucide/arrow-down")
	clockIcon     = icon("lucide/clock")
	moreIcon      = icon("lucide/ellipsis")
	copyIcon      = icon("lucide/copy")
	checkIcon     = icon("lucide/check")
	listIcon      = icon("lucide/list")
	outlineIcon   = icon("lucide/table-of-contents")
	codeIcon      = icon("lucide/code")
	eyeIcon       = icon("lucide/eye")
	expandIcon    = icon("lucide/maximize-2")
	restoreIcon   = icon("lucide/minimize-2")
	searchIcon    = icon("lucide/search")
)
