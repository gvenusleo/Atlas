//go:build darwin || linux

package tools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const shellDescription = "执行 Bash 命令。"

// shellCommand 启动独立 Bash 进程组，取消时终止仍在组内的全部进程。
func shellCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	// 进程组终止失败时，仍限制取消后的等待时间并终止直接子进程。
	cmd.WaitDelay = time.Second
	return cmd, nil
}
