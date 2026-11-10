package gui

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
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

// palette 是一套主题色。强调色分成两个：accent 是实心填充用的，上面放 onAccent 的字；
// accentFg 是文字、图标与链接用的。暗色主题里两者通常不是同一个色。悬停与按下的强调色
// 由 accent 推出，见 applyTheme。running 用作文字（任务进行中），所以取的是能当文字看的那个黄。
type palette struct {
	canvas, subtle           ui.Color // 主区底色、次级底色（侧栏、用户消息）；亮色里次级底色更深
	border                   ui.Color
	text, muted              ui.Color
	accent, accentFg         ui.Color
	onAccent                 ui.Color // 放在 accent 上的文字
	danger, success, running ui.Color
}

// scheme 是一套配色：亮暗两个应用色板、两组终端的 16 色，以及两组代码高亮色。
// 终端的前景、底色与选区取自 palette，不另存；代码高亮的六种颜色是单独调的，
// 保证在各自底色上都能当文字看，所以与 16 色里的同色系不必相等。
type scheme struct {
	light, dark         palette
	lightANSI, darkANSI [16]ui.Color
	// tokens 依次是关键字、字符串、数字、函数、类型与标签的颜色。
	lightTokens, darkTokens [6]ui.Color
}

// activeScheme 是当前配色，只经 useScheme 在主线程改写；样式函数遍布各文件，
// 不便都改成 App 的方法，所以放在包里。
var activeScheme = schemeChoices[schemeIndex(defaultScheme)].scheme

// useScheme 换用 id 对应的配色；不认识时用默认配色。
func useScheme(id string) {
	activeScheme = schemeChoices[schemeIndex(id)].scheme
}

// accentFg 是当前主题里文字、图标与淡底用的强调色；实心填充用 t.Accent。
func accentFg(t *ui.Theme) ui.Color {
	if t.Dark {
		return activeScheme.dark.accentFg
	}
	return activeScheme.light.accentFg
}

// 铺在背景上的淡色，由浅到深：rowFill 是行与下拉触发器的悬停（也是淡底按钮的静止态），
// buttonFill 是图标按钮与标签的悬停，selectedFill 是侧栏与设置分类里选中的一项，pressFill 是
// 按下；activeFill 是开着的开关与选中的档位，用淡的强调色；fieldBorder 是输入框的细边框。
func rowFill(t *ui.Theme) ui.Color      { return t.Text.Alpha(0.06) }
func buttonFill(t *ui.Theme) ui.Color   { return t.Text.Alpha(0.08) }
func selectedFill(t *ui.Theme) ui.Color { return t.Text.Alpha(0.09) }
func pressFill(t *ui.Theme) ui.Color    { return t.Text.Alpha(0.12) }
func activeFill(t *ui.Theme) ui.Color   { return accentFg(t).Alpha(0.15) }
func fieldBorder(t *ui.Theme) ui.Color  { return t.Text.Alpha(0.12) }

// quietRows 让 fn 里创建的列表类部件（Outline、Tree 等）用本应用安静的行样式：选中的行是
// SurfaceHover 的淡底、字色不变，悬停是更淡的 rowFill，与文件树一致。MyGo 的部件在创建时
// 从主题取色，选中行用强调色，所以临时换一份主题。InactiveAccent 设成同一个淡底：留零时
// 窗口不在前台，MyGo 会把选中行换成由 AccentText（这里是正文色）推出的中灰，反而更重。
func quietRows(c *ui.Context, t *ui.Theme, fn func()) {
	local := *t
	local.Accent, local.AccentText = t.SurfaceHover, t.Text
	local.InactiveAccent = local.Accent
	local.SurfaceHover = rowFill(t)
	c.SetTheme(&local)
	fn()
	c.SetTheme(t)
}

// applyTheme 在系统的亮色或暗色主题上换成当前配色的色板与选定的界面字体，返回本帧的主题。
// 除 Dark（来自系统或偏好）与 ScrollbarStyle（留零即按 Scrollbar 与 ScrollbarWidth 画）之外，
// 每个字段都显式填上，不靠 MyGo 的默认值；InactiveAccent 显式留零，原因见下。
func (a *App) applyTheme(c *ui.Context) *ui.Theme {
	t := *c.Theme()
	p := activeScheme.light
	if t.Dark {
		p = activeScheme.dark
	}
	t.Background, t.Surface = p.canvas, p.subtle
	// 悬停与按下的底色是文字色叠在底色上：暗色里侧栏等次级底色比主区亮，
	// 叠得太淡就与它分不开，所以暗色叠得更多。
	hover, pressed := float32(0.08), float32(0.12)
	if t.Dark {
		hover, pressed = 0.12, 0.16
	}
	t.SurfaceHover = p.text.Alpha(hover).Over(p.canvas)
	t.SurfacePressed = p.text.Alpha(pressed).Over(p.canvas)
	t.Border, t.Text, t.TextMuted = p.border, p.text, p.muted
	// 悬停与按下的强调色由 accent 推出，各配色的手感因此一致：亮色里悬停变深、按下更深；
	// 暗色里悬停变亮、按下变深，与 MyGo 的默认主题相同。
	black, white := ui.RGB(0, 0, 0), ui.RGB(255, 255, 255)
	t.Accent = p.accent
	t.AccentHover, t.AccentPressed = p.accent.Mix(black, 0.1), p.accent.Mix(black, 0.2)
	if t.Dark {
		t.AccentHover, t.AccentPressed = p.accent.Mix(white, 0.12), p.accent.Mix(black, 0.15)
	}
	t.AccentText = p.onAccent
	t.Danger, t.Warning, t.Success = p.danger, p.running, p.success
	// 留零时 MyGo 取一种与 AccentText 对比够读的灰，窗口不在前台时强调色褪成它，与原生应用
	// 一致；从色板里另挑反而顾不全各配色的 AccentText。
	t.InactiveAccent = ui.Transparent
	t.Selection, t.Focus = p.accent.Alpha(0.25), p.accent
	t.FocusRingWidth, t.FocusRingOffset = 2, 1
	t.Inverse, t.InverseText = p.text, p.canvas
	t.Scrollbar, t.ScrollbarWidth = p.text.Alpha(0.32), 6
	t.Radius, t.Spacing, t.FontSize = controlRound, 4, bodySize
	t.TooltipDelay = 600 * time.Millisecond
	t.Font = a.prefs.uiFamily()
	// 链接与代码高亮都跟着配色走；MyGo 默认拿 t.Accent 当链接色，暗色下对比度不够。
	// 内容没变就沿用旧指针：Markdown 按指针判断主题变没变，每帧换新指针会让它每帧重算，
	// 而内容变了必须换指针，它才会重新高亮。
	if code := codeTheme(&t); a.code == nil || *a.code != *code {
		a.code = code
	}
	for _, theme := range []*markdown.Theme{a.md.Theme, a.mdNote.Theme, a.mdFile.Theme} {
		theme.Link, theme.CodeColors = p.accentFg, a.code
	}
	c.SetTheme(&t)
	return &t
}

// boxColor 是代码框的底色：文字色淡淡地铺在背景上。
func boxColor(t *ui.Theme) ui.Color { return t.Text.Alpha(0.04).Over(t.Background) }

// codeTheme 是代码与对比的配色。编辑器主题的每个字段都显式填上，不靠 MyGo 的默认值；
// 与配色无关的装饰色（当前行、括号、搜索命中）只随亮暗变，用黑白的透明度。
// 对比里的增删改取终端的绿、红与蓝：MyGo 把它们按比例混进底色，要用饱和的亮色才看得出；
// 配色的 success 与 danger 是给文字调的，混进去几乎没有颜色。见 diffColor。
func codeTheme(t *ui.Theme) *editor.Theme {
	tok, ansi := activeScheme.lightTokens, activeScheme.lightANSI
	if t.Dark {
		tok, ansi = activeScheme.darkTokens, activeScheme.darkANSI
	}
	th := &editor.Theme{
		Background: boxColor(t),
		Text:       t.Text,

		LineNumber:        t.TextMuted.Alpha(0.75),
		ActiveLineNumber:  t.Text,
		ActiveLine:        ui.RGBA(0, 0, 0, 0.035),
		Selection:         t.Selection,
		InactiveSelection: t.Selection.Alpha(0.55),
		Cursor:            t.Accent,
		MatchingBracket:   ui.RGBA(0, 0, 0, 0.3),
		Match:             ui.RGBA(255, 200, 0, 0.3),
		CurrentMatch:      ui.RGBA(255, 140, 0, 0.45),
		Occurrence:        ui.RGBA(0, 0, 0, 0.07),
		IndentGuide:       t.Border.Alpha(0.55),
		ActiveIndentGuide: t.TextMuted.Alpha(0.6),
		WrapGuide:         t.Border.Alpha(0.6),
		Whitespace:        t.TextMuted.Alpha(0.45),

		Error:   t.Danger,
		Warning: t.Warning,
		Info:    accentFg(t),
		Hint:    t.TextMuted,

		Added:    diffColor(t, ansi[2], ansi[10]),
		Deleted:  diffColor(t, ansi[1], ansi[9]),
		Modified: diffColor(t, ansi[4], ansi[12]),
	}
	if t.Dark {
		th.ActiveLine = ui.RGBA(255, 255, 255, 0.045)
		th.MatchingBracket = ui.RGBA(255, 255, 255, 0.35)
		th.Match = ui.RGBA(255, 210, 60, 0.22)
		th.CurrentMatch = ui.RGBA(255, 160, 40, 0.5)
		th.Occurrence = ui.RGBA(255, 255, 255, 0.1)
	}
	setTokens(th, t, tok)
	return th
}

// diffColor 从终端的一对常规色与亮色里挑对比用的那个：亮色主题取亮色，暗色主题取两者中更亮
// 的。多数配色的亮色更亮，但 Flexoki 暗色的亮色反而更暗，混进暗底就看不见了。
func diffColor(t *ui.Theme, normal, bright ui.Color) ui.Color {
	if t.Dark && lightness(normal) > lightness(bright) {
		return normal
	}
	return bright
}

// lightness 是颜色的相对亮度（Rec. 709 的权重），只用来比较两个颜色谁更亮。
func lightness(c ui.Color) float32 {
	return 0.2126*float32(c.R) + 0.7152*float32(c.G) + 0.0722*float32(c.B)
}

// setTokens 给 25 种 token 全部显式设色，tok 是配色的六种代码色。
// Emphasis 斜体、Strong 与 Heading 的粗体是字形，不在颜色里，单独设。
func setTokens(th *editor.Theme, t *ui.Theme, tok [6]ui.Color) {
	th.Tokens[editor.TokenText].Color = t.Text
	th.Tokens[editor.TokenVariable].Color = t.Text
	th.Tokens[editor.TokenPunctuation].Color = t.Text
	th.Tokens[editor.TokenEmphasis].Color = t.Text
	th.Tokens[editor.TokenStrong].Color = t.Text
	th.Tokens[editor.TokenKeyword].Color = tok[0]
	th.Tokens[editor.TokenOperator].Color = tok[0]
	th.Tokens[editor.TokenString].Color = tok[1]
	th.Tokens[editor.TokenLink].Color = tok[1]
	th.Tokens[editor.TokenCode].Color = tok[1]
	th.Tokens[editor.TokenInserted].Color = t.Success
	th.Tokens[editor.TokenNumber].Color = tok[2]
	th.Tokens[editor.TokenConstant].Color = tok[2]
	th.Tokens[editor.TokenProperty].Color = tok[2]
	th.Tokens[editor.TokenAttribute].Color = tok[2]
	th.Tokens[editor.TokenEscape].Color = tok[2]
	th.Tokens[editor.TokenHeading].Color = tok[2]
	th.Tokens[editor.TokenFunction].Color = tok[3]
	th.Tokens[editor.TokenMeta].Color = tok[3]
	th.Tokens[editor.TokenType].Color = tok[4]
	th.Tokens[editor.TokenTag].Color = tok[5]
	th.Tokens[editor.TokenRegexp].Color = tok[5]
	th.Tokens[editor.TokenComment].Color = t.TextMuted
	th.Tokens[editor.TokenDeleted].Color = t.Danger
	th.Tokens[editor.TokenInvalid].Color = t.Danger
	th.Tokens[editor.TokenEmphasis].Italic = true
	th.Tokens[editor.TokenStrong].Bold = true
	th.Tokens[editor.TokenHeading].Bold = true
}
