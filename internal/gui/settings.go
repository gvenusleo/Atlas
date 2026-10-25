package gui

import (
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/gvenusleo/atlas/internal/config"
)

// defaultFont 是字体框里代表“用默认字体”的一项，对应偏好里的空串。
const defaultFont = "默认"

// themeChoice 是主题分段控件的一项：图标显示在控件上，名称用于悬停提示与无障碍。
type themeChoice struct {
	label  string
	icon   *ui.SVG
	source mygo.ThemeSource
}

// themeChoices 是主题分段控件的各项，次序与界面上的一致；跟随系统对应空的主题。
var themeChoices = []themeChoice{
	{"浅色", sunIcon, mygo.ThemeLight},
	{"深色", moonIcon, mygo.ThemeDark},
	{"跟随系统", monitorIcon, ""},
}

// themeIndex 是主题 source 在 themeChoices 里的位置。
func themeIndex(source mygo.ThemeSource) int {
	return slices.IndexFunc(themeChoices, func(c themeChoice) bool { return c.source == source })
}

// loadAppPrefs 读取配置目录下的偏好；找不到配置目录时用默认值。
func loadAppPrefs() prefs {
	dir, err := config.Dir()
	if err != nil {
		return prefs{}
	}
	return loadPrefs(dir)
}

// setPrefs 换上新的偏好并立即生效，随后保存；保存失败只提示，偏好在本次运行里仍有效。
// 文件很小，在主线程同步写，连续的更改才不会乱序。
func (a *App) setPrefs(p prefs) {
	if source := p.Theme; source != a.prefs.Theme {
		// SetSource 会通知窗口重新取主题，所以不在构建视图的过程中调用，放到这一帧之后。
		a.win.Update(func() { mygo.Theme.SetSource(source) })
	}
	a.prefs = p
	a.applyFonts()
	dir, err := config.Dir()
	if err == nil {
		err = savePrefs(dir, p)
	}
	if err != nil {
		a.toast = "保存偏好失败：" + err.Error()
	}
}

// applyFonts 换上代码字体：全局的 codeFont 与终端的字体，以及 Markdown 的代码；编辑器与
// 对比的字体在画它们时从 codeFont 取。界面字体在每帧的主题里取（见 applyTheme）。
// 已打开的终端保持原来的字体，新开的终端用新字体。
func (a *App) applyFonts() {
	codeFont.Family = a.prefs.codeFamily()
	termFont.Family = codeFont.Family
	for _, theme := range []*markdown.Theme{a.md.Theme, a.mdNote.Theme, a.mdFile.Theme} {
		theme.CodeFont = codeFont.Family
	}
}

// settingsPage 是设置页的状态。
type settingsPage struct {
	open     bool
	gen      int // 打开的次数，后台取系统字体的结果据此作废
	category int // 左侧选中的分类，目前只有“外观”
	theme    int // 主题分段控件选中的一项，见 themeChoices
	// 两个字体框里显示的字体名，默认的显示为 defaultFont。
	uiFont, codeFont string
	// 字体框的选项，默认的在最前；系统字体取回之前只有默认的。
	uiFonts, codeFonts []string
}

// openSettings 打开设置页，各项取自当前的偏好，并在后台取系统字体的列表。
func (a *App) openSettings() {
	s := &a.settings
	*s = settingsPage{
		open: true, gen: s.gen + 1,
		theme:  themeIndex(a.prefs.Theme),
		uiFont: fontLabel(a.prefs.UIFont), codeFont: fontLabel(a.prefs.CodeFont),
		uiFonts: []string{defaultFont}, codeFonts: []string{defaultFont},
	}
	a.loadFonts(s.gen)
}

// loadFonts 在后台列出系统字体：界面字体可以选全部，代码字体只能选等宽的。
// 第一次列要几十毫秒；打开之后又打开过一次的（gen 对不上），结果作废。
func (a *App) loadFonts(gen int) {
	go func() {
		var all, mono []string
		for _, f := range ui.FontFamilies() {
			all = append(all, f.Name)
			if f.Monospaced {
				mono = append(mono, f.Name)
			}
		}
		a.win.Update(func() {
			if s := &a.settings; s.gen == gen {
				s.uiFonts = slices.Concat([]string{defaultFont}, all)
				s.codeFonts = slices.Concat([]string{defaultFont}, mono)
			}
		})
	}()
}

// fontLabel 是偏好里的字体在字体框里的显示：空串显示为 defaultFont。
func fontLabel(name string) string {
	if name == "" {
		return defaultFont
	}
	return name
}

// fontPref 是字体框里的显示对应的偏好：defaultFont 对应空串。
func fontPref(label string) string {
	if label == defaultFont {
		return ""
	}
	return label
}

// chooseTheme 选了主题分段控件的第 i 项，立即生效并保存。
func (a *App) chooseTheme(i int) {
	p := a.prefs
	p.Theme = themeChoices[i].source
	a.setPrefs(p)
}

// chooseFonts 在字体框的选择变了之后，把两个字体框的内容存进偏好。
func (a *App) chooseFonts() {
	p := a.prefs
	p.UIFont, p.CodeFont = fontPref(a.settings.uiFont), fontPref(a.settings.codeFont)
	a.setPrefs(p)
}
