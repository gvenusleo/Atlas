package gui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// composer 构建输入区：工作目录标签和圆角输入框；聚焦时边框与光环为强调色。
func (a *App) composer(c *ui.Context, t *ui.Theme, ch *chat) {
	ui.Column(c).Gap(4).Children(func() {
		a.folder(c, t, ch)
		card := ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(5, 5, 5, 10).Radius(8).
			Background(t.Background).Border(1, t.Border)
		if ch.focus.FocusWithin(c) {
			card.Border(1, t.Accent).Shadow(0, 0, 0, 3, t.Accent.Alpha(0.3))
		}
		card.Children(func() {
			input := ui.TextAreaBase(c.Key("input-"+ch.key), &ch.input).Bind(&ch.focus).
				Placeholder("发送消息").Lines(1, 10).Grow(1).Padding(5, 0).Disabled(ch.loading).
				OnTextKey(func(k ui.TextKey) bool {
					if k.Key != ui.KeyEnter || k.Mods != 0 {
						return false
					}
					if a.send(ch, k.Text()) {
						k.SetText("")
					}
					return true
				})
			if !a.keyNav {
				input.AutoFocus()
			}
			a.sendButton(c, ch)
		})
	})
}

// sendButton 是输入框右下角的按钮：有内容时发送，运行时停止，空闲且为空时为灰色。
func (a *App) sendButton(c *ui.Context, ch *chat) {
	running := ch.running
	glyph, label := sendIcon, "发送"
	if running {
		glyph, label = stopIcon, "停止"
	}
	empty := strings.TrimSpace(ch.input) == ""
	if !running && empty || ch.loading {
		ui.Button(c, "").Size(30, 30).Padding(0).AlignSelf(ui.End).Label(label).Tooltip(label).
			Disabled(true).Children(func() {
			ui.Icon(c, glyph).FontSize(18)
		})
		return
	}
	ui.PrimaryButton(c, "").Size(30, 30).Padding(0).AlignSelf(ui.End).Label(label).Tooltip(label).
		Disabled(running && ch.session == nil).Children(func() {
		ui.Icon(c, glyph).FontSize(18)
	}).OnClick(func() {
		switch {
		case running:
			ch.session.Cancel()
		case a.send(ch, ch.input):
			ch.input = ""
		}
	})
}

// folder 显示会话的工作目录；尚未创建的会话可以点击更换。
func (a *App) folder(c *ui.Context, t *ui.Theme, ch *chat) {
	var row ui.Element
	if ch.id == "" && !ch.running {
		row = ui.ButtonBase(c).Cursor(ui.CursorPointer).Label("更换工作目录")
		if row.Hovered() {
			row.Background(t.Text.Alpha(0.05))
		}
		row.OnClick(func() { a.pickDir(ch) })
	} else {
		row = ui.Row(c)
	}
	row.Gap(6).AlignItems(ui.Center).Padding(2, 6).Radius(controlRound).AlignSelf(ui.Start)
	row.Children(func() {
		ui.Icon(c, folderIcon).FontSize(14).TextColor(t.TextMuted)
		ui.Text(c, shortPath(ch.cwd)).FontSize(12).TextColor(t.TextMuted).SingleLine()
	})
}

// shortPath 把主目录缩写为 ~。
func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "~"
		}
		return filepath.Join("~", rel)
	}
	return path
}
