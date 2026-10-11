package skills

import (
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// 定义 skills 加载限制。
const (
	maxNameLength        = 64
	maxDescriptionLength = 1024
	maxFileBytes         = 10 << 20
)

// Skill 只保存基本信息，正文通过 read tool 按需读取。
type Skill struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	FilePath    string `xml:"filePath"`
}

// Load 扫描 skills 来源的子目录，按名称排序；缺失目录略过，加载错误直接返回。
func Load(dirs ...string) ([]Skill, error) {
	var skills []Skill
	names := make(map[string]string)
	paths := make(map[string]bool)

	for _, dir := range dirs {
		root, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("%s 路径解析失败: %w", dir, err)
		}

		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s 扫描失败: %w", root, err)
		}

		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			// 允许 skill 目录通过符号链接共享。
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			dirPath := filepath.Join(root, entry.Name())
			info, err := os.Stat(dirPath)
			if err != nil {
				return nil, fmt.Errorf("%s 检查失败: %w", dirPath, err)
			}
			if !info.IsDir() {
				continue
			}

			path := filepath.Join(dirPath, "SKILL.md")
			info, err = os.Stat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("%s 检查失败: %w", path, err)
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("%s 不是普通文件", path)
			}
			if info.Size() > maxFileBytes {
				return nil, fmt.Errorf("%s 文件大小超过 10 MiB 限制", path)
			}

			realPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil, fmt.Errorf("%s 符号链接解析失败: %w", path, err)
			}
			if paths[realPath] {
				continue
			}

			skill, err := loadSkill(path)
			if err != nil {
				return nil, fmt.Errorf("%s 加载失败: %w", path, err)
			}
			if previous, exists := names[skill.Name]; exists {
				return nil, fmt.Errorf(
					"技能 %q 同名冲突: %s 与 %s",
					skill.Name, previous, path,
				)
			}

			names[skill.Name] = path
			paths[realPath] = true
			skills = append(skills, skill)
		}
	}
	slices.SortFunc(skills, func(a, b Skill) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return skills, nil
}

// loadSkill 读取文件并解析元信息，不保存正文。
func loadSkill(path string) (Skill, error) {
	file, err := os.Open(path)
	if err != nil {
		return Skill{}, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return Skill{}, err
	}
	if len(data) > maxFileBytes {
		return Skill{}, errors.New("文件大小超过 10 MiB 限制")
	}
	if !utf8.Valid(data) {
		return Skill{}, errors.New("文件不是 UTF-8 文本")
	}

	header, _, err := splitFrontmatter(string(data))
	if err != nil {
		return Skill{}, err
	}

	// 使用原始 YAML 值检查类型，避免把数字或布尔值当成字符串。
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(header), &fields); err != nil {
		return Skill{}, fmt.Errorf("YAML 解析失败: %w", err)
	}

	name, ok := fields["name"].(string)
	if !ok {
		return Skill{}, errors.New("name 必须是字符串")
	}
	if err := validateName(name); err != nil {
		return Skill{}, err
	}

	description, ok := fields["description"].(string)
	if !ok {
		return Skill{}, errors.New("description 必须是字符串")
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return Skill{}, errors.New("description 不能为空")
	}
	if utf8.RuneCountInString(description) > maxDescriptionLength {
		return Skill{}, fmt.Errorf(
			"description 不能超过 %d 个字符", maxDescriptionLength,
		)
	}

	return Skill{
		Name:        name,
		Description: description,
		FilePath:    path,
	}, nil
}

// splitFrontmatter 提取 YAML 头和正文，分隔符必须独占一行；支持 BOM 和 CRLF。
func splitFrontmatter(text string) (string, string, error) {
	text = strings.TrimPrefix(text, "\uFEFF")
	text = strings.ReplaceAll(text, "\r\n", "\n")

	first, rest, found := strings.Cut(text, "\n")
	if !found || first != "---" {
		return "", "", errors.New("文件必须以 YAML frontmatter 开始")
	}

	end := 0
	for line := range strings.SplitSeq(rest, "\n") {
		if line == "---" {
			bodyStart := min(end+len(line)+1, len(rest))
			return rest[:end], rest[bodyStart:], nil
		}
		end += len(line) + 1
	}
	return "", "", errors.New("YAML frontmatter 缺少结束分隔符")
}

// ReadBody 读取技能正文，去除 frontmatter 和首尾空白；读取失败带技能路径返回。
func ReadBody(skill Skill) (string, error) {
	data, err := os.ReadFile(skill.FilePath)
	if err != nil {
		return "", fmt.Errorf("读取 %s 失败: %w", skill.FilePath, err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%s 不是 UTF-8 文本", skill.FilePath)
	}
	_, body, err := splitFrontmatter(string(data))
	if err != nil {
		return "", fmt.Errorf("%s 加载失败: %w", skill.FilePath, err)
	}
	return strings.TrimSpace(body), nil
}

// validateName 校验名称长度与字符，不要求名称和目录名相同。
func validateName(name string) error {
	if len(name) == 0 || len(name) > maxNameLength {
		return fmt.Errorf("name 长度必须在 1 到 %d 个字符之间", maxNameLength)
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return errors.New("name 只能包含小写字母、数字和连字符")
		}
	}
	if strings.HasPrefix(name, "-") ||
		strings.HasSuffix(name, "-") ||
		strings.Contains(name, "--") {
		return errors.New("name 不能以连字符开头或结尾，也不能包含连续连字符")
	}
	return nil
}

// FormatPrompt 生成技能目录，XML 编码负责转义名称、描述和路径。
func FormatPrompt(skills []Skill) (string, error) {
	if len(skills) == 0 {
		return "", nil
	}

	catalog := struct {
		XMLName xml.Name `xml:"available_skills"`
		Skills  []Skill  `xml:"skill"`
	}{
		Skills: skills,
	}
	data, err := xml.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return "", fmt.Errorf("技能目录生成失败: %w", err)
	}

	return "以下技能提供特定任务的操作说明。" +
		"用户点名技能或任务符合其描述时，先用 read 读取对应的 SKILL.md。" +
		"读取结果被截断时，继续读取剩余部分。" +
		"技能中引用的相对路径，以 SKILL.md 所在目录为基准。\n\n" +
		string(data), nil
}
