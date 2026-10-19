package gui

import "github.com/egoist/mygo/ui"

// 界面的尺寸约定，单位为 DIP。
const (
	contentWidth = 720 // 阅读栏的最大宽度
	bubbleWidth  = 680 // 用户消息的最大宽度
	bodySize     = 15  // 对话正文字号
	stepSize     = 14  // 思考与工具步骤标题的字号
	controlRound = 6   // 控件圆角
	boxRound     = 8   // 代码框与对比框的圆角
	inputRound   = 16  // 输入框圆角
	cardRound    = 12  // 浮动卡片圆角
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

	treeMin       = 140 // 文件树的最小宽度
	treeDefault   = 200 // 文件树的默认宽度
	treeRowHeight = 24  // 文件树一行的高度
	treeIndent    = 14  // 文件树每深一层缩进的宽度
	fileIconSize  = 14  // 文件类型图标的边长
	barGap        = 4   // 标题栏右端相邻按钮的间距

	sidebarMin     = 200 // 侧栏可拖动到的最小宽度
	sidebarMax     = 400 // 侧栏可拖动到的最大宽度
	sidebarDefault = 240 // 侧栏默认宽度，双击分隔条恢复
)

// palette 是一套主题色，取自 GitHub Primer 的默认亮色与暗色。
type palette struct {
	canvas, subtle           ui.Color // 主区底色、次级底色（侧栏、用户消息）
	hover, pressed           ui.Color // 控件悬停与按下
	border                   ui.Color
	text, muted              ui.Color
	accent, accentHover      ui.Color
	accentPressed            ui.Color
	danger, success, running ui.Color
}

var (
	light = palette{
		canvas: ui.Hex("#FFFFFF"), subtle: ui.Hex("#F6F8FA"),
		hover: ui.Hex("#EAEEF2"), pressed: ui.Hex("#D0D7DE"),
		border: ui.Hex("#D0D7DE"),
		text:   ui.Hex("#1F2328"), muted: ui.Hex("#59636E"),
		accent: ui.Hex("#0969DA"), accentHover: ui.Hex("#0860CA"), accentPressed: ui.Hex("#0550AE"),
		danger: ui.Hex("#D1242F"), success: ui.Hex("#1A7F37"), running: ui.Hex("#BF8700"),
	}
	dark = palette{
		canvas: ui.Hex("#0D1117"), subtle: ui.Hex("#151B23"),
		hover: ui.Hex("#262C36"), pressed: ui.Hex("#3D444D"),
		border: ui.Hex("#3D444D"),
		text:   ui.Hex("#F0F6FC"), muted: ui.Hex("#9198A1"),
		accent: ui.Hex("#1F6FEB"), accentHover: ui.Hex("#388BFD"), accentPressed: ui.Hex("#1158C7"),
		danger: ui.Hex("#F85149"), success: ui.Hex("#3FB950"), running: ui.Hex("#D29922"),
	}
)

// rowFill 与 buttonFill 是悬停或选中时铺在背景上的淡色：行与下拉触发器用 rowFill，
// 图标按钮与标签用 buttonFill。
func rowFill(t *ui.Theme) ui.Color    { return t.Text.Alpha(0.06) }
func buttonFill(t *ui.Theme) ui.Color { return t.Text.Alpha(0.08) }

// applyTheme 在系统的亮色或暗色主题上换成 Atlas 的色板，返回本帧的主题。
func applyTheme(c *ui.Context) *ui.Theme {
	t := *c.Theme()
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
	t.Radius, t.FontSize = controlRound, 14
	c.SetTheme(&t)
	return &t
}
