package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// TodoWrite is the todo list management tool. The model passes the complete list each call, replacing all entries.
type TodoWrite struct{}

// todoWriteParams is the parameters for the todo_write tool.
type todoWriteParams struct {
	Todos []todoItem `json:"todos"`
}

// todoItem is a single todo entry.
type todoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// Definition returns the tool definition for todo_write.
func (TodoWrite) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name: "todo_write",
		Description: "Manage a structured task list for multi-step work. " +
			"Each call replaces the entire list. " +
			"Use pending, in_progress, or completed for status. " +
			"Keep at most one item in_progress at a time. " +
			"Skip for simple or single-step tasks.\n\n" +
			"When to use:\n" +
			"- Multi-step tasks that span several tool calls\n" +
			"- Planning a sequence of edits before making them\n" +
			"- After receiving new multi-step instructions, capture the requirements as todos\n" +
			"- Before starting a tracked task, mark exactly one item as in_progress\n" +
			"- Immediately after finishing a tracked task, mark it completed\n\n" +
			"When NOT to use:\n" +
			"- Single-shot answers that complete in one or two tool calls\n" +
			"- Trivial requests where tracking adds no clarity\n\n" +
			"Avoid churn:\n" +
			"- Do not re-call this tool when nothing meaningful has changed since the last call\n" +
			"- Update the list only after real progress, not after every tool call\n" +
			"- Keep titles short and actionable (e.g., \"Read main.go\", \"Fix nil pointer in handler\")",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"todos": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"content": map[string]any{
								"type":        "string",
								"description": "Brief description of the task",
							},
							"status": map[string]any{
								"type":        "string",
								"enum":        []string{"pending", "in_progress", "completed"},
								"description": "Current status of the task",
							},
						},
						"required": []string{"content", "status"},
					},
					"description": "The updated todo list",
				},
			},
			"required": []string{"todos"},
		},
	}
}

// Run parses parameters and returns a todo list summary and structured metadata.
func (TodoWrite) Run(_ context.Context, arguments string) (string, error) {
	var params todoWriteParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	entries := make([]model.TodoEntry, 0, len(params.Todos))
	for _, item := range params.Todos {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			return "", fmt.Errorf("todo content is required")
		}
		status := model.TodoStatus(item.Status)
		switch status {
		case model.TodoStatusPending, model.TodoStatusInProgress, model.TodoStatusCompleted:
		default:
			return "", fmt.Errorf("invalid status %q for todo %q", item.Status, content)
		}
		entries = append(entries, model.TodoEntry{
			Content: content,
			Status:  status,
		})
	}

	var b strings.Builder
	pending, inProgress, completed := 0, 0, 0
	for _, entry := range entries {
		switch entry.Status {
		case model.TodoStatusPending:
			pending++
		case model.TodoStatusInProgress:
			inProgress++
		case model.TodoStatusCompleted:
			completed++
		}
	}
	fmt.Fprintf(&b, "Todo list updated (%d total).\n", len(entries))
	fmt.Fprintf(&b, "Status: %d pending, %d in progress, %d completed\n", pending, inProgress, completed)
	b.WriteString("Continue with the remaining tasks.")
	return b.String(), nil
}

// Metadata returns structured presentation data for todo list presentation.
func (TodoWrite) Metadata(arguments string, _ string) model.ToolMetadata {
	var params todoWriteParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return model.ToolMetadata{}
	}
	entries := make([]model.TodoEntry, 0, len(params.Todos))
	for _, item := range params.Todos {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		status := model.TodoStatus(item.Status)
		switch status {
		case model.TodoStatusPending, model.TodoStatusInProgress, model.TodoStatusCompleted:
		default:
			continue
		}
		entries = append(entries, model.TodoEntry{
			Content: content,
			Status:  status,
		})
	}
	return model.ToolMetadata{Todos: entries}
}
