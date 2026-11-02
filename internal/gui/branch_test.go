package gui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gvenusleo/atlas/internal/protocol"
)

func testGit(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	base := []string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false",
		"-c", "user.name=Atlas Test", "-c", "user.email=test@example.invalid", "-C", cwd}
	out, err := exec.CommandContext(t.Context(), "git", append(base, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func testRepo(t *testing.T) string {
	t.Helper()
	cwd := t.TempDir()
	testGit(t, cwd, "init", "-b", "main")
	testGit(t, cwd, "commit", "--allow-empty", "-m", "初始提交")
	return cwd
}

// 同一工作树的子目录、符号链接共享运行保护，独立 worktree 不受影响。
func TestBranchBusyAcrossDirectories(t *testing.T) {
	cwd := testRepo(t)
	sub := filepath.Join(cwd, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(cwd, alias); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), "worktree")
	testGit(t, cwd, "worktree", "add", "-b", "independent", linked)
	for _, target := range []struct {
		cwd  string
		busy bool
	}{{cwd, true}, {sub, true}, {alias, true}, {linked, false}} {
		running := &chat{cwd: sub, running: true, branch: testBranch(t, sub)}
		idle := &chat{cwd: target.cwd, branch: testBranch(t, target.cwd)}
		a := &App{chats: map[string]*chat{"running": running}}
		if got := a.branchBusy(idle); got != target.busy {
			t.Errorf("工作目录 %s：禁止切换=%v，期望 %v", target.cwd, got, target.busy)
		}
	}
}

func testBranch(t *testing.T, cwd string) branchState {
	t.Helper()
	dir, err := gitDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	return branchState{dir: dir}
}

// 同一工作树中的刷新请求必须合并到每个正在读取的会话与面板。
func TestBranchChangedCoalescesWorktreeRefresh(t *testing.T) {
	cwd := testRepo(t)
	sub := filepath.Join(cwd, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	ch := &chat{cwd: cwd, branch: testBranch(t, cwd)}
	other := &chat{cwd: sub, branch: testBranch(t, sub), changes: &changePanel{loading: true}}
	other.branch.loading = true
	other.openPanel(changesPanel)
	a := &App{chats: map[string]*chat{"other": other}}
	a.branchChanged(ch)
	if !other.branch.again || !other.changes.again {
		t.Fatal("同一工作树的分支或面板刷新请求丢失")
	}
}

type closeService struct{ protocol.Service }

func (closeService) CloseSession(context.Context, protocol.CloseSessionRequest) error { return nil }

// 任务失败与会话丢弃必须同时取消监视、使异步分支结果失效。
func TestFailedTaskDiscardsBranch(t *testing.T) {
	for _, created := range []bool{false, true} {
		ch := &chat{starts: 3, job: &taskRun{}}
		if created {
			ch.id = "task-session"
		}
		stopped := false
		ch.unwatchHead = func() { stopped = true }
		a := &App{chats: map[string]*chat{ch.id: ch}, service: closeService{}}
		a.tasks.errs = map[string]string{}
		a.failTask(ch, errors.New("任务失败"))
		if !stopped || ch.unwatchHead != nil || ch.starts <= 3 {
			t.Fatalf("已建立会话=%v：监视未停止或在途结果未失效", created)
		}
		if created && a.chats[ch.id] != nil {
			t.Fatal("失败的任务会话仍在已打开列表中")
		}
	}
}
