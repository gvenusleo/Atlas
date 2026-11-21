package gui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gvenusleo/atlas/internal/protocol"
)

func testWorktreeRemoval(t *testing.T) (worktreeRecovery, worktreeEntry) {
	t.Helper()
	cwd := testRepo(t)
	if err := os.WriteFile(filepath.Join(cwd, ".gitignore"), []byte(".env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testGit(t, cwd, "add", ".gitignore")
	testGit(t, cwd, "commit", "-m", "忽略本机设置")
	list, err := readWorktrees(t.Context(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "工作树 with space")
	target, err := createAtlasWorktree(t.Context(), cwd, dir, list.common, "demo", "atlas/demo")
	if err != nil {
		t.Fatal(err)
	}
	list, err = readWorktrees(t.Context(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.entries) != 2 || list.entries[1].cwd != target {
		t.Fatalf("工作树清单错误：%+v", list)
	}
	entry := list.entries[1]
	return worktreeRecovery{Root: target, Main: list.main, Common: list.common,
		Branch: entry.branch, Head: entry.head}, entry
}

func TestWorktreeRemovalPreservesFilesAndRecovery(t *testing.T) {
	record, entry := testWorktreeRemoval(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(record.Root, alias); err != nil {
		t.Fatal(err)
	}
	dir, kept := t.TempDir(), filepath.Join(t.TempDir(), "废纸篓")
	refs := []protocol.SessionInfo{{SessionID: "old", Cwd: alias}}
	// 最后一次检查之后才出现的本地文件也必须随检出一起保留。
	trash := func(staged string) error {
		if err := os.WriteFile(filepath.Join(staged, ".env"), []byte("local"), 0o600); err != nil {
			return err
		}
		return os.Rename(staged, kept)
	}
	saved, err := persistAndTrashWorktree(t.Context(), record.Main, dir, record, entry, refs,
		trash)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(kept, ".env"))
	if err != nil || string(data) != "local" {
		t.Fatalf("移除后未保留本地文件：%v", err)
	}
	loaded, err := loadWorktreeRecoveries(dir)
	if err != nil || loaded["old"] != saved || loaded["new"].Root != "" {
		t.Fatalf("恢复记录未限定到原会话：%+v，%v", loaded, err)
	}
	assertWorktreePathReuse(t, saved, loaded, alias)
	if err := restoreWorktree(t.Context(), saved, alias); err != nil {
		t.Fatal(err)
	}
	if head := testGit(t, alias, "rev-parse", "HEAD"); head != record.Head {
		t.Fatalf("恢复到错误的提交：%s", head)
	}
	if err := forgetWorktreeRecovery(dir, saved); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadWorktreeRecoveries(dir); err != nil || len(loaded) != 0 {
		t.Fatalf("恢复完成后仍有待恢复的会话：%+v，%v", loaded, err)
	}
}

func assertWorktreePathReuse(
	t *testing.T, record worktreeRecovery, loaded map[string]worktreeRecovery, alias string,
) {
	t.Helper()
	newRepo := testRepo(t)
	newList, err := readWorktrees(t.Context(), newRepo)
	if err != nil {
		t.Fatal(err)
	}
	newRoot, err := createAtlasWorktree(t.Context(), newRepo, filepath.Dir(record.Root),
		newList.common, filepath.Base(record.Root), "new")
	if err != nil || newRoot != record.Root {
		t.Fatalf("同名工作树创建失败：%s，%v", newRoot, err)
	}
	if err := requireWorktreeRecovery(loaded["new"], newRoot); err != nil {
		t.Fatalf("旧记录阻止了新会话：%v", err)
	}
	if err := restoreWorktree(t.Context(), record, alias); err == nil {
		t.Fatal("恢复接受了同一路径的其他仓库")
	}
	testGit(t, newRepo, "worktree", "remove", "--", newRoot)
	testGit(t, record.Main, "worktree", "add", "-b", "other", "--", record.Root, "HEAD")
	if err := restoreWorktree(t.Context(), record, alias); err == nil {
		t.Fatal("恢复接受了同一路径的其他分支")
	}
	testGit(t, record.Main, "worktree", "remove", "--", record.Root)
}

func TestWorktreeRemovalStopsOrRollsBackOnFailure(t *testing.T) {
	record, entry := testWorktreeRemoval(t)
	refs := []protocol.SessionInfo{{SessionID: "old", Cwd: record.Root}}
	trash := func(string) error { t.Fatal("检出目录不应移入废纸篓"); return nil }
	ignored := filepath.Join(record.Root, ".env")
	if err := os.WriteFile(ignored, []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := persistAndTrashWorktree(t.Context(), record.Main, dir, record, entry, refs,
		trash); err == nil {
		t.Fatal("含忽略文件的工作树通过移除检查")
	}
	if err := os.Remove(ignored); err != nil {
		t.Fatal(err)
	}
	testGit(t, record.Root, "update-index", "--assume-unchanged", ".gitignore")
	if _, err := persistAndTrashWorktree(t.Context(), record.Main, dir, record, entry, refs,
		trash); err == nil {
		t.Fatal("无法检查本地改动的工作树通过移除检查")
	}
	testGit(t, record.Root, "update-index", "--no-assume-unchanged", ".gitignore")
	badDir := t.TempDir()
	badFile := filepath.Join(badDir, worktreeRecoveryFile)
	if err := os.WriteFile(badFile, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := persistAndTrashWorktree(t.Context(), record.Main, badDir, record, entry, refs,
		trash); err == nil {
		t.Fatal("恢复记录无法写入时仍继续移除")
	}
	trashErr := errors.New("废纸篓不可用")
	saved, err := persistAndTrashWorktree(t.Context(), record.Main, dir, record, entry, refs,
		func(string) error { return trashErr })
	if saved.Root != "" || !errors.Is(err, trashErr) {
		t.Fatalf("移入废纸篓失败后未回退：%+v，%v", saved, err)
	}
	if err := checkRestoredWorktree(t.Context(), record, record.Root); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadWorktreeRecoveries(dir); err != nil || len(loaded) != 0 {
		t.Fatalf("回退后仍有待恢复的会话：%+v，%v", loaded, err)
	}
}

func TestWorktreeCreateResultAfterSessionChange(t *testing.T) {
	original := &chat{cwd: "/repo", starts: 1}
	current := &chat{cwd: "/other"}
	d := &worktreePicker{ch: original, starts: 1, cwd: original.cwd, createOpen: true}
	a := &App{current: current, worktrees: d, creatingWorktree: true}
	a.finishWorktreeCreate(d, "/worktree", true, nil)
	if a.current != current || original.cwd != "/repo" || a.creatingWorktree ||
		!strings.Contains(a.toast, "/worktree") {
		t.Fatalf("过期结果改变了会话或未报告结果，提示为 %q", a.toast)
	}
}

func TestWorktreeRemovalGuardsInFlightOperations(t *testing.T) {
	root := resolveWorktreePath(t.TempDir())
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	ch := &chat{cwd: alias, loading: true}
	a := &App{chats: map[string]*chat{"active": ch}}
	if a.worktreeUseReason(root) == "" {
		t.Fatal("未拦截正在打开的会话")
	}
	ch.loading, ch.run.Running = false, true
	if a.worktreeUseReason(root) == "" {
		t.Fatal("未拦截从符号链接路径启动的会话")
	}
	clear(a.chats)
	file := filepath.Join(alias, "new.txt")
	if !a.beginFileWrite(file) || a.worktreeUseReason(root) == "" {
		t.Fatal("未拦截已有的文件写入")
	}
	a.endFileWrite(file)
	a.mutatingWorktree = root
	if a.beginFileWrite(file) || a.newChatAt(alias) {
		t.Fatal("移除期间接受了新的文件写入或会话")
	}
}
