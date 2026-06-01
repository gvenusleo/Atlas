package prompt

import (
	"strings"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/storage"
)

func TestBuildAddsSkillsContextWithoutPersistedHistory(t *testing.T) {
	session := storage.Session{Workdir: "/tmp/work", Model: "test-model"}
	history := []storage.Message{{Role: string(model.RoleUser), Content: "$think plan"}}
	extra := ExtraContext{
		AvailableSkills: "<skills_instructions>\n- think\n</skills_instructions>",
		SkillBlocks:     []string{"<skill>\nthink body\n</skill>"},
	}

	system, messages := Builder{}.Build(session, history, extra)
	if !strings.Contains(system, "<skills_instructions>") {
		t.Fatalf("system prompt should include skills list: %q", system)
	}
	if len(messages) != 2 {
		t.Fatalf("expected history plus transient skill message, got %#v", messages)
	}
	if messages[0].Role != model.RoleUser || !strings.Contains(messages[0].Content, "<skill>") {
		t.Fatalf("first message should be transient skill block: %#v", messages[0])
	}
	if messages[1].Content != "$think plan" {
		t.Fatalf("last message should be persisted user request: %#v", messages)
	}
}
