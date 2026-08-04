package tool

import (
	"fmt"
	"path/filepath"
	"strings"
)

func resolveFilePath(cwd, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if cwd == "" {
		return filepath.Clean(path), nil
	}
	return filepath.Join(cwd, path), nil
}
