package gui

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
)

// 这个文件管应用在后台运行：托盘图标，以及关闭窗口只隐藏窗口。设计见设计文档的“gui”一节。

// trayIcon 是托盘的模板图标，由 cmd/icon 生成。
//
//go:embed tray.png
var trayIcon []byte

// startTray 创建托盘图标，菜单项的点击回到主线程处理。建不起来时记一条日志，a.tray 留空。
func (a *App) startTray() {
	onMain := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			if a.win != nil { // 窗口还没建好就点了
				a.win.Update(fn)
			}
		}
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开 Atlas", Click: onMain(a.reveal)},
		{Label: "设置", Click: onMain(func() {
			a.reveal()
			a.openSettings()
		})},
		mygo.Separator(),
		{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID: "main", Icon: trayIcon, IconIsTemplate: true, ToolTip: "Atlas", Menu: menu,
	})
	if err != nil {
		log.Printf("创建托盘失败: %v", err)
		return
	}
	a.tray = tray
}

// reveal 显示并聚焦主窗口：窗口可能是隐藏的，也可能被最小化了。
func (a *App) reveal() {
	a.win.Show()
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Focus()
}

// beforeQuit 在退出开始、窗口关闭之前先确认；确认通过后记下正在退出，之后窗口的关闭才不会
// 被当作隐藏而拦下。
func (a *App) beforeQuit(e *mygo.QuitEvent) {
	a.confirmQuit(&e.Preventable)
	a.quitting = !e.DefaultPrevented()
}

// closeWindow 处理用户关闭窗口：只隐藏窗口，应用、核心与定时任务继续运行，有未保存修改的
// 文件也不询问；退出过程中关闭的窗口放行。回到窗口靠 Dock 图标或托盘。
func (a *App) closeWindow(e *mygo.CloseEvent) {
	if a.quitting {
		return
	}
	e.PreventDefault()
	a.win.Hide()
}

// syncTray 让托盘图标与偏好一致：该显示而没有就建，不该显示而有就拆。
func (a *App) syncTray() {
	switch {
	case a.prefs.HideTray && a.tray != nil:
		a.tray.Destroy()
		a.tray = nil
	case !a.prefs.HideTray && a.tray == nil:
		a.startTray()
	}
}
