package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// vcsExcludeGlobs excludes version-control metadata directories, matching the
// .git/.hg/.svn skipping behavior of the walk fallback.
var vcsExcludeGlobs = []string{"!.git", "!.hg", "!.svn"}

// rgBinary returns the ripgrep executable path, or "" when unavailable. Tests
// force the empty path so the doublestar fallback is exercised deterministically
// without depending on a locally installed rg.
var rgBinary = sync.OnceValue(func() string {
	if testing.Testing() {
		return ""
	}
	path, err := exec.LookPath("rg")
	if err != nil {
		return ""
	}
	return path
})

// rgGlobPattern anchors a glob pattern to the search root so "*.go" matches
// only top-level files, matching Atlas's glob semantics (a single * does not
// cross directory boundaries).
func rgGlobPattern(pattern string) string {
	pattern = filepath.ToSlash(pattern)
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	return pattern
}

// rgGlobFiles runs `rg --files` filtered by the glob pattern and returns
// root-relative, forward-slash file paths. ok is false when rg is unavailable
// or the command fails, signaling the caller to fall back to doublestar. ok
// true with empty matches means a confirmed no-match.
func rgGlobFiles(ctx context.Context, pattern, root string, limit int) (matches []string, ok bool) {
	name := rgBinary()
	if name == "" {
		return nil, false
	}
	args := []string{"--files", "--null", "--hidden"}
	for _, g := range vcsExcludeGlobs {
		args = append(args, "--glob="+g)
	}
	args = append(args, "--glob="+rgGlobPattern(pattern))

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, false
	}

	// Stream reads with NUL separation to avoid buffering the full --files
	// output on large trees (e.g. $HOME). The candidate pool is bounded to
	// keep memory small.
	pool := limit*20 + 1000
	reader := bufio.NewReader(stdout)
	for {
		path, rerr := reader.ReadString(0)
		if len(path) > 0 {
			if p := strings.TrimRight(path, "\x00"); p != "" {
				matches = append(matches, filepath.ToSlash(p))
			}
		}
		if rerr != nil {
			break
		}
		if len(matches) >= pool {
			break
		}
	}
	_ = stdout.Close()
	waitErr := cmd.Wait()
	if waitErr != nil && len(matches) == 0 {
		// Exit code 1 is ripgrep's "no matches"; other non-zero exits are
		// treated as failure and fall back to doublestar.
		if ee, ok := waitErr.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, true
		}
		return nil, false
	}
	return matches, true
}

// rgGrepMatch is a single match extracted from ripgrep JSON output.
type rgGrepMatch struct {
	rel  string
	line int
	text string
}

// rgGrepSearch runs `rg --json` to search pattern and returns root-relative
// matches. An empty include disables file-type filtering. ok is false when rg
// is unavailable or fails; true with empty matches means a confirmed no-match.
func rgGrepSearch(ctx context.Context, pattern, root, include string, limit int) (matches []rgGrepMatch, ok bool) {
	name := rgBinary()
	if name == "" {
		return nil, false
	}
	args := []string{"--json", "-n", "--hidden"}
	for _, g := range vcsExcludeGlobs {
		args = append(args, "--glob="+g)
	}
	if include != "" {
		// include is not root-anchored: ripgrep's --glob without a leading
		// slash matches by basename at any level, matching Atlas's include
		// semantics.
		args = append(args, "--glob="+include)
	}
	args = append(args, "--regexp="+pattern, ".")

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		// Exit code 1 is ripgrep's "no matches"; other errors fall back to
		// Go regexp.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, true
		}
		return nil, false
	}

	pool := limit*2 + 100
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var m struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
			} `json:"data"`
		}
		if json.Unmarshal(line, &m) != nil || m.Type != "match" {
			continue
		}
		text := strings.TrimRight(m.Data.Lines.Text, "\n")
		text = strings.TrimRight(text, "\r")
		matches = append(matches, rgGrepMatch{
			rel:  filepath.ToSlash(m.Data.Path.Text),
			line: m.Data.LineNumber,
			text: text,
		})
		if limit > 0 && len(matches) >= pool {
			break
		}
	}
	return matches, true
}
