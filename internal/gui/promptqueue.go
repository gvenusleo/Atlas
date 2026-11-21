package gui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gvenusleo/atlas/internal/protocol"
)

type promptSubmission struct {
	request protocol.SendPromptRequest
	draft   promptDraft
}

type failedPrompt struct {
	id     int
	reason string
	draft  promptDraft
}

type promptQueueEdit struct {
	item     protocol.QueuedPrompt
	original promptDraft
	saving   bool
	err      string
}

func (ch *chat) queuedPrompt(id uint64) (protocol.QueuedPrompt, bool) {
	for _, item := range ch.queue.Items {
		if item.ID == id {
			return item, true
		}
	}
	return protocol.QueuedPrompt{}, false
}

func (a *App) editQueuedPrompt(ch *chat, id uint64) {
	if !ch.canEditQueue() {
		return
	}
	item, exists := ch.queuedPrompt(id)
	if !exists || item.Sending || ch.needsReopen {
		return
	}
	ch.queueEditing = &promptQueueEdit{item: item, original: ch.promptDraft}
	draft := ch.promptDraftFrom(item.Prompt)
	ch.setInputDraft(draft)
	a.loadDraftThumbnails(draft.attachments)
}

func (ch *chat) canEditQueue() bool {
	return ch.queueEditing == nil && ch.preparing == 0 && !ch.loading
}

func (ch *chat) setInputDraft(draft promptDraft) {
	ch.promptDraft = draft
	ch.menu.cur = completion{}
	ch.menu.caretAt, ch.menu.caretDue = utf8.RuneCountInString(draft.input), true
	ch.focusInput = true
}

func (ch *chat) closeQueueEdit() {
	ch.setInputDraft(ch.queueEditing.original)
	ch.queueEditing = nil
}

func (ch *chat) queueEditProblem() string {
	edit := ch.queueEditing
	if edit.saving {
		return ""
	}
	if edit.err != "" {
		return edit.err
	}
	item, exists := ch.queuedPrompt(edit.item.ID)
	switch {
	case ch.needsReopen:
		return "会话已失效，修改稿可保留为草稿"
	case !exists:
		return "原消息已离开队列"
	case item.Sending:
		return "原消息正在发送"
	case item.Version != edit.item.Version:
		return "原消息已修改"
	default:
		return ""
	}
}

func (a *App) saveQueueEdit(ch *chat, text string) {
	edit := ch.queueEditing
	if edit.saving || ch.preparing != 0 || ch.queueEditProblem() != "" {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" && len(ch.attachments) == 0 {
		return
	}
	ch.err = ""
	edit.saving = true
	req := protocol.UpdateQueuedPromptRequest{SessionID: ch.id,
		ID: edit.item.ID, Version: edit.item.Version, Prompt: draftPrompt(text, ch.attachments)}
	go func() {
		err := a.service.UpdateQueuedPrompt(context.Background(), req)
		a.win.Update(func() {
			if a.chats[req.SessionID] == ch {
				ch.finishQueueEdit(edit, err)
			}
		})
	}()
}

func (ch *chat) finishQueueEdit(edit *promptQueueEdit, err error) {
	if ch.queueEditing != edit {
		return
	}
	if err != nil {
		edit.saving = false
		if errors.Is(err, protocol.ErrQueuedPromptChanged) || errors.Is(err, protocol.ErrNotFound) {
			edit.err = "原消息已不可修改"
		} else {
			ch.err = err.Error()
			ch.focusInput = true
		}
		return
	}
	ch.closeQueueEdit()
}

func (ch *chat) keepQueueEdit() {
	ch.keepFailedDraft(ch.promptDraft, ch.queueEditProblem())
	ch.closeQueueEdit()
}

func (a *App) deleteQueuedPrompt(ch *chat, item protocol.QueuedPrompt) {
	req := protocol.DeleteQueuedPromptRequest{
		SessionID: ch.id, ID: item.ID, Version: item.Version,
	}
	go func() {
		err := a.service.DeleteQueuedPrompt(context.Background(), req)
		a.win.Update(func() {
			if err != nil && a.chats[req.SessionID] == ch {
				if a.current == ch {
					a.toast = err.Error()
				} else {
					ch.err = err.Error()
				}
			}
		})
	}()
}

func (a *App) resumePromptQueue(ch *chat) {
	if a.worktreeBlocked(ch.cwd) {
		a.toast = "工作树正在维护，请稍后再试"
		return
	}
	if ch.cwdGone {
		a.askCwdGone(ch)
		return
	}
	ch.err = ""
	ch.resuming = true
	req := protocol.ResumePromptQueueRequest{SessionID: ch.id}
	cwd := ch.cwd
	record := a.worktreeRecoveries[ch.id]
	go func() {
		err := requireWorktreeRecovery(record, cwd)
		if err == nil {
			err = a.service.ResumePromptQueue(context.Background(), req)
		}
		a.win.Update(func() {
			if a.chats[req.SessionID] == ch {
				ch.resuming = false
				a.finish(ch, err)
			}
		})
	}()
}

// keepFailedDraft 保留原始输入。用户可以继续打字，再单独恢复每条退回的草稿。
func (ch *chat) keepFailedDraft(draft promptDraft, reason string) {
	ch.next++
	ch.failedDrafts = append(ch.failedDrafts, failedPrompt{
		id: ch.next, draft: draft, reason: reason,
	})
}

func (a *App) restoreFailedDraft(ch *chat, failed failedPrompt) {
	if ch.queueEditing != nil || ch.hasDraft() || ch.preparing != 0 {
		return
	}
	ch.setInputDraft(failed.draft)
	a.loadDraftThumbnails(failed.draft.attachments)
	ch.removeFailedDraft(failed.id)
}

func (ch *chat) removeFailedDraft(id int) {
	ch.failedDrafts = slices.DeleteFunc(ch.failedDrafts, func(f failedPrompt) bool {
		return f.id == id
	})
}

func (ch *chat) promptDraftFrom(prompt []protocol.ContentBlock) promptDraft {
	var draft promptDraft
	var texts []string
	for _, block := range prompt {
		switch block.Type {
		case protocol.ContentText:
			texts = append(texts, block.Text)
		case protocol.ContentImage:
			ch.next++
			draft.attachments = append(draft.attachments, &attachment{id: ch.next,
				name: fmt.Sprintf("图片 %d", len(draft.attachments)+1), image: block})
		}
	}
	draft.input = strings.Join(texts, "\n")
	return draft
}

func (a *App) loadDraftThumbnails(attachments []*attachment) {
	for _, f := range attachments {
		if f.bitmap != nil {
			continue
		}
		img := f.image
		go func() {
			bitmap, _ := decodePicture(img, userPictureEdge)
			a.win.Update(func() { f.bitmap = bitmap })
		}()
	}
}

// recoverQueue 在会话失效时把未发送内容留在本地草稿里，供重新打开后恢复。
func (ch *chat) recoverQueue() {
	for _, item := range ch.queue.Items {
		ch.keepFailedDraft(ch.promptDraftFrom(item.Prompt), "会话已失效，待发消息已保留")
	}
}
