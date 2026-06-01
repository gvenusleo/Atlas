package skills

import (
	"path/filepath"
	"strings"
)

// SelectMentioned returns enabled skills explicitly mentioned in user input.
func SelectMentioned(skills []Skill, input string) []Skill {
	names, paths := ExtractMentions(input)
	nameCounts := make(map[string]int)
	for _, skill := range skills {
		nameCounts[skill.Name]++
	}
	var selected []Skill
	seen := make(map[string]bool)
	for _, skill := range skills {
		if seen[skill.Path] {
			continue
		}
		if paths[pathKey(skill.Path)] {
			selected = append(selected, skill)
			seen[skill.Path] = true
			continue
		}
		if names[skill.Name] && nameCounts[skill.Name] == 1 {
			selected = append(selected, skill)
			seen[skill.Path] = true
		}
	}
	return selected
}

// ExtractMentions parses $skill-name and [$skill](skill://path) mentions.
func ExtractMentions(input string) (map[string]bool, map[string]bool) {
	names := make(map[string]bool)
	paths := make(map[string]bool)
	for index := 0; index < len(input); {
		if input[index] == '[' {
			if name, path, next, ok := parseLinkedMention(input, index); ok {
				names[name] = true
				paths[pathKey(path)] = true
				index = next
				continue
			}
		}
		if input[index] != '$' {
			index++
			continue
		}
		start := index + 1
		if start >= len(input) || !isMentionChar(input[start]) {
			index++
			continue
		}
		end := start + 1
		for end < len(input) && isMentionChar(input[end]) {
			end++
		}
		name := input[start:end]
		if !isCommonEnvVar(name) {
			names[name] = true
		}
		index = end
	}
	return names, paths
}

func parseLinkedMention(input string, start int) (string, string, int, bool) {
	if start+2 >= len(input) || input[start+1] != '$' {
		return "", "", start, false
	}
	nameStart := start + 2
	if !isMentionChar(input[nameStart]) {
		return "", "", start, false
	}
	nameEnd := nameStart + 1
	for nameEnd < len(input) && isMentionChar(input[nameEnd]) {
		nameEnd++
	}
	if nameEnd >= len(input) || input[nameEnd] != ']' {
		return "", "", start, false
	}
	pathStart := nameEnd + 1
	for pathStart < len(input) && input[pathStart] == ' ' {
		pathStart++
	}
	if pathStart >= len(input) || input[pathStart] != '(' {
		return "", "", start, false
	}
	pathEnd := pathStart + 1
	for pathEnd < len(input) && input[pathEnd] != ')' {
		pathEnd++
	}
	if pathEnd >= len(input) {
		return "", "", start, false
	}
	path := strings.TrimSpace(input[pathStart+1 : pathEnd])
	if path == "" {
		return "", "", start, false
	}
	return input[nameStart:nameEnd], strings.TrimPrefix(path, "skill://"), pathEnd + 1, true
}

func pathKey(path string) string {
	if strings.HasPrefix(path, "file://") {
		path = strings.TrimPrefix(path, "file://")
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func isMentionChar(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_' || b == '-' || b == ':'
}

func isCommonEnvVar(name string) bool {
	switch strings.ToUpper(name) {
	case "PATH", "HOME", "USER", "SHELL", "PWD", "TMPDIR", "TEMP", "TMP", "LANG", "TERM":
		return true
	default:
		return false
	}
}
