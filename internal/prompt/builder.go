package prompt

import (
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

// Build creates the system prompt and recent model messages.
func (b Builder) Build(session storage.Session, messages []storage.Message) (string, []model.Message) {
	system := b.SystemPrompt
	if strings.TrimSpace(system) == "" {
		system = defaultSystemPrompt
	}
	system = fmt.Sprintf("%s\n\n<env>\nWorking directory: %s\nModel: %s\n</env>",
		system,
		session.Workdir,
		session.Model,
	)

	maxMessages := b.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 80
	}
	if len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}

	out := make([]model.Message, 0, len(messages))
	for _, message := range messages {
		out = append(out, model.Message{
			Role:       model.Role(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		})
	}
	return system, out
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
