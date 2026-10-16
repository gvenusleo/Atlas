package gui

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// iconFiles 是取用的 Lucide 图标（lucide-static，ISC 许可）；整个目录嵌入，许可证随二进制发布。
//
//go:embed icons
var iconFiles embed.FS

// icon 解析一个嵌入的图标；图标是描边的，颜色取自文字色。
func icon(name string) *ui.SVG {
	data, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

var (
	plusIcon     = icon("plus")
	sendIcon     = icon("arrow-up")
	stopIcon     = icon("square")
	sidebarIcon  = icon("panel-left")
	chevronIcon  = icon("chevron-right")
	closeIcon    = icon("x")
	folderIcon   = icon("folder")
	terminalIcon = icon("terminal")
	clockIcon    = icon("clock")
	moreIcon     = icon("ellipsis")
	copyIcon     = icon("copy")
	checkIcon    = icon("check")
)
