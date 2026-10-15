package gui

import "github.com/egoist/mygo/ui"

// 界面的尺寸约定，单位为 DIP。
const (
	contentWidth = 720 // 阅读栏的最大宽度
	bubbleWidth  = 680 // 用户消息的最大宽度
	bodySize     = 15  // 对话正文字号
	controlRound = 6   // 控件圆角
	inputRound   = 16  // 输入框圆角
	cardRound    = 12  // 浮动卡片圆角
	edge         = 6   // 浮动卡片与窗口边缘的空隙
	handleWidth  = 6   // 卡片拖动条的宽度
	barButton    = 24  // 标题栏按钮的边长
	tabHeight    = 24  // 终端标签的高度，与标题栏按钮同高
	tabInset     = 4   // 标签栏左右的内边距，加边框后与标签上方的空隙相等
	titleBarMin  = 44  // 标题栏的最小高度，交通灯在其中垂直居中
	gutter       = 20  // 阅读区两侧的留白

	termMin     = 240 // 终端面板的最小宽度
	termDefault = 420 // 终端面板的默认宽度
	chatMin     = 320 // 打开终端面板时对话区的最小宽度

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
