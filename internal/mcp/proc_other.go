//go:build !darwin && !linux

package mcp

import "os/exec"

// setGroup 与 killGroup 在没有进程组的平台上什么也不做：包装器启动的孙进程可能残留。
func setGroup(*exec.Cmd) {}

func killGroup(*exec.Cmd) {}
