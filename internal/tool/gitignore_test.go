package tool

import "testing"

func TestGitIgnoreCommentsEscapesAndNegation(t *testing.T) {
	ignorer := mustCompileGitIgnoreLines(t, []string{
		"# comment",
		`ignored.txt`,
		`!important.txt`,
		`\#literal`,
		`\!literal`,
	})

	assertGitIgnored(t, ignorer, "ignored.txt", false)
	assertGitNotIgnored(t, ignorer, "important.txt", false)
	assertGitIgnored(t, ignorer, "#literal", false)
	assertGitIgnored(t, ignorer, "!literal", false)
}

func TestGitIgnoreDirectoryPatternMatchesChildren(t *testing.T) {
	ignorer := mustCompileGitIgnoreLines(t, []string{"build/"})

	assertGitIgnored(t, ignorer, "build", true)
	assertGitIgnored(t, ignorer, "build/artifact.txt", false)
	assertGitNotIgnored(t, ignorer, "build", false)
	assertGitIgnored(t, ignorer, "nested/build/file.txt", false)
}

func TestGitIgnoreRootAnchoredPattern(t *testing.T) {
	ignorer := mustCompileGitIgnoreLines(t, []string{"/*.log"})

	assertGitIgnored(t, ignorer, "atlas.log", false)
	assertGitNotIgnored(t, ignorer, "logs/atlas.log", false)
}

func TestGitIgnoreSlashPatternIsRootRelative(t *testing.T) {
	ignorer := mustCompileGitIgnoreLines(t, []string{"Documentation/*.html"})

	assertGitIgnored(t, ignorer, "Documentation/git.html", false)
	assertGitNotIgnored(t, ignorer, "Documentation/ppc/ppc.html", false)
	assertGitNotIgnored(t, ignorer, "tools/perf/Documentation/perf.html", false)
}

func TestGitIgnoreDoubleStarPattern(t *testing.T) {
	ignorer := mustCompileGitIgnoreLines(t, []string{"**/target", "a/**/b"})

	assertGitIgnored(t, ignorer, "target", false)
	assertGitIgnored(t, ignorer, "cmd/target", false)
	assertGitIgnored(t, ignorer, "a/b", false)
	assertGitIgnored(t, ignorer, "a/x/y/b", false)
	assertGitNotIgnored(t, ignorer, "a/x/y/c", false)
}

func TestGitIgnoreTrailingDoubleStarKeepsDirectory(t *testing.T) {
	ignorer := mustCompileGitIgnoreLines(t, []string{"foo/**"})

	assertGitNotIgnored(t, ignorer, "foo", true)
	assertGitIgnored(t, ignorer, "foo/a.txt", false)
	assertGitIgnored(t, ignorer, "foo/bar/baz.txt", false)
}

func TestGitIgnoreTrailingSpaces(t *testing.T) {
	ignorer := mustCompileGitIgnoreLines(t, []string{`name   `, `literal\ `})

	assertGitIgnored(t, ignorer, "name", false)
	assertGitNotIgnored(t, ignorer, "name   ", false)
	assertGitIgnored(t, ignorer, "literal ", false)
}

func mustCompileGitIgnoreLines(t *testing.T, lines []string) *gitIgnore {
	t.Helper()

	ignorer, err := compileGitIgnoreLines(lines)
	if err != nil {
		t.Fatalf("compileGitIgnoreLines() error = %v", err)
	}
	return ignorer
}

func assertGitIgnored(t *testing.T, ignorer *gitIgnore, rel string, isDir bool) {
	t.Helper()

	if !isGitIgnored(ignorer, rel, isDir) {
		t.Fatalf("isGitIgnored(%q, %v) = false, want true", rel, isDir)
	}
}

func assertGitNotIgnored(t *testing.T, ignorer *gitIgnore, rel string, isDir bool) {
	t.Helper()

	if isGitIgnored(ignorer, rel, isDir) {
		t.Fatalf("isGitIgnored(%q, %v) = true, want false", rel, isDir)
	}
}
