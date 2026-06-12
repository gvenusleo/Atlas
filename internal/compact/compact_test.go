package compact

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestSelectPlanCutsBeforeUserTurn(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleUser, Content: "first"},
		{Role: model.RoleAssistant, Content: "first response"},
		{Role: model.RoleUser, Content: "second"},
		{Role: model.RoleAssistant, Content: "second response"},
		{Role: model.RoleUser, Content: "third"},
		{Role: model.RoleAssistant, Content: "third response"},
	}

	plan, ok := SelectPlan(messages, 0, 1)
	if !ok {
		t.Fatal("SelectPlan() ok = false")
	}
	if plan.CompactCount != 4 {
		t.Fatalf("CompactCount = %d, want 4", plan.CompactCount)
	}
	if messages[plan.CompactCount].Content != "third" {
		t.Fatalf("cut message = %#v", messages[plan.CompactCount])
	}
}

func TestSelectPlanDoesNotCutToolExchange(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleUser, Content: "first"},
		{
			Role:    model.RoleAssistant,
			Content: "checking",
			ToolCalls: []model.ToolCall{{
				ID:   "call-1",
				Name: "read_file",
			}},
		},
		{Role: model.RoleTool, Content: "result", ToolCallID: "call-1"},
		{Role: model.RoleAssistant, Content: "done"},
		{Role: model.RoleUser, Content: "second"},
		{Role: model.RoleAssistant, Content: "second response"},
	}

	plan, ok := SelectPlan(messages, 0, 1)
	if !ok {
		t.Fatal("SelectPlan() ok = false")
	}
	if plan.CompactCount != 4 {
		t.Fatalf("CompactCount = %d, want 4", plan.CompactCount)
	}
}

func TestSelectPlanKeepsOversizedRecentTurnWhole(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleUser, Content: "first"},
		{Role: model.RoleAssistant, Content: "first response"},
		{Role: model.RoleUser, Content: strings.Repeat("x", 1000)},
		{Role: model.RoleAssistant, Content: strings.Repeat("y", 1000)},
	}

	plan, ok := SelectPlan(messages, 0, 10)
	if !ok {
		t.Fatal("SelectPlan() ok = false")
	}
	if plan.CompactCount != 2 {
		t.Fatalf("CompactCount = %d, want 2", plan.CompactCount)
	}
	if plan.KeepCount != 2 {
		t.Fatalf("KeepCount = %d, want 2", plan.KeepCount)
	}
}

func TestSelectPlanReturnsFalseWithoutSafeCut(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleUser, Content: "only"},
		{Role: model.RoleAssistant, Content: "response"},
	}

	if _, ok := SelectPlan(messages, 0, 1); ok {
		t.Fatal("SelectPlan() ok = true")
	}
}

func TestSelectManualPlanKeepsNewestTurn(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleUser, Content: "first"},
		{Role: model.RoleAssistant, Content: "first response"},
		{Role: model.RoleUser, Content: "second"},
		{Role: model.RoleAssistant, Content: "second response"},
		{Role: model.RoleUser, Content: "third"},
		{Role: model.RoleAssistant, Content: "third response"},
	}

	plan, ok := SelectManualPlan(messages, 0)
	if !ok {
		t.Fatal("SelectManualPlan() ok = false")
	}
	if plan.CompactCount != 4 || plan.KeepCount != 2 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildActiveMessagesAddsSummaryAndTail(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleUser, Content: "old"},
		{Role: model.RoleAssistant, Content: "old response"},
		{Role: model.RoleUser, Content: "new"},
	}

	active := BuildActiveMessages("summary", 2, messages)
	if len(active) != 2 {
		t.Fatalf("active = %#v", active)
	}
	if active[0].Role != model.RoleUser || !strings.Contains(active[0].Content, "summary") {
		t.Fatalf("summary message = %#v", active[0])
	}
	if active[1].Content != "new" {
		t.Fatalf("tail = %#v", active[1])
	}
}

func TestBuildSummaryMessagesIncludesPreviousSummaryAndInstruction(t *testing.T) {
	got := BuildSummaryMessages("old summary", []model.Message{
		{Role: model.RoleUser, Content: "please edit"},
		{Role: model.RoleTool, Content: strings.Repeat("tool", 1000)},
	}, "focus files")

	if len(got) != 1 || got[0].Role != model.RoleUser {
		t.Fatalf("messages = %#v", got)
	}
	content := got[0].Content
	for _, want := range []string{"old summary", "focus files", "[User]: please edit", "[Tool result]:"} {
		if !strings.Contains(content, want) {
			t.Fatalf("summary prompt missing %q: %s", want, content)
		}
	}
	if !strings.Contains(content, "[truncated]") {
		t.Fatalf("summary prompt did not truncate tool result: %s", content)
	}
}

func TestBuildSummaryMessagesTruncatesToolResultWithoutBreakingUTF8(t *testing.T) {
	got := BuildSummaryMessages("", []model.Message{
		{Role: model.RoleTool, Content: strings.Repeat("界", 3000)},
	}, "")

	if !utf8.ValidString(got[0].Content) {
		t.Fatalf("summary prompt is not valid UTF-8")
	}
	if !strings.Contains(got[0].Content, "[truncated]") {
		t.Fatalf("summary prompt did not truncate tool result: %s", got[0].Content)
	}
}
