package gui

import "github.com/egoist/mygo/ui"

// icon 解析一个 24×24 的描边图标，颜色取自文字色。
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" ` +
		`fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" ` +
		`stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	plusIcon    = icon(`<path d="M12 5v14M5 12h14"/>`)
	sidebarIcon = icon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>`)
	sendIcon    = icon(`<path d="M12 19V5M5 12l7-7 7 7"/>`)
	stopIcon    = icon(`<rect x="7" y="7" width="10" height="10" rx="2" fill="currentColor"/>`)
	chevronIcon = icon(`<path d="M9 6l6 6-6 6"/>`)
	closeIcon   = icon(`<path d="M6 6l12 12M18 6L6 18"/>`)
	folderIcon  = icon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5` +
		`a2 2 0 0 1-2-2z"/>`)
	fileIcon = icon(`<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/>` +
		`<path d="M14 3v5h5M9 13h6M9 17h6"/>`)
	terminalIcon = icon(`<rect x="3" y="5" width="18" height="14" rx="2"/>` +
		`<path d="M7 10l3 2-3 2M12 15h5"/>`)
	editIcon = icon(`<path d="M4 20h4L19 9l-4-4L4 16zM13.5 6.5l4 4"/>`)
	toolIcon = icon(`<path d="M14.7 6.3a4 4 0 0 0-5.4 5.2L4 16.8V20h3.2l5.3-5.3a4 4 0 0 0 ` +
		`5.2-5.4l-2.5 2.5-2.7-.7-.7-2.7z"/>`)
)

// toolInfo 返回工具对应的图标和动词；未知工具用扳手和工具名。
func toolInfo(name string) (*ui.SVG, string) {
	switch name {
	case "read":
		return fileIcon, "读取"
	case "write":
		return editIcon, "写入"
	case "edit":
		return editIcon, "编辑"
	case "shell":
		return terminalIcon, "运行"
	}
	return toolIcon, name
}
