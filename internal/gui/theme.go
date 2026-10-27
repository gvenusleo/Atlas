package gui

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// 界面的尺寸约定，单位为 DIP。
const (
	contentWidth = 720 // 阅读栏的最大宽度
	bubbleWidth  = 680 // 用户消息的最大宽度
	controlRound = 6   // 控件圆角
	boxRound     = 8   // 代码框与对比框的圆角
	inputRound   = 16  // 输入框圆角
	cardRound    = 12  // 浮动卡片圆角
	popupRound   = 10  // 弹出面板圆角
	edge         = 6   // 浮动卡片与窗口边缘的空隙
	handleWidth  = 6   // 卡片拖动条的宽度
	barButton    = 24  // 标题栏按钮的边长
	barInset     = 12  // 标题栏右端的按钮离窗口边缘的空隙
	tabHeight    = 24  // 终端标签的高度，与标题栏按钮同高
	tabInset     = 4   // 标签栏左右的内边距，加边框后与标签上方的空隙相等
	titleBarMin  = 44  // 标题栏的最小高度，交通灯在其中垂直居中
	gutter       = 20  // 阅读区两侧的留白
	edgeHeight   = 32  // 消息列表上下沿的渐隐带的高度
	edgePad      = 18  // 列表内容离上下边缘的留白，让静止时的内容不落在带里

	sideMin     = 240 // 右侧栏的最小宽度
	sideDefault = 420 // 右侧栏的默认宽度
	filesMin    = 560 // 打开文件面板时右侧栏至少这么宽，放得下文件树与内容
	stackMin    = 160 // 右侧栏里叠放的卡片各自的最小高度
	chatMin     = 320 // 打开右侧栏时对话区的最小宽度

	treeMin           = 140 // 文件树的最小宽度
	treeDefault       = 200 // 文件树的默认宽度
	changeTreeDefault = 180 // 更改树的默认宽度，右端要留得下增删行数与更改类型
	treeIndent        = 14  // 文件树每深一层缩进的宽度
	barGap            = 4   // 标题栏右端相邻按钮的间距
	barSlots          = 4   // 标题栏右端为开关留的位置：文件、更改、终端、大纲

	sidebarMin     = 200 // 侧栏可拖动到的最小宽度
	sidebarMax     = 400 // 侧栏可拖动到的最大宽度
	sidebarDefault = 240 // 侧栏默认宽度，双击分隔条恢复
)

// 界面的字号，单位为 DIP。没指定字号的文字用 bodySize，见 applyTheme。
// mygo 的部件按主题字号（即 bodySize）推导自己的字号：侧栏分组标题与提示框小 1，
// 徽标小 2，对话框标题（ui.AlertDialog）大 1。改 bodySize 时它们跟着变。
const (
	titleSize  = 22           // 空会话的标题
	dialogSize = bodySize + 1 // 对话框的标题，与 ui.AlertDialog 的标题一样大
	bodySize   = 14           // 正文：对话、输入框、标题栏与侧栏，以及步骤标题
	textSize   = 13           // 面板、列表、标签、补全里的文字
	smallSize  = 12           // 状态栏、时间、说明、徽标
	codeSize   = 12           // 等宽文字：代码、命令输出、编辑器
)

// monoFamily 是等宽字体：JetBrainsMono Nerd Font Mono，未安装时退回系统等宽字体。
const monoFamily = "JetBrainsMono Nerd Font Mono, monospace"

// 图标的尺寸，单位为 DIP。
const (
	iconSize      = 16 // 按钮与入口里的图标
	smallIconSize = 14 // 与 textSize 的文字并排的图标：文件类型、展开箭头、目录
	emptyIconSize = 28 // 空状态里的大图标
)

// palette 是一套主题色，取自 GitHub Primer（@primer/primitives）的默认亮色与暗色。
// Primer 把强调色分成两个：accent 是实心填充用的（bgColor-accent-emphasis，上面放白字），
// accentFg 是文字、图标与淡底用的（fgColor-accent）。暗色里两者不同，亮色里相同；
// running 取 fgColor-attention，因为它用作文字。
type palette struct {
	canvas, subtle           ui.Color // 主区底色、次级底色（侧栏、用户消息）
	hover, pressed           ui.Color // 控件悬停与按下
	border                   ui.Color
	text, muted              ui.Color
	accent, accentHover      ui.Color
	accentPressed, accentFg  ui.Color
	danger, success, running ui.Color
}

var (
	light = palette{
		canvas: ui.Hex("#FFFFFF"), subtle: ui.Hex("#F6F8FA"),
		hover: ui.Hex("#EFF2F5"), pressed: ui.Hex("#E6EAEF"),
		border: ui.Hex("#D1D9E0"),
		text:   ui.Hex("#1F2328"), muted: ui.Hex("#59636E"),
		accent: ui.Hex("#0969DA"), accentHover: ui.Hex("#0860CA"), accentPressed: ui.Hex("#0550AE"),
		accentFg: ui.Hex("#0969DA"),
		danger:   ui.Hex("#D1242F"), success: ui.Hex("#1A7F37"), running: ui.Hex("#9A6700"),
	}
	dark = palette{
		canvas: ui.Hex("#0D1117"), subtle: ui.Hex("#151B23"),
		hover: ui.Hex("#262C36"), pressed: ui.Hex("#2A313C"),
		border: ui.Hex("#3D444D"),
		text:   ui.Hex("#F0F6FC"), muted: ui.Hex("#9198A1"),
		accent: ui.Hex("#1F6FEB"), accentHover: ui.Hex("#388BFD"), accentPressed: ui.Hex("#1158C7"),
		accentFg: ui.Hex("#4493F8"),
		danger:   ui.Hex("#F85149"), success: ui.Hex("#3FB950"), running: ui.Hex("#D29922"),
	}
)

// accentFg 是当前主题里文字、图标与淡底用的强调色；实心填充用 t.Accent。
func accentFg(t *ui.Theme) ui.Color {
	if t.Dark {
		return dark.accentFg
	}
	return light.accentFg
}

// rowFill 与 buttonFill 是悬停或选中时铺在背景上的淡色：行与下拉触发器用 rowFill，
// 图标按钮与标签用 buttonFill。
func rowFill(t *ui.Theme) ui.Color    { return t.Text.Alpha(0.06) }
func buttonFill(t *ui.Theme) ui.Color { return t.Text.Alpha(0.08) }

// quietRows 让 fn 里创建的列表类部件（Outline、Tree 等）用本应用安静的行样式：选中的行是
// SurfaceHover 的淡底、字色不变，悬停是更淡的 rowFill，与文件树一致。MyGo 的部件在创建时
// 从主题取色，选中行用强调色，所以临时换一份主题。
func quietRows(c *ui.Context, t *ui.Theme, fn func()) {
	local := *t
	local.Accent, local.AccentText = t.SurfaceHover, t.Text
	local.SurfaceHover = rowFill(t)
	c.SetTheme(&local)
	fn()
	c.SetTheme(t)
}

// applyTheme 在系统的亮色或暗色主题上换成 Atlas 的色板与选定的界面字体，返回本帧的主题。
func (a *App) applyTheme(c *ui.Context) *ui.Theme {
	t := *c.Theme()
	t.Font = a.prefs.uiFamily()
	p := light
	if t.Dark {
		p = dark
	}
	t.Background, t.Surface = p.canvas, p.subtle
	t.SurfaceHover, t.SurfacePressed = p.hover, p.pressed
	t.Border, t.Text, t.TextMuted = p.border, p.text, p.muted
	t.Accent, t.AccentHover, t.AccentPressed = p.accent, p.accentHover, p.accentPressed
	t.AccentText = ui.Hex("#FFFFFF")
	t.Danger, t.Success, t.Warning = p.danger, p.success, p.running
	t.Focus, t.Selection = p.accent, p.accent.Alpha(0.25)
	t.Radius, t.FontSize = controlRound, bodySize
	// 链接是文字，用文字的强调色；MyGo 默认取 t.Accent，暗色下对比度不够。
	for _, theme := range []*markdown.Theme{a.md.Theme, a.mdNote.Theme, a.mdFile.Theme} {
		theme.Link = p.accentFg
	}
	c.SetTheme(&t)
	return &t
}
