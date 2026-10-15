package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const shellDescription = "执行 PowerShell 命令，优先使用 PowerShell 7，否则使用 Windows PowerShell。" +
	"使用兼容 Windows PowerShell 5.1 的语法；用条件语句连接依赖命令，避免使用 && 和 ||。"

// shellCommand 选择 PowerShell 并设置 UTF-8 输出，取消时使用 taskkill 终止进程树。
func shellCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	shell, err := exec.LookPath("pwsh.exe")
	if errors.Is(err, exec.ErrNotFound) {
		shell, err = exec.LookPath("powershell.exe")
	}
	if err != nil {
		return nil, fmt.Errorf("查找 PowerShell 失败: %w", err)
	}

	// 同时设置控制台输出和传给原生命令的管道输入编码，兼容 PowerShell 5.1。
	command = "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n" +
		"$OutputEncoding = [Console]::OutputEncoding\n" + command
	args := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Cancel = func() error {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		// 原命令的 context 已取消，清理进程树需要独立且有界的 context。
		killCtx, cancel := context.WithTimeout(context.Background(), shellKillTimeout)
		defer cancel()
		killer := exec.CommandContext(killCtx, filepath.Join(root, "System32", "taskkill.exe"),
			"/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		killer.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := killer.Run(); err != nil {
			return fmt.Errorf("终止命令进程树失败: %w", err)
		}
		return nil
	}
	// taskkill 失败时，仍限制取消后的等待时间并终止直接子进程。
	cmd.WaitDelay = shellWaitDelay
	return cmd, nil
}
