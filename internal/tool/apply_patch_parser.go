package tool

import (
	"fmt"
	"strings"
)

const (
	patchBeginMarker  = "*** Begin Patch"
	patchEndMarker    = "*** End Patch"
	patchAddMarker    = "*** Add File: "
	patchUpdateMarker = "*** Update File: "
	patchDeleteMarker = "*** Delete File: "
	patchMoveMarker   = "*** Move to: "
	patchEOFMarker    = "*** End of File"
)

type patchActionKind int

const (
	patchAdd patchActionKind = iota
	patchUpdate
	patchDelete
)

type patchAction struct {
	kind     patchActionKind
	path     string
	movePath string
	content  string
	chunks   []patchChunk
}

type patchChunk struct {
	context    string
	hasContext bool
	oldLines   []string
	newLines   []string
	endOfFile  bool
}

func parsePatch(input string) ([]patchAction, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(input, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != patchBeginMarker || strings.TrimSpace(lines[len(lines)-1]) != patchEndMarker {
		return nil, fmt.Errorf("apply_patch must start with %q and end with %q", patchBeginMarker, patchEndMarker)
	}

	var actions []patchAction
	for index := 1; index < len(lines)-1; {
		line := lines[index]
		switch {
		case strings.HasPrefix(line, patchAddMarker):
			action, next, err := parseAddPatch(lines, index)
			if err != nil {
				return nil, err
			}
			actions = append(actions, action)
			index = next
		case strings.HasPrefix(line, patchDeleteMarker):
			path, err := patchHeaderPath(line, patchDeleteMarker, index+1)
			if err != nil {
				return nil, err
			}
			actions = append(actions, patchAction{kind: patchDelete, path: path})
			index++
		case strings.HasPrefix(line, patchUpdateMarker):
			action, next, err := parseUpdatePatch(lines, index)
			if err != nil {
				return nil, err
			}
			actions = append(actions, action)
			index = next
		default:
			return nil, fmt.Errorf("invalid patch hunk on line %d: %q", index+1, line)
		}
	}
	if len(actions) == 0 {
		return nil, fmt.Errorf("apply_patch contains no file operations")
	}
	return actions, nil
}

func parseAddPatch(lines []string, index int) (patchAction, int, error) {
	path, err := patchHeaderPath(lines[index], patchAddMarker, index+1)
	if err != nil {
		return patchAction{}, 0, err
	}
	index++
	var content []string
	for index < len(lines)-1 && !isPatchActionHeader(lines[index]) {
		line := lines[index]
		if !strings.HasPrefix(line, "+") {
			return patchAction{}, 0, fmt.Errorf("invalid add line %d: every content line must start with '+'", index+1)
		}
		content = append(content, strings.TrimPrefix(line, "+"))
		index++
	}
	text := ""
	if len(content) > 0 {
		text = strings.Join(content, "\n") + "\n"
	}
	return patchAction{kind: patchAdd, path: path, content: text}, index, nil
}

func parseUpdatePatch(lines []string, index int) (patchAction, int, error) {
	path, err := patchHeaderPath(lines[index], patchUpdateMarker, index+1)
	if err != nil {
		return patchAction{}, 0, err
	}
	action := patchAction{kind: patchUpdate, path: path}
	index++
	if index < len(lines)-1 && strings.HasPrefix(lines[index], patchMoveMarker) {
		action.movePath, err = patchHeaderPath(lines[index], patchMoveMarker, index+1)
		if err != nil {
			return patchAction{}, 0, err
		}
		index++
	}

	var current *patchChunk
	flush := func() {
		if current != nil {
			action.chunks = append(action.chunks, *current)
			current = nil
		}
	}
	for index < len(lines)-1 && !isPatchActionHeader(lines[index]) {
		line := lines[index]
		if current != nil && current.endOfFile {
			return patchAction{}, 0, fmt.Errorf("unexpected line after %q on line %d", patchEOFMarker, index+1)
		}
		switch {
		case line == "@@" || strings.HasPrefix(line, "@@ "):
			flush()
			current = &patchChunk{}
			if line != "@@" {
				current.context = strings.TrimPrefix(line, "@@ ")
				current.hasContext = true
			}
		case line == patchEOFMarker:
			if current == nil {
				return patchAction{}, 0, fmt.Errorf("unexpected %q on line %d", patchEOFMarker, index+1)
			}
			current.endOfFile = true
		case strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " "):
			if current == nil {
				current = &patchChunk{}
			}
			text := line[1:]
			if line[0] != '+' {
				current.oldLines = append(current.oldLines, text)
			}
			if line[0] != '-' {
				current.newLines = append(current.newLines, text)
			}
		default:
			return patchAction{}, 0, fmt.Errorf("invalid update line %d: %q", index+1, line)
		}
		index++
	}
	flush()
	if len(action.chunks) == 0 && action.movePath == "" {
		return patchAction{}, 0, fmt.Errorf("update file hunk for %q is empty", path)
	}
	for _, chunk := range action.chunks {
		if len(chunk.oldLines) == 0 && len(chunk.newLines) == 0 {
			return patchAction{}, 0, fmt.Errorf("update file hunk for %q contains an empty chunk", path)
		}
	}
	return action, index, nil
}

func patchHeaderPath(line, marker string, lineNumber int) (string, error) {
	path := strings.TrimSpace(strings.TrimPrefix(line, marker))
	if path == "" {
		return "", fmt.Errorf("patch path is required on line %d", lineNumber)
	}
	return path, nil
}

func isPatchActionHeader(line string) bool {
	return strings.HasPrefix(line, patchAddMarker) || strings.HasPrefix(line, patchUpdateMarker) || strings.HasPrefix(line, patchDeleteMarker)
}
