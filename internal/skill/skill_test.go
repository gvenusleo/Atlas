package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadScansUserAndProjectSkills(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)

	writeSkill(t, filepath.Join(home, ".agents", "skills", "write", "SKILL.md"), `---
name: write
description: "user description"
---

# User Write
`)
	writeSkill(t, filepath.Join(cwd, ".atlas", "skills", "project", "SKILL.md"), `---
name: project
description: project description
---

# Project
`)

	catalog, err := Load(cwd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	summaries := catalog.Summaries()
	if len(summaries) != 2 {
		t.Fatalf("summaries = %#v", summaries)
	}
	if summaries[0].Name != "project" || summaries[1].Name != "write" {
		t.Fatalf("summaries = %#v", summaries)
	}
}

func TestLoadProjectSkillOverridesUserSkill(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)

	writeSkill(t, filepath.Join(home, ".agents", "skills", "shared", "SKILL.md"), `---
name: shared
description: user description
---

# User
`)
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "shared", "SKILL.md"), `---
name: shared
description: project description
---

# Project
`)

	catalog, err := Load(cwd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	skill, ok := catalog.Lookup("shared")
	if !ok {
		t.Fatal("Lookup() did not find shared")
	}
	if skill.Description != "project description" {
		t.Fatalf("description = %q", skill.Description)
	}
	if !strings.Contains(skill.Content, "# Project") {
		t.Fatalf("content = %q", skill.Content)
	}
}

func TestParseFrontmatter(t *testing.T) {
	skill, err := parse("/tmp/SKILL.md", `---
name: demo
description: "demo: description"
disable-model-invocation: true
---

# Demo
`)
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if skill.Name != "demo" || skill.Description != "demo: description" || !skill.DisableModelInvocation {
		t.Fatalf("skill = %#v", skill)
	}
}

func TestCatalogHidesDisabledSkills(t *testing.T) {
	catalog, err := NewCatalog([]Skill{
		{Name: "hidden", Description: "hidden", DisableModelInvocation: true},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	if got := catalog.Summaries(); len(got) != 0 {
		t.Fatalf("Summaries() = %#v", got)
	}
	if _, ok := catalog.Lookup("hidden"); ok {
		t.Fatal("Lookup() found disabled skill")
	}
}

func writeSkill(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func setTestHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	volume := filepath.VolumeName(home)
	if volume != "" {
		t.Setenv("HOMEDRIVE", volume)
		t.Setenv("HOMEPATH", strings.TrimPrefix(home, volume))
	}
}
