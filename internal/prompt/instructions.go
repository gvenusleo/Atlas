package prompt

import (
	"errors"
	"os"
	"path/filepath"
)

const maxInstructionBytes = 64 * 1024

// InstructionFile 是一个已加载的 AGENTS.md 指令文件。
type InstructionFile struct {
	Path    string
	Content string
}

// LoadInstructions 只加载全局和当前目录下的 AGENTS.md。
func LoadInstructions(cwd string) ([]InstructionFile, error) {
	globalPath, err := globalInstructionsPath()
	if err != nil {
		return nil, err
	}
	if cwd == "" {
		cwd = "."
	}
	currentPath := filepath.Join(cwd, "AGENTS.md")

	paths := []string{globalPath, currentPath}
	files := make([]InstructionFile, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}

		file, err := loadInstructionFile(absPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func globalInstructionsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".atlas", "AGENTS.md"), nil
}

func loadInstructionFile(path string) (InstructionFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return InstructionFile{}, err
	}
	if info.IsDir() {
		return InstructionFile{}, os.ErrNotExist
	}
	if info.Size() > maxInstructionBytes {
		return InstructionFile{}, errors.New("AGENTS.md is too large")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return InstructionFile{}, err
	}
	return InstructionFile{
		Path:    path,
		Content: string(content),
	}, nil
}
