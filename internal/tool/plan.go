package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	maxPlanSteps     = 50
	maxPlanStepRunes = 500
)

// UpdatePlan manages the complete task plan for multi-step work.
type UpdatePlan struct{}

type updatePlanParams struct {
	Plan *[]planItem `json:"plan"`
}

type planItem struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

// Definition returns the tool definition for update_plan.
func (UpdatePlan) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name: "update_plan",
		Description: "Update the complete task plan for multi-step work. " +
			"Each call replaces the entire plan. " +
			"Use pending, in_progress, or completed for status. " +
			"Keep at most one step in_progress at a time. " +
			"Skip for simple or single-step tasks.\n\n" +
			"When to use:\n" +
			"- Multi-step tasks that span several tool calls\n" +
			"- Planning a sequence of edits before making them\n" +
			"- After receiving new multi-step instructions, capture the requirements as plan steps\n" +
			"- Before starting a tracked step, mark exactly one step as in_progress\n" +
			"- Immediately after finishing a tracked step, mark it completed\n\n" +
			"When NOT to use:\n" +
			"- Single-shot answers that complete in one or two tool calls\n" +
			"- Trivial requests where tracking adds no clarity\n\n" +
			"Avoid churn:\n" +
			"- Do not re-call this tool when nothing meaningful has changed since the last call\n" +
			"- Update the plan only after real progress, not after every tool call\n" +
			"- Keep steps short and actionable (e.g., \"Read main.go\", \"Fix nil pointer in handler\")",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"plan": map[string]any{
					"type":     "array",
					"maxItems": maxPlanSteps,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"step": map[string]any{
								"type":        "string",
								"description": "Task step text",
								"maxLength":   maxPlanStepRunes,
							},
							"status": map[string]any{
								"type":        "string",
								"enum":        []string{"pending", "in_progress", "completed"},
								"description": "Current step status",
							},
						},
						"required": []string{"step", "status"},
					},
					"description": "The updated task plan",
				},
			},
			"required": []string{"plan"},
		},
	}
}

// ParsePlan validates plan arguments and returns normalized entries.
func ParsePlan(arguments string) ([]model.PlanEntry, error) {
	var params updatePlanParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return nil, fmt.Errorf("parse arguments: %w", err)
	}
	if params.Plan == nil {
		return nil, fmt.Errorf("plan is required")
	}
	if len(*params.Plan) > maxPlanSteps {
		return nil, fmt.Errorf("plan must contain at most %d steps", maxPlanSteps)
	}

	entries := make([]model.PlanEntry, 0, len(*params.Plan))
	inProgress := 0
	for _, item := range *params.Plan {
		step := strings.TrimSpace(item.Step)
		if step == "" {
			return nil, fmt.Errorf("plan step is required")
		}
		if utf8.RuneCountInString(step) > maxPlanStepRunes {
			return nil, fmt.Errorf("plan step must contain at most %d characters", maxPlanStepRunes)
		}
		if strings.IndexFunc(step, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("plan step must not contain control characters")
		}
		status := model.PlanStatus(item.Status)
		switch status {
		case model.PlanStatusPending, model.PlanStatusInProgress, model.PlanStatusCompleted:
		default:
			return nil, fmt.Errorf("invalid status %q for plan step %q", item.Status, step)
		}
		if status == model.PlanStatusInProgress {
			inProgress++
			if inProgress > 1 {
				return nil, fmt.Errorf("plan must contain at most one in_progress step")
			}
		}
		entries = append(entries, model.PlanEntry{Step: step, Status: status})
	}
	return entries, nil
}

// Run validates and replaces the current task plan.
func (UpdatePlan) Run(_ context.Context, arguments string) (string, error) {
	if _, err := ParsePlan(arguments); err != nil {
		return "", err
	}
	return "Plan updated", nil
}

// Metadata returns structured presentation data for the task plan.
func (UpdatePlan) Metadata(arguments string, _ string) model.ToolMetadata {
	entries, err := ParsePlan(arguments)
	if err != nil {
		return model.ToolMetadata{}
	}
	return model.ToolMetadata{Plan: entries}
}
