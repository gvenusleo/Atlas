package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	skillFileName     = "SKILL.md"
	maxScanDepth      = 6
	maxDirsPerRoot    = 2000
	maxNameLen        = 64
	maxDescriptionLen = 1024
)

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Load discovers SKILL.md files below roots and returns a stable catalog.
func Load(roots []string) Catalog {
	catalog := Catalog{}
	seen := make(map[string]bool)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			catalog.Errors = append(catalog.Errors, LoadError{Path: root, Message: err.Error()})
			continue
		}
		scanRoot(abs, seen, &catalog)
	}
	sort.Slice(catalog.Skills, func(i, j int) bool {
		if catalog.Skills[i].Name != catalog.Skills[j].Name {
			return catalog.Skills[i].Name < catalog.Skills[j].Name
		}
		return catalog.Skills[i].Path < catalog.Skills[j].Path
	})
	return catalog
}

func scanRoot(root string, seen map[string]bool, catalog *Catalog) {
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		catalog.Errors = append(catalog.Errors, LoadError{Path: root, Message: err.Error()})
		return
	}
	if !info.IsDir() {
		return
	}

	visited := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			catalog.Errors = append(catalog.Errors, LoadError{Path: path, Message: err.Error()})
			return nil
		}
		if path != root && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			visited++
			if visited > maxDirsPerRoot {
				return filepath.SkipAll
			}
			if depth(root, path) > maxScanDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != skillFileName {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			resolved = path
		}
		abs, err := filepath.Abs(resolved)
		if err != nil {
			catalog.Errors = append(catalog.Errors, LoadError{Path: path, Message: err.Error()})
			return nil
		}
		if seen[abs] {
			return nil
		}
		seen[abs] = true
		skill, err := ParseFile(abs)
		if err != nil {
			catalog.Errors = append(catalog.Errors, LoadError{Path: abs, Message: err.Error()})
			return nil
		}
		catalog.Skills = append(catalog.Skills, skill)
		return nil
	})
}

// depth returns path depth relative to root for scan limiting.
func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(os.PathSeparator)))
}

// ParseFile reads one SKILL.md file and extracts its required metadata.
func ParseFile(path string) (Skill, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("read skill: %w", err)
	}
	meta, err := parseFrontmatter(string(contents))
	if err != nil {
		return Skill{}, err
	}
	name := sanitizeLine(meta.Name)
	if name == "" {
		name = defaultSkillName(path)
	}
	description := sanitizeLine(meta.Description)
	if len([]rune(name)) > maxNameLen {
		return Skill{}, fmt.Errorf("name exceeds %d characters", maxNameLen)
	}
	if len([]rune(description)) > maxDescriptionLen {
		return Skill{}, fmt.Errorf("description exceeds %d characters", maxDescriptionLen)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return Skill{Name: name, Description: description, Path: filepath.Clean(abs)}, nil
}

// parseFrontmatter extracts the YAML block at the top of a skill file.
func parseFrontmatter(contents string) (skillFrontmatter, error) {
	normalized := strings.ReplaceAll(contents, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return skillFrontmatter{}, fmt.Errorf("missing YAML frontmatter")
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return skillFrontmatter{}, fmt.Errorf("missing YAML frontmatter close")
	}
	var meta skillFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return skillFrontmatter{}, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	return meta, nil
}

// sanitizeLine collapses frontmatter values into one prompt-safe line.
func sanitizeLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// defaultSkillName falls back to the directory name when frontmatter omits name.
func defaultSkillName(path string) string {
	dir := filepath.Base(filepath.Dir(path))
	if dir == "." || dir == string(os.PathSeparator) {
		return "skill"
	}
	return sanitizeLine(dir)
}
