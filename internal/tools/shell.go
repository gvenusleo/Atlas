package tools

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/shellenv"
)

// 定义 shell 执行限制。
const (
	shellMaxOutputBytes    = 50 << 10
	shellMaxOutputLines    = 2000
	shellMaxTimeoutSeconds = 30 * 60
	// shellKillTimeout 限制清理进程树的耗时；shellWaitDelay 限制取消后等待输出收尾的时间。
	shellKillTimeout = 5 * time.Second
	shellWaitDelay   = time.Second
)

// shellArgs 保留 timeout 的原始值，以区分省略参数与显式 null。
type shellArgs struct {
	Command string         `json:"command"`
	Timeout jsontext.Value `json:"timeout"`
}

// Shell 构造 shell 工具；cwd 应为工作目录绝对路径，每次调用启动独立 shell。
func Shell(cwd string) agent.Tool {
	return agent.Tool{
		Name: "shell",
		Description: shellDescription +
			"在工作目录执行前台命令，继承当前环境，不支持交互式输入。" +
			"每次启动独立 shell，目录和环境变量更改不跨调用保留。" +
			"合并返回 stdout 和 stderr，最多显示末尾 2000 行或 50 KiB，先达到的限制生效。" +
			"输出截断时提供完整日志路径。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"minLength":   1,
					"description": "要执行的命令，语法须与当前 shell 一致，支持管道和重定向。",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     shellMaxTimeoutSeconds,
					"description": "超时秒数；省略时不设超时。",
				},
			},
			"required": []string{"command"},
		},
		Execute: func(
			ctx context.Context, _ string, params jsontext.Value,
		) (agent.ToolResult, error) {
			return runShell(ctx, cwd, params)
		},
	}
}

// parseShellArgs 校验命令与可选超时，返回命令文本和超时秒数（0 表示不设超时）。
func parseShellArgs(params jsontext.Value) (string, int64, error) {
	var args shellArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return "", 0, fmt.Errorf("参数无效: %w", err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return "", 0, errors.New("参数无效: command 不能为空")
	}
	if strings.ContainsRune(args.Command, '\x00') {
		return "", 0, errors.New("参数无效: command 不能包含 NUL")
	}
	var timeout int64
	if len(args.Timeout) > 0 {
		if err := json.Unmarshal(args.Timeout, &timeout); err != nil {
			return "", 0, fmt.Errorf("参数无效: timeout 必须是正整数秒: %w", err)
		}
		if timeout < 1 || timeout > shellMaxTimeoutSeconds {
			return "", 0, fmt.Errorf("参数无效: timeout 必须在 1–%d 秒之间", shellMaxTimeoutSeconds)
		}
	}
	return args.Command, timeout, nil
}

// runShell 将输出直接写入临时文件；执行失败仍保留已产生的输出。
func runShell(ctx context.Context, cwd string, params jsontext.Value) (agent.ToolResult, error) {
	command, timeout, err := parseShellArgs(params)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	cmd, err := shellCommand(runCtx, command)
	if err != nil {
		return agent.ToolResult{}, err
	}
	cmd.Dir, cmd.Env = cwd, shellenv.Environ()

	output, err := os.CreateTemp("", "atlas-shell-*.log")
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("创建命令日志失败: %w", err)
	}
	keepOutput := false
	defer func() {
		_ = output.Close()
		if !keepOutput {
			_ = os.Remove(output.Name())
		}
	}()

	// 直接连接文件，不使用管道，避免后代进程持有输出管道时阻塞 Wait。
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return agent.ToolResult{}, fmt.Errorf("命令启动失败: %w", err)
	}
	runErr := cmd.Wait()

	text, truncated, err := shellOutput(output)
	if err != nil {
		keepOutput = true
		return agent.ToolResult{}, fmt.Errorf("读取命令输出失败，原始日志: %s: %w", output.Name(), err)
	}
	keepOutput = truncated
	if runErr != nil {
		text += "\n\n[" + shellStatus(ctx, runCtx, runErr, timeout) + "]"
	}
	return agent.ToolResult{
		Content: []agent.Content{agent.TextContent{Text: text}},
		IsError: runErr != nil,
	}, nil
}

// shellStatus 说明命令失败的原因，ctx 是调用方上下文，runCtx 带有超时。
func shellStatus(ctx, runCtx context.Context, runErr error, timeout int64) string {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return "命令已取消"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "调用上下文已超时"
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return fmt.Sprintf("命令超过 %d 秒，已终止", timeout)
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](runErr); ok && exitErr.ExitCode() >= 0 {
		return fmt.Sprintf("命令退出码: %d", exitErr.ExitCode())
	}
	return fmt.Sprintf("命令执行失败: %s", runErr)
}

// shellOutput 只读取日志尾部；通常保留完整行，超长末行则显示该行尾部。
func shellOutput(file *os.File) (string, bool, error) {
	data, err := readTail(file)
	if err != nil {
		return "", false, err
	}
	start, truncated, partial := tailWindow(data)
	if partial {
		// 截断点可能落在 UTF-8 字符内部，跳过其剩余字节。
		for start < len(data) && data[start]&0xc0 == 0x80 {
			start++
		}
	}
	data = data[start:]
	if !validText(data) {
		return "", truncated, errors.New("输出不是支持的 UTF-8 文本")
	}

	text := string(data)
	if text == "" {
		text = "无输出"
	}
	if truncated {
		note := "输出已截断，仅显示末尾最多 2000 行或 50 KiB。"
		if partial {
			note = "末行超过 50 KiB，仅显示该行尾部。"
		}
		text += fmt.Sprintf("\n\n[%s 完整输出: %s]", note, file.Name())
	}
	return text, truncated, nil
}

// readTail 读取日志末尾；多读一个字节，用于判断片段是否从行中间开始。
func readTail(file *os.File) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	offset := max(int64(0), info.Size()-shellMaxOutputBytes-1)
	data := make([]byte, info.Size()-offset)
	if len(data) > 0 {
		if _, err := file.ReadAt(data, offset); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// tailWindow 返回保留片段的起点；truncated 表示内容被截断，partial 表示起点落在行中间。
func tailWindow(data []byte) (start int, truncated, partial bool) {
	truncated = len(data) > shellMaxOutputBytes
	if truncated {
		start = 1
		if data[0] != '\n' {
			newline := bytes.IndexByte(data[start:], '\n')
			if newline >= 0 && start+newline+1 < len(data) {
				start += newline + 1
			} else {
				partial = true
			}
		}
	}

	end := len(data)
	if end > start && data[end-1] == '\n' {
		end-- // 末尾换行不额外计为一行
	}
	cursor := end
	for lines := range shellMaxOutputLines {
		newline := bytes.LastIndexByte(data[start:cursor], '\n')
		if newline < 0 {
			break
		}
		cursor = start + newline
		if lines == shellMaxOutputLines-1 {
			start = cursor + 1
			truncated = true
			partial = false
		}
	}
	return start, truncated, partial
}
