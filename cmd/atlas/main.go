// Command atlas 启动 Atlas 桌面应用。
package main

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type app struct {
	name  string
	count int
}

// view 构建 Atlas 主窗口的示例界面。
func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(16).Padding(24).Children(func() {
		ui.Text(c, "mygo").FontSize(26).Bold()
		ui.TextInput(c, &a.name).Placeholder("Your name").Label("Name").Width(240)
		greeting := "Hello!"
		if a.name != "" {
			greeting = fmt.Sprintf("Hello, %s!", a.name)
		}
		ui.Text(c, greeting).TextColor(t.TextMuted)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "−").Width(44).Clicked() {
				a.count--
			}
			ui.Textf(c, "%d", a.count).FontSize(20).Width(48).TextAlign(ui.Center)
			if ui.PrimaryButton(c, "+").Width(44).Clicked() {
				a.count++
			}
		})
	})
}

// main 创建并启动 Atlas 主窗口。
func main() {
	a := &app{}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "Atlas",
			Width:     480,
			Height:    360,
			MinWidth:  320,
			MinHeight: 280,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
