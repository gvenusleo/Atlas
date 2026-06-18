// Package skill 加载本地 Atlas skill 的元数据和正文。
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Skill 表示一个从 SKILL.md 加载的本地 skill。
type Skill struct {
	Name                   string
	Description            string
	DisableModelInvocation bool
	Dir                    string
	Path                   string
	Content                string
}

// Summary 是可以直接放进系统提示词的 skill 摘要。
type Summary struct {
	Name        string
	Description string
}

// Catalog 保存当前运行目录可用的 skill。
type Catalog struct {
	skills []Skill
	byName map[string]Skill
}

type root struct {
	path string
}

// Load 扫描用户级和当前目录级 skill。
func Load(cwd string) (*Catalog, error) {
	roots, err := rootsFor(cwd)
	if err != nil {
		return nil, err
	}
	return loadFromRoots(roots)
}

// NewCatalog 从给定 skill 创建目录，主要供测试和调用方注入已知集合。
func NewCatalog(skills []Skill) (*Catalog, error) {
	byName := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		if strings.TrimSpace(skill.Name) == "" {
			return nil, fmt.Errorf("skill name is required")
		}
		if _, ok := byName[skill.Name]; ok {
			return nil, fmt.Errorf("duplicate skill %q", skill.Name)
		}
		byName[skill.Name] = skill
	}
	return newCatalogFromMap(byName), nil
}

// Summaries 返回模型可见的 skill 摘要。
func (c *Catalog) Summaries() []Summary {
	if c == nil {
		return nil
	}
	summaries := make([]Summary, 0, len(c.skills))
	for _, skill := range c.skills {
		if skill.DisableModelInvocation {
			continue
		}
		summaries = append(summaries, Summary{
			Name:        skill.Name,
			Description: skill.Description,
		})
	}
	return summaries
}

// Lookup 按名称返回模型可加载的 skill。
func (c *Catalog) Lookup(name string) (Skill, bool) {
	if c == nil {
		return Skill{}, false
	}
	skill, ok := c.byName[name]
	if !ok || skill.DisableModelInvocation {
		return Skill{}, false
	}
	return skill, true
}

func rootsFor(cwd string) ([]root, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []root{
		{path: filepath.Join(home, ".atlas", "skills")},
		{path: filepath.Join(home, ".agents", "skills")},
		{path: filepath.Join(cwd, ".atlas", "skills")},
		{path: filepath.Join(cwd, ".agents", "skills")},
	}, nil
}

func loadFromRoots(roots []root) (*Catalog, error) {
	byName := make(map[string]Skill)
	for _, root := range roots {
		skills, err := loadRoot(root.path)
		if err != nil {
			return nil, err
		}
		for _, skill := range skills {
			byName[skill.Name] = skill
		}
	}
	return newCatalogFromMap(byName), nil
}

func loadRoot(path string) ([]Skill, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(path, entry.Name(), "SKILL.md")
		skill, err := loadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

func loadFile(path string) (Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	skill, err := parse(path, string(content))
	if err != nil {
		return Skill{}, err
	}
	skill.Path = path
	skill.Dir = filepath.Dir(path)
	skill.Content = string(content)
	return skill, nil
}

func parse(path, content string) (Skill, error) {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return Skill{}, fmt.Errorf("%s: missing frontmatter", path)
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return Skill{}, fmt.Errorf("%s: missing frontmatter terminator", path)
	}

	meta := make(map[string]string)
	for _, line := range lines[1:end] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Skill{}, fmt.Errorf("%s: invalid frontmatter line %q", path, line)
		}
		meta[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}

	skill := Skill{
		Name:        meta["name"],
		Description: meta["description"],
	}
	if skill.Name == "" {
		return Skill{}, fmt.Errorf("%s: skill name is required", path)
	}
	if skill.Description == "" {
		return Skill{}, fmt.Errorf("%s: skill description is required", path)
	}
	if value := meta["disable-model-invocation"]; value != "" {
		disabled, err := strconv.ParseBool(value)
		if err != nil {
			return Skill{}, fmt.Errorf("%s: invalid disable-model-invocation: %w", path, err)
		}
		skill.DisableModelInvocation = disabled
	}
	return skill, nil
}

func unquote(value string) string {
	if value == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2 {
		return strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
	}
	return value
}

func newCatalogFromMap(byName map[string]Skill) *Catalog {
	skills := make([]Skill, 0, len(byName))
	for _, skill := range byName {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	catalogByName := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		catalogByName[skill.Name] = skill
	}
	return &Catalog{
		skills: skills,
		byName: catalogByName,
	}
}
