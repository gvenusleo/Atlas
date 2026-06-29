package prompt

import (
	"errors"
	"os"
	"path/filepath"
)

const maxInstructionBytes = 64 * 1024

// InstructionFile is a loaded AGENTS.md instruction file.
type InstructionFile struct {
	Path    string
	Content string
}

// LoadInstructions loads only the global and current-directory AGENTS.md files.
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
