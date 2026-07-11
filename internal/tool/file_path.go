package tool

import "path/filepath"

func resolveToolPath(cwd, pathValue string) string {
	if pathValue == "" {
		if cwd != "" {
			return cwd
		}
		return "."
	}
	if filepath.IsAbs(pathValue) || cwd == "" {
		return filepath.Clean(pathValue)
	}
	return filepath.Clean(filepath.Join(cwd, pathValue))
}
