// Package prompt 加载指令和技能目录，组装 Atlas 的系统提示词。
package prompt

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gvenusleo/atlas/internal/skills"
)

const baseSystemPrompt = `你是名为 Atlas 的通用助手，通过工具和技能帮助用户完成任务。

工作方式：
- 修改已有文件前，先读取相关内容。
- 列目录和搜索文件使用 shell，读取文件内容使用 read。
- 新建文件或完整重写使用 write，局部替换使用 edit。
- 同一文件的多处独立修改，尽量合并为一次 edit 调用。
- read 返回截断提示时，按提示继续读取所需部分。
- 工具执行失败时，根据错误信息处理；无法完成时说明原因。
- 完成操作后进行必要验证，区分已验证的结果和未验证的判断。
- 回复简洁，明确指出相关文件路径。

指令文件：
- AGENTS.md 提供用户可覆盖的默认指令；全局文件作用于整个会话，目录中的文件作用于该目录及其子目录。
- 同一事项冲突时，优先级从高到低为：用户会话中的明确要求、任务所在目录的指令、上层目录的指令、全局指令；不冲突的指令继续生效。
- 处理工作目录的子目录前，检查目标路径上是否有尚未加载的 AGENTS.md，并用 read 读取。`

// Resources 是一次资源发现的结果，提示词和 skills 来自同一份快照。
type Resources struct {
	SystemPrompt string
	Skills       []skills.Skill
}

// Build 在会话创建或恢复时组装提示词，并返回供会话和 App 共用的 skills 快照。
func Build(workdir string) (Resources, error) {
	cwd, err := filepath.Abs(workdir)
	if err != nil {
		return Resources{}, fmt.Errorf("工作目录解析失败: %w", err)
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return Resources{}, fmt.Errorf("%s 检查失败: %w", cwd, err)
	}
	if !info.IsDir() {
		return Resources{}, fmt.Errorf("%s 不是目录", cwd)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Resources{}, fmt.Errorf("定位主目录失败: %w", err)
	}

	instructions, err := loadInstructions(home, cwd)
	if err != nil {
		return Resources{}, err
	}
	discoveredSkills, err := skills.Discover(cwd)
	if err != nil {
		return Resources{}, err
	}
	skillsPrompt, err := skills.FormatPrompt(discoveredSkills)
	if err != nil {
		return Resources{}, err
	}

	sections := []string{baseSystemPrompt}
	if len(instructions) > 0 {
		sections = append(sections, "用户与工作区指令：\n\n"+strings.Join(instructions, "\n\n"))
	}
	if skillsPrompt != "" {
		sections = append(sections, skillsPrompt)
	}

	environment := struct {
		XMLName xml.Name `xml:"environment"`
		Cwd     string   `xml:"cwd"`
		OS      string   `xml:"os"`
	}{Cwd: cwd, OS: runtime.GOOS}
	data, err := xml.MarshalIndent(environment, "", "  ")
	if err != nil {
		return Resources{}, fmt.Errorf("运行环境生成失败: %w", err)
	}
	sections = append(sections, string(data))
	return Resources{SystemPrompt: strings.Join(sections, "\n\n"), Skills: discoveredSkills}, nil
}

// loadInstructions 全局优先选择 Atlas 指令，再按根目录到工作目录加载 AGENTS.md。
func loadInstructions(home, cwd string) ([]string, error) {
	atlasPath := filepath.Join(home, ".atlas", "AGENTS.md")
	agentsPath := filepath.Join(home, ".agents", "AGENTS.md")
	globalPath := atlasPath
	// 按存在性选择，空文件或读取失败不回退；失效符号链接也交给读取时报错。
	if _, err := os.Lstat(atlasPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s 检查失败: %w", atlasPath, err)
		}
		globalPath = agentsPath
	}
	global, err := readInstruction(globalPath, "整个会话")
	if err != nil {
		return nil, err
	}
	var instructions []string
	if global != "" {
		instructions = append(instructions, global)
	}

	var dirs []string
	for dir := cwd; ; dir = filepath.Dir(dir) {
		dirs = append(dirs, dir)
		if filepath.Dir(dir) == dir {
			break
		}
	}
	slices.Reverse(dirs)
	for _, dir := range dirs {
		path := filepath.Join(dir, "AGENTS.md")
		if path == atlasPath || path == agentsPath {
			continue
		}
		instruction, err := readInstruction(path, dir)
		if err != nil {
			return nil, err
		}
		if instruction != "" {
			instructions = append(instructions, instruction)
		}
	}
	return instructions, nil
}

// readInstruction 略过缺失或空文件；来源、作用范围和正文通过 XML 编码保留。
func readInstruction(path, scope string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// 只有路径本身不存在才略过，失效符号链接属于读取错误。
		if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
			return "", nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%s 不是 UTF-8 文本", path)
	}
	content := strings.TrimPrefix(string(data), "\uFEFF")
	if strings.TrimSpace(content) == "" {
		return "", nil
	}

	instruction := struct {
		XMLName xml.Name `xml:"instructions"`
		Path    string   `xml:"path,attr"`
		Scope   string   `xml:"scope,attr"`
		Content string   `xml:",chardata"`
	}{Path: path, Scope: scope, Content: "\n" + content + "\n"}
	encoded, err := xml.Marshal(instruction)
	if err != nil {
		return "", fmt.Errorf("%s 指令生成失败: %w", path, err)
	}
	return string(encoded), nil
}
