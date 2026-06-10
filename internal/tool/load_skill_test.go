package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/liuyuxin/atlas/internal/skill"
)

func TestLoadSkillDefinitionListsSkills(t *testing.T) {
	catalog, err := skill.NewCatalog([]skill.Skill{{
		Name:        "write",
		Description: "polish prose",
	}})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	def := (LoadSkill{Skills: catalog}).Definition()
	if def.Name != "load_skill" {
		t.Fatalf("name = %q", def.Name)
	}
	if !strings.Contains(def.Description, "write: polish prose") {
		t.Fatalf("description = %q", def.Description)
	}
}

func TestLoadSkillRunReturnsFullContent(t *testing.T) {
	catalog, err := skill.NewCatalog([]skill.Skill{{
		Name:        "write",
		Description: "polish prose",
		Dir:         "/tmp/skills/write",
		Path:        "/tmp/skills/write/SKILL.md",
		Content:     "---\nname: write\n---\n\n# Write\nFull body",
	}})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	got, err := (LoadSkill{Skills: catalog}).Run(context.Background(), `{"name":"write"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, check := range []string{
		"Skill: write",
		"Directory: /tmp/skills/write",
		"Path: /tmp/skills/write/SKILL.md",
		"# Write",
		"Full body",
	} {
		if !strings.Contains(got, check) {
			t.Fatalf("Run() missing %q in %q", check, got)
		}
	}
}

func TestLoadSkillRunRejectsUnknownSkill(t *testing.T) {
	catalog, err := skill.NewCatalog(nil)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	_, err = (LoadSkill{Skills: catalog}).Run(context.Background(), `{"name":"missing"}`)
	if err == nil {
		t.Fatal("Run() error = nil")
	}
}
