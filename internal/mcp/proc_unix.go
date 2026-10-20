//go:build darwin || linux

package mcp

import (
	"os/exec"
	"syscall"
)

// setGroup 让子进程自成一个进程组，这样 npx、uvx 这类包装器启动的孙进程可以一起终止。
func setGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup 终止进程组里还在的进程；cmd 为空或还没启动时什么也不做。
func killGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
