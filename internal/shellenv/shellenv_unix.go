//go:build darwin || linux

package shellenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// marker 夹在环境两头：交互 shell 的配置文件常会输出横幅之类的内容，要把它们分开。
const marker = "__ATLAS_ENV__"

// transient 是登录 shell 自己造出来的变量，不属于用户的环境。
var transient = map[string]bool{"_": true, "SHLVL": true, "PWD": true, "OLDPWD": true}

// load 让用户的登录 shell 以交互方式执行 env 并读出结果。要带 -i：mise、nvm 这类工具
// 多半在只有交互 shell 才读的 .zshrc 或 .bashrc 里激活。超时或取消时终止整个进程组。
func load(ctx context.Context) ([]string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		}
	}
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c",
		"printf %s "+marker+"; env -0; printf %s "+marker)
	// 新会话没有控制终端：从终端启动时，处在后台进程组的交互 shell 去碰终端会被停住，直到超时。
	// 它同时是自己进程组的组长，下面可以按组终止。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", shell, err)
	}
	return parse(out)
}

// parse 取出两个标记之间的环境：以 NUL 分隔的 NAME=值，去掉登录 shell 自己造的变量。
func parse(out []byte) ([]string, error) {
	_, rest, opened := bytes.Cut(out, []byte(marker))
	body, _, closed := bytes.Cut(rest, []byte(marker))
	if !opened || !closed {
		return nil, errors.New("输出里没有找到环境")
	}
	var env []string
	for kv := range strings.SplitSeq(string(body), "\x00") {
		if key, _, ok := strings.Cut(kv, "="); ok && key != "" && !transient[key] {
			env = append(env, kv)
		}
	}
	return env, nil
}
