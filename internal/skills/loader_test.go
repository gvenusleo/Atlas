package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDiscoversSkillMetadata(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "think", "think", "Plan work before coding")

	catalog := Load([]string{root})
	if len(catalog.Errors) != 0 {
		t.Fatalf("unexpected load errors: %#v", catalog.Errors)
	}
	if len(catalog.Skills) != 1 {
		t.Fatalf("expected one skill, got %#v", catalog.Skills)
	}
	skill := catalog.Skills[0]
	if skill.Name != "think" || skill.Description != "Plan work before coding" {
		t.Fatalf("unexpected skill metadata: %#v", skill)
	}
	if !filepath.IsAbs(skill.Path) {
		t.Fatalf("skill path should be absolute: %q", skill.Path)
	}
}

func TestSelectMentionedUsesUniqueNamesAndPaths(t *testing.T) {
	root := t.TempDir()
	first := writeSkill(t, root, "one", "demo", "First")
	second := writeSkill(t, root, "two", "demo", "Second")
	catalog := Load([]string{root})

	if got := SelectMentioned(catalog.Skills, "$demo"); len(got) != 0 {
		t.Fatalf("duplicate skill names should not be selected by plain name: %#v", got)
	}
	got := SelectMentioned(catalog.Skills, "[$demo](skill://"+first+")")
	if len(got) != 1 || pathKey(got[0].Path) != pathKey(first) {
		t.Fatalf("path mention should select exact skill: %#v, first=%q second=%q", got, first, second)
	}
}

func TestBuildPromptContextInjectsMentionedSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "think", "think", "Plan work before coding")
	catalog := Load([]string{root})

	ctx := BuildPromptContext(catalog, "$think make a plan")
	if !strings.Contains(ctx.Available, "think") {
		t.Fatalf("available skills should include think: %q", ctx.Available)
	}
	if len(ctx.Injected) != 1 {
		t.Fatalf("expected one injected skill: %#v", ctx.Injected)
	}
	if !strings.Contains(ctx.Injected[0].Contents, "Plan carefully") {
		t.Fatalf("injected skill should include full body: %q", ctx.Injected[0].Contents)
	}
}

func writeSkill(t *testing.T, root, dir, name, description string) string {
	t.Helper()
	path := filepath.Join(root, dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\nPlan carefully.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
