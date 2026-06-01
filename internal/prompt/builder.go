package prompt

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/storage"
)

const defaultSystemPrompt = `You are Atlas, a coding agent running on the user's machine.

Work directly in the local repository. Use tools to inspect files before editing.
Prefer small, precise changes. After edits, run relevant checks when practical.
You have full local access; do not claim a permission system exists.`

// Builder converts durable session history into provider-neutral model input.
type Builder struct {
	SystemPrompt string
	MaxMessages  int
}

// ExtraContext contains transient prompt fragments that should not be persisted.
type ExtraContext struct {
	AvailableSkills string
	SkillBlocks     []string
}

// Build creates the system prompt and recent model messages.
func (b Builder) Build(session storage.Session, messages []storage.Message, extra ExtraContext) (string, []model.Message) {
	system := b.SystemPrompt
	if strings.TrimSpace(system) == "" {
		system = defaultSystemPrompt
	}
	system = fmt.Sprintf("%s\n\n<env>\nWorking directory: %s\nModel: %s\n</env>",
		system,
		session.Workdir,
		session.Model,
	)
	if strings.TrimSpace(extra.AvailableSkills) != "" {
		system += "\n\n" + strings.TrimSpace(extra.AvailableSkills)
	}
	return b.buildMessages(system, messages, extra)
}

// buildMessages appends recent history and transient context to model input.
func (b Builder) buildMessages(system string, messages []storage.Message, extra ExtraContext) (string, []model.Message) {
	maxMessages := b.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 80
	}
	if len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}

	blocks := cleanSkillBlocks(extra.SkillBlocks)
	insertAt := len(messages)
	if len(blocks) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			if model.Role(messages[i].Role) == model.RoleUser {
				insertAt = i
				break
			}
		}
	}

	out := make([]model.Message, 0, len(messages)+len(blocks))
	for i, message := range messages {
		if i == insertAt {
			out = append(out, blocks...)
		}
		item := model.Message{
			Role:       model.Role(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		}
		if message.ToolCalls != "" {
			_ = json.Unmarshal([]byte(message.ToolCalls), &item.ToolCalls)
		}
		out = append(out, item)
	}
	if insertAt == len(messages) {
		out = append(out, blocks...)
	}
	return system, out
}

// cleanSkillBlocks converts non-empty skill fragments into transient user messages.
func cleanSkillBlocks(blocks []string) []model.Message {
	out := make([]model.Message, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		out = append(out, model.Message{
			Role:    model.RoleUser,
			Content: block,
		})
	}
	return out
}

// DefaultDBPath returns the local database path used by the CLI and TUI.
func DefaultDBPath() string {
	if value := strings.TrimSpace(os.Getenv("ATLAS_DB")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".atlas.db"
	}
	return home + "/.atlas/atlas.db"
}
