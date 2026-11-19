package gui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
)

const queueInset = 16

func (a *App) promptQueuePanel(c *ui.Context, t *ui.Theme, ch *chat) {
	items := ch.queue.Items
	if ch.needsReopen {
		items = nil
	}
	if len(items) == 0 && len(ch.failedDrafts) == 0 {
		return
	}
	ui.Column(c.Key("prompt-queue")).MarginX(queueInset).Padding(composerPad).Gap(4).
		Radius(cardRound, cardRound, 0, 0).Background(t.Background).
		BorderWidth(1, 1, 0, 1).BorderColor(fieldBorder(t)).Children(func() {
		if ch.queue.Paused && len(items) != 0 {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				reason := ch.queue.Error
				if reason == "" {
					reason = "队列已暂停"
				}
				ui.Text(c, reason).Grow(1).MinWidth(0).FontSize(smallSize).
					TextColor(t.TextMuted).SingleLine().Tooltip(reason)
				resumeDisabled := ch.run.Running || ch.resuming
				queueButton(c.Key("resume"), t, "继续发送", resumeDisabled,
					func() { a.resumePromptQueue(ch) })
			})
		}
		ui.Scroll(c.Key("queue-scroll")).MaxHeight(224).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				var group protocol.PromptDelivery
				for _, item := range items {
					if item.Delivery != group {
						group = item.Delivery
						queueGroup(c.Key(group), t, group)
					}
					a.queuedPromptRow(c.Key(item.ID), t, ch, item)
				}
				a.unsentPromptRows(c.Key("unsent"), t, ch)
			})
		})
	})
}

func queueGroup(c *ui.Context, t *ui.Theme, group protocol.PromptDelivery) {
	label := "下一轮起依次发送"
	if group == protocol.PromptInTurn {
		label = "插入本轮"
	}
	ui.Text(c, label).Padding(3, 6).FontSize(smallSize).TextColor(t.TextMuted)
}

func (a *App) queuedPromptRow(c *ui.Context, t *ui.Theme, ch *chat, item protocol.QueuedPrompt) {
	editing := ch.queueEditing != nil && ch.queueEditing.item.ID == item.ID
	fill := rowFill(t)
	if editing {
		fill = buttonFill(t)
	}
	ui.Row(c).MinHeight(barButton+4).AlignItems(ui.Center).Padding(2, 6).Gap(6).
		Radius(controlRound).Background(fill).Children(func() {
		previewPrompt(c, t, item.Prompt)
		switch {
		case item.Sending:
			ui.Text(c, "发送中").TextColor(t.TextMuted).FontSize(smallSize)
			return
		case editing:
			ui.Text(c, "编辑中").TextColor(t.TextMuted).FontSize(smallSize)
			return
		}
		iconButton(c.Key("edit"), t, pencilIcon, "编辑", barButton, func() {
			a.editQueuedPrompt(ch, item.ID)
		}).Disabled(!ch.canEditQueue())
		iconButton(c.Key("delete"), t, trashIcon, "删除", barButton, func() {
			a.deleteQueuedPrompt(ch, item)
		})
	})
}

func (a *App) queueEditActions(c *ui.Context, t *ui.Theme, ch *chat) {
	edit := ch.queueEditing
	if edit == nil {
		return
	}
	ui.Row(c.Key("queue-edit-actions")).AlignItems(ui.Center).Gap(6).
		Padding(4, composerInset, 0).Children(func() {
		problem := ch.queueEditProblem()
		status := "编辑待发消息"
		if problem != "" {
			status = problem
		}
		ui.Text(c, status).Grow(1).MinWidth(0).FontSize(smallSize).
			TextColor(t.TextMuted).SingleLine().Tooltip(status)
		queueButton(c.Key("cancel"), t, "取消编辑", edit.saving || ch.preparing != 0, ch.closeQueueEdit)
		canSave := ch.preparing == 0 && ch.hasDraft()
		if problem != "" {
			queueButton(c.Key("keep"), t, "保留为草稿", !canSave, ch.keepQueueEdit)
		} else {
			label := "保存"
			if edit.saving {
				label = "保存中"
			}
			queueButton(c.Key("save"), t, label, edit.saving || !canSave,
				func() { a.saveQueueEdit(ch, ch.input) })
		}
	})
}

func (a *App) unsentPromptRows(c *ui.Context, t *ui.Theme, ch *chat) {
	if len(ch.failedDrafts) == 0 {
		return
	}
	ui.Text(c, "未发送的草稿").Padding(3, 6).FontSize(smallSize).TextColor(t.TextMuted)
	for _, failed := range ch.failedDrafts {
		ui.Row(c.Key(failed.id)).AlignItems(ui.Center).Padding(2, 6).Gap(6).
			Radius(controlRound).Background(rowFill(t)).Tooltip(failed.reason).Children(func() {
			previewPrompt(c, t, draftPrompt(failed.draft.input, failed.draft.attachments))
			label := "恢复到输入框"
			disabled := ch.queueEditing != nil || ch.hasDraft() || ch.preparing != 0
			if disabled {
				label = "输入框已有草稿"
			}
			queueButton(c.Key("restore"), t, label, disabled, func() {
				a.restoreFailedDraft(ch, failed)
			})
			iconButton(c.Key("discard"), t, trashIcon, "移除", barButton, func() {
				ch.removeFailedDraft(failed.id)
			})
		})
	}
}

func previewPrompt(c *ui.Context, t *ui.Theme, prompt []protocol.ContentBlock) {
	var text string
	var images int
	for _, block := range prompt {
		switch block.Type {
		case protocol.ContentText:
			if text == "" {
				text, _, _ = strings.Cut(strings.TrimSpace(block.Text), "\n")
			}
		case protocol.ContentImage:
			images++
		}
	}
	if images > 0 {
		if text != "" {
			text += " · "
		}
		text += fmt.Sprintf("%d 张图片", images)
	}
	ui.Text(c, text).Grow(1).MinWidth(0).FontSize(textSize).SingleLine().Tooltip(text)
}

func queueButton(c *ui.Context, t *ui.Theme, label string, disabled bool, action func()) {
	btn := ui.ButtonBase(c).Padding(3, 7).Radius(controlRound).Label(label).
		Disabled(disabled).OnClick(action)
	if btn.Hovered() {
		btn.Background(buttonFill(t))
	}
	btn.Children(func() { ui.Text(c, label).FontSize(smallSize) })
}
