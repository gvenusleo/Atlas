package gui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/config"
)

// serviceFormWidth 是新建与编辑模型服务的对话框的宽度。
const serviceFormWidth = 560

// modelSettings 画“模型接入”：默认模型，以及模型服务的列表。
func (a *App) modelSettings(c *ui.Context, t *ui.Theme) {
	p := &a.settings.models
	if p.cfg == nil {
		ui.Text(c, "没能读取模型设置：请先修正 ~/.atlas/settings.json。\n"+p.loadErr).
			FontSize(textSize).TextColor(t.Danger).Selectable()
		return
	}
	settingRow(c, "默认模型", func() {
		dropdown(c, t, "默认模型", &p.pick, p.modelRefs(), p.modelLabel, false,
			a.chooseDefaultModel)
	})
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "模型服务").Grow(1).MinWidth(0)
		fillButton(c, t, plusIcon, "添加服务", func() { p.form = newServiceForm() })
	})
	ui.Column(c).Gap(8).Children(func() {
		for _, name := range p.providerNames() {
			a.serviceRow(c, t, name)
		}
	})
	if len(p.cfg.Providers) == 0 {
		ui.Text(c, "还没有模型服务。添加一个，填上接口地址和 API key，再声明它的模型。").
			FontSize(textSize).TextColor(t.TextMuted)
	}
	if p.saveErr != "" {
		ui.Text(c, p.saveErr).FontSize(textSize).TextColor(t.Danger)
	}
}

// serviceRow 画服务列表里的一行：名称、接口类型与模型数，行尾是 API key 的状态；点一下编辑。
func (a *App) serviceRow(c *ui.Context, t *ui.Theme, name string) {
	p := &a.settings.models
	prov := p.cfg.Providers[name]
	row := ui.ButtonBase(c.Key("service-"+name)).AlignItems(ui.Center).Gap(10).Padding(10, 14).
		Radius(controlRound + 2).Background(rowFill(t)).Label(name)
	if row.Hovered() {
		row.Background(buttonFill(t))
	}
	row.OnClick(func() { a.editService(name) })
	dot, status := t.Warning, "没有 API key"
	if p.keys[name] {
		dot, status = t.Success, "已设置 API key"
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, name).FontSize(textSize).SingleLine()
			ui.Text(c, wireAPIName(prov.WireAPI)+" · "+strconv.Itoa(len(prov.Models))+" 个模型").
				FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
		})
		ui.Box(c).Size(7, 7).Shrink(0).Radius(3.5).Background(dot)
		ui.Text(c, status).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
		ui.Icon(c, chevronIcon).FontSize(smallIconSize).TextColor(t.TextMuted)
	})
}

// wireAPIName 是接口类型的显示名。
func wireAPIName(api config.WireAPI) string {
	switch api {
	case config.WireAPIOpenAIResponses:
		return "OpenAI Responses"
	case config.WireAPIOpenAICompletions:
		return "OpenAI Chat Completions"
	case "":
		return "未设置接口类型"
	}
	return string(api)
}

// serviceDialog 在新建或编辑模型服务时画对话框，结构与任务表单的一样：标题与关闭，表单，底部是
// 错误与取消、保存。对话框自己关掉（按 Esc、点遮罩）时丢弃表单。
func (a *App) serviceDialog(c *ui.Context, t *ui.Theme) {
	p := &a.settings.models
	open := p.form != nil
	width, height := c.Size()
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		sheetPanel(backdrop, panel, t, min(serviceFormWidth, width-2*gutter)).
			MaxHeight(height - 2*gutter).Clip()
		ui.Column(c).Grow(1).MinHeight(0).Padding(settingsTop, 0, 0).Children(func() {
			title := "添加模型服务"
			if p.form.orig != "" {
				title = "编辑模型服务"
			}
			dialogHeader(c, t, title, func() { p.form = nil })
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { a.serviceFields(c, t) })
			dialogButtons(c, t, p.form.err, func() { a.deleteButton(c, t) },
				func() { p.form = nil }, a.saveService)
		})
	})
	if !open {
		p.form = nil
	}
}

// serviceFields 画服务表单：名称、接口类型、接口地址、API key，以及这个服务的模型。
func (a *App) serviceFields(c *ui.Context, t *ui.Theme) {
	p := &a.settings.models
	f := p.form
	ui.Column(c).Gap(6).Padding(settingsGap, settingsPad, settingsPad).Children(func() {
		fieldRow(c, "名称", func() {
			textField(c, t, "service-name", &f.name, "例如：openai").Grow(1).MinWidth(0)
		})
		fieldRow(c, "接口类型", func() {
			pillSelect(c, t, "接口类型", &f.api, f.apis, wireAPIName)
		})
		fieldRow(c, "接口地址", func() {
			textField(c, t, "service-url", &f.baseURL, "https://api.example.com/v1").
				Grow(1).MinWidth(0)
		})
		fieldRow(c, "API key", func() {
			// 输入框与右边的显示开关挨得近一些，不用表单行的间距。
			ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(4).Children(func() {
				input := textField(c, t, "service-key", &f.key, "未设置，保存在 ~/.atlas/auth.json")
				if !f.showKey {
					input.Password()
				}
				input.Grow(1).MinWidth(0)
				label, glyph := "显示 API key", eyeIcon
				if f.showKey {
					label, glyph = "隐藏 API key", eyeOffIcon
				}
				quietButton(c, t, label, func() { f.showKey = !f.showKey }).
					Size(fieldHeight, fieldHeight).Shrink(0).Children(func() {
					ui.Icon(c, glyph).FontSize(iconSize).TextColor(t.TextMuted)
				})
			})
		})
		ui.Row(c).AlignItems(ui.Center).Padding(14, 0, 2).Children(func() {
			ui.Text(c, "模型").Bold().Grow(1).MinWidth(0)
			fillButton(c, t, plusIcon, "添加模型", func() {
				f.models = append(f.models, f.blankModel())
			})
		})
		if len(f.models) == 0 {
			ui.Text(c, "还没有模型。点“添加模型”声明一个，没有声明的模型选不到。").
				FontSize(textSize).TextColor(t.TextMuted)
		}
		ui.Column(c).Gap(8).Children(func() {
			for _, m := range f.models {
				a.modelCard(c, t, f, m)
			}
		})
	})
}

// modelCard 画服务表单里的一个模型：收起时是一行摘要，点一下展开成各个字段；右边的叉删除它。
func (a *App) modelCard(c *ui.Context, t *ui.Theme, f *serviceForm, m *modelForm) {
	key := "model-" + strconv.Itoa(m.key) + "-"
	card := ui.Column(c).Radius(controlRound + 2).Background(rowFill(t))
	card.Children(func() {
		// 悬停的底色铺满整行，包括右边的删除按钮。
		header := ui.Row(c).AlignItems(ui.Center).Padding(0, 6, 0, 0).Radius(controlRound + 2)
		if header.Hovered() {
			header.Background(buttonFill(t))
		}
		header.Children(func() {
			a.modelHead(c, t, m, key)
			quietButton(c, t, "删除模型", func() { f.removeModel(m) }).Size(barButton, barButton).
				Shrink(0).Children(func() {
				ui.Icon(c, closeIcon).FontSize(barButton * 0.6).TextColor(t.TextMuted)
			})
		})
		if m.open {
			ui.Column(c).Gap(6).Padding(2, 14, 14).Children(func() { a.modelFields(c, t, m, key) })
		}
	})
}

// modelHead 画模型卡片收起时的一行：向右（展开时向下）的箭头，模型 ID，有展示名时其下是展示名。
func (a *App) modelHead(c *ui.Context, t *ui.Theme, m *modelForm, key string) {
	title := strings.TrimSpace(m.id)
	if title == "" {
		title = "新模型"
	}
	head := ui.ButtonBase(c.Key(key+"head")).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(10).
		Padding(10, 12).Radius(controlRound + 2).Label(title)
	head.OnClick(func() { m.open = !m.open })
	head.Children(func() {
		arrow := ui.Icon(c, chevronIcon).FontSize(smallIconSize).TextColor(t.TextMuted)
		if m.open {
			arrow.Rotate(90)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(textSize).Bold().SingleLine()
			if name := strings.TrimSpace(m.name); name != "" {
				ui.Text(c, name).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
			}
		})
	})
}

// modelFields 画展开的模型的各个字段。
func (a *App) modelFields(c *ui.Context, t *ui.Theme, m *modelForm, key string) {
	fieldRow(c, "模型 ID", func() {
		textField(c, t, key+"id", &m.id, "例如：gpt-5").Grow(1).MinWidth(0)
	})
	fieldRow(c, "展示名", func() {
		textField(c, t, key+"name", &m.name, "不填就用模型 ID").Grow(1).MinWidth(0)
	})
	fieldRow(c, "上下文窗口", func() {
		textField(c, t, key+"context", &m.context, "token 数").Width(150)
	})
	fieldRow(c, "最大输出", func() {
		textField(c, t, key+"output", &m.output, "token 数").Width(150)
	})
	fieldRow(c, "接受图片", func() { ui.Switch(c.Key(key+"images"), &m.images).Label("接受图片") })
	fieldRow(c, "思考档位", func() {
		ui.Row(c).Wrap().Gap(6).Grow(1).MinWidth(0).Padding(4, 0).Children(func() {
			for i, level := range thinkingLevels {
				levelChip(c, t, key+level, level, &m.levels[i])
			}
		})
	})
}

// levelChip 画一个思考档位的标签按钮：选中时是淡的强调色底，没选中时是主区底色，点一下切换。
func levelChip(c *ui.Context, t *ui.Theme, key, label string, on *bool) {
	chip := ui.ButtonBase(c.Key(key)).Height(26).Padding(0, 12).Radius(controlRound).
		AlignItems(ui.Center).Label(label)
	color := t.TextMuted
	switch {
	case *on:
		chip.Background(accentFg(t).Alpha(0.15))
		color = accentFg(t)
	case chip.Hovered():
		chip.Background(buttonFill(t).Over(t.Background))
	default:
		chip.Background(t.Background)
	}
	chip.OnClick(func() { *on = !*on })
	chip.Children(func() { ui.Text(c, label).FontSize(textSize).TextColor(color) })
}

// deleteButton 画服务对话框底部左边的删除按钮；新建时没有。第一次点只是要求确认。
func (a *App) deleteButton(c *ui.Context, t *ui.Theme) {
	f := a.settings.models.form
	if f.orig == "" {
		return
	}
	label := "删除服务"
	if f.confirmDelete {
		label = "确认删除，同时删除 API key"
	}
	quietButton(c, t, label, a.deleteService).Height(28).Padding(0, 12).Shrink(0).
		Children(func() { ui.Text(c, label).FontSize(textSize).TextColor(t.Danger) })
}
