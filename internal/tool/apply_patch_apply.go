package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/liuyuxin/atlas/internal/model"
)

const maxPatchDiffBytes = 512 * 1024

var patchLocks = newPathLockSet()

type patchFileState struct {
	exists  bool
	content string
	mode    os.FileMode
}

type patchMutation struct {
	path string
	old  patchFileState
	new  patchFileState
}

func applyPatchActions(ctx context.Context, cwd string, actions []patchAction) (RunResult, error) {
	paths := resolvedPatchPaths(actions, cwd)
	unlock := patchLocks.lock(paths)
	defer unlock()

	originals := make(map[string]patchFileState, len(paths))
	states := make(map[string]patchFileState, len(paths))
	load := func(path string) (patchFileState, error) {
		if state, ok := states[path]; ok {
			return state, nil
		}
		state, err := readPatchFile(path)
		if err != nil {
			return patchFileState{}, err
		}
		originals[path] = state
		states[path] = state
		return state, nil
	}

	for _, action := range actions {
		if err := ctx.Err(); err != nil {
			return RunResult{}, err
		}
		path := absolutePatchPath(cwd, action.path)
		state, err := load(path)
		if err != nil {
			return RunResult{}, err
		}
		switch action.kind {
		case patchAdd:
			mode := state.mode
			if !state.exists {
				mode = 0o644
			}
			states[path] = patchFileState{exists: true, content: action.content, mode: mode}
		case patchDelete:
			if !state.exists {
				return RunResult{}, fmt.Errorf("cannot delete missing file %s", path)
			}
			states[path] = patchFileState{}
		case patchUpdate:
			if !state.exists {
				return RunResult{}, fmt.Errorf("cannot update missing file %s", path)
			}
			updated, err := applyPatchChunks(state.content, action.chunks, path)
			if err != nil {
				return RunResult{}, err
			}
			updatedState := patchFileState{exists: true, content: updated, mode: state.mode}
			if action.movePath == "" {
				states[path] = updatedState
				continue
			}
			destination := absolutePatchPath(cwd, action.movePath)
			if destination == path {
				return RunResult{}, fmt.Errorf("cannot move %s onto itself", path)
			}
			if _, err := load(destination); err != nil {
				return RunResult{}, err
			}
			states[path] = patchFileState{}
			states[destination] = updatedState
		}
	}

	mutations := patchMutations(originals, states)
	if len(mutations) == 0 {
		return RunResult{}, fmt.Errorf("apply_patch made no changes")
	}
	for path, original := range originals {
		current, err := readPatchFile(path)
		if err != nil {
			return RunResult{}, err
		}
		if !samePatchState(current, original) {
			return RunResult{}, fmt.Errorf("file changed while applying patch: %s", path)
		}
	}

	result := RunResult{Metadata: model.ToolMetadata{Locations: patchLocations(mutations)}}
	var completed []patchMutation
	for _, mutation := range mutations {
		if !mutation.new.exists {
			continue
		}
		if err := ctx.Err(); err != nil {
			setPatchResult(&result, completed, cwd)
			return result, err
		}
		if err := os.MkdirAll(filepath.Dir(mutation.path), 0o755); err != nil {
			setPatchResult(&result, completed, cwd)
			return result, err
		}
		if err := os.WriteFile(mutation.path, []byte(mutation.new.content), mutation.new.mode.Perm()); err != nil {
			if actual, readErr := readPatchFile(mutation.path); readErr == nil && !samePatchState(actual, mutation.old) {
				completed = append(completed, patchMutation{path: mutation.path, old: mutation.old, new: actual})
			}
			setPatchResult(&result, completed, cwd)
			return result, fmt.Errorf("write %s: %w; target may be partially modified", mutation.path, err)
		}
		completed = append(completed, mutation)
		if err := os.Chmod(mutation.path, mutation.new.mode.Perm()); err != nil {
			setPatchResult(&result, completed, cwd)
			return result, fmt.Errorf("set permissions on %s: %w", mutation.path, err)
		}
	}
	for _, mutation := range mutations {
		if mutation.new.exists {
			continue
		}
		if err := os.Remove(mutation.path); err != nil {
			setPatchResult(&result, completed, cwd)
			return result, fmt.Errorf("delete %s: %w", mutation.path, err)
		}
		completed = append(completed, mutation)
	}
	setPatchResult(&result, completed, cwd)
	return result, nil
}

func setPatchResult(result *RunResult, completed []patchMutation, cwd string) {
	result.Metadata.Diffs = patchDiffs(completed)
	result.Content = patchSummary(completed, cwd)
}

func applyPatchChunks(content string, chunks []patchChunk, path string) (string, error) {
	if len(chunks) == 0 {
		return content, nil
	}
	bom := strings.HasPrefix(content, "\uFEFF")
	if bom {
		content = strings.TrimPrefix(content, "\uFEFF")
	}
	lineEnding := "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	position := 0
	for _, chunk := range chunks {
		if chunk.hasContext {
			index := findPatchSequence(lines, []string{chunk.context}, position, false)
			if index < 0 {
				return "", fmt.Errorf("failed to find context %q in %s", chunk.context, path)
			}
			position = index + 1
		}
		if len(chunk.oldLines) == 0 {
			index := position
			if !chunk.hasContext || chunk.endOfFile {
				index = len(lines)
			}
			lines = replacePatchLines(lines, index, 0, chunk.newLines)
			position = index + len(chunk.newLines)
			continue
		}
		index := findPatchSequence(lines, chunk.oldLines, position, chunk.endOfFile)
		if index < 0 {
			return "", fmt.Errorf("failed to find expected lines in %s:\n%s", path, strings.Join(chunk.oldLines, "\n"))
		}
		lines = replacePatchLines(lines, index, len(chunk.oldLines), chunk.newLines)
		position = index + len(chunk.newLines)
	}
	updated := strings.Join(lines, "\n")
	if len(lines) > 0 {
		updated += "\n"
	}
	if lineEnding == "\r\n" {
		updated = strings.ReplaceAll(updated, "\n", "\r\n")
	}
	if bom {
		updated = "\uFEFF" + updated
	}
	return updated, nil
}

func findPatchSequence(lines, pattern []string, start int, endOfFile bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}
	if endOfFile {
		start = len(lines) - len(pattern)
		if slicesEqual(lines[start:], pattern) {
			return start
		}
		return -1
	}
	for index := start; index+len(pattern) <= len(lines); index++ {
		if slicesEqual(lines[index:index+len(pattern)], pattern) {
			return index
		}
	}
	return -1
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func replacePatchLines(lines []string, start, oldLength int, replacement []string) []string {
	result := make([]string, 0, len(lines)-oldLength+len(replacement))
	result = append(result, lines[:start]...)
	result = append(result, replacement...)
	result = append(result, lines[start+oldLength:]...)
	return result
}

func readPatchFile(path string) (patchFileState, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return patchFileState{}, nil
	}
	if err != nil {
		return patchFileState{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return patchFileState{}, fmt.Errorf("apply_patch does not support symbolic links: %s", path)
	}
	if !info.Mode().IsRegular() {
		return patchFileState{}, fmt.Errorf("apply_patch path is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return patchFileState{}, err
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return patchFileState{}, fmt.Errorf("apply_patch path is not a UTF-8 text file: %s", path)
	}
	return patchFileState{exists: true, content: string(data), mode: info.Mode()}, nil
}

func resolvedPatchPaths(actions []patchAction, cwd string) []string {
	seen := make(map[string]struct{})
	for _, action := range actions {
		seen[absolutePatchPath(cwd, action.path)] = struct{}{}
		if action.movePath != "" {
			seen[absolutePatchPath(cwd, action.movePath)] = struct{}{}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func absolutePatchPath(cwd, path string) string {
	resolved := resolveToolPath(cwd, path)
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return resolved
	}
	return filepath.Clean(absolute)
}

func patchMutations(originals, states map[string]patchFileState) []patchMutation {
	mutations := make([]patchMutation, 0, len(states))
	for path, state := range states {
		original := originals[path]
		if samePatchState(original, state) {
			continue
		}
		mutations = append(mutations, patchMutation{path: path, old: original, new: state})
	}
	sort.Slice(mutations, func(i, j int) bool { return mutations[i].path < mutations[j].path })
	return mutations
}

func samePatchState(left, right patchFileState) bool {
	return left.exists == right.exists && left.content == right.content && left.mode.Perm() == right.mode.Perm()
}

func patchLocations(mutations []patchMutation) []model.ToolLocation {
	locations := make([]model.ToolLocation, 0, len(mutations))
	for _, mutation := range mutations {
		locations = append(locations, model.ToolLocation{Path: mutation.path})
	}
	return locations
}

func patchDiffs(mutations []patchMutation) []model.ToolDiff {
	var diffs []model.ToolDiff
	total := 0
	for _, mutation := range mutations {
		oldText := (*string)(nil)
		if mutation.old.exists {
			text := mutation.old.content
			oldText = &text
			total += len(text)
		}
		newText := ""
		if mutation.new.exists {
			newText = mutation.new.content
			total += len(newText)
		}
		if total > maxPatchDiffBytes {
			break
		}
		diffs = append(diffs, model.ToolDiff{Path: mutation.path, OldText: oldText, NewText: newText})
	}
	return diffs
}

func patchSummary(mutations []patchMutation, cwd string) string {
	if len(mutations) == 0 {
		return ""
	}
	var lines []string
	for _, mutation := range mutations {
		kind := "M"
		if !mutation.old.exists {
			kind = "A"
		} else if !mutation.new.exists {
			kind = "D"
		}
		path := mutation.path
		if relative, err := filepath.Rel(cwd, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			path = relative
		}
		lines = append(lines, kind+" "+path)
	}
	return "Success. Updated files:\n" + strings.Join(lines, "\n")
}

type pathLockSet struct {
	mu    sync.Mutex
	locks map[string]*pathLockEntry
}

type pathLockEntry struct {
	mu   sync.Mutex
	refs int
}

func newPathLockSet() *pathLockSet {
	return &pathLockSet{locks: make(map[string]*pathLockEntry)}
}

func (s *pathLockSet) lock(paths []string) func() {
	s.mu.Lock()
	locks := make([]*pathLockEntry, 0, len(paths))
	for _, path := range paths {
		lock := s.locks[path]
		if lock == nil {
			lock = &pathLockEntry{}
			s.locks[path] = lock
		}
		lock.refs++
		locks = append(locks, lock)
	}
	s.mu.Unlock()
	for _, lock := range locks {
		lock.mu.Lock()
	}
	return func() {
		for index := len(locks) - 1; index >= 0; index-- {
			locks[index].mu.Unlock()
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for index, path := range paths {
			locks[index].refs--
			if locks[index].refs == 0 {
				delete(s.locks, path)
			}
		}
	}
}
