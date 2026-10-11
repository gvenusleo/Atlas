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
	"unicode/utf8"

	"github.com/gvenusleo/atlas/internal/agent"
)

// 定义 shell 执行限制。
const (
	shellMaxOutputBytes    = 50 << 10
	shellMaxOutputLines    = 2000
	shellMaxTimeoutSeconds = 30 * 60
)

// shellArgs 保留 timeout 的原始值，以区分省略参数与显式 null。
type shellArgs struct {
	Command string         `json:"command"`
	Timeout jsontext.Value `json:"timeout"`
}

// Shell 构造 shell 工具；workdir 应为工作目录绝对路径，每次调用启动独立 shell。
func Shell(workdir string) agent.Tool {
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
		Execute: func(ctx context.Context, _ string, params jsontext.Value) (agent.ToolResult, error) {
			return runShell(ctx, workdir, params)
		},
	}
}

// runShell 将输出直接写入临时文件；执行失败仍保留已产生的输出。
func runShell(ctx context.Context, workdir string, params jsontext.Value) (agent.ToolResult, error) {
	var args shellArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return agent.ToolResult{}, fmt.Errorf("参数无效: %w", err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return agent.ToolResult{}, errors.New("参数无效: command 不能为空")
	}
	if strings.ContainsRune(args.Command, '\x00') {
		return agent.ToolResult{}, errors.New("参数无效: command 不能包含 NUL")
	}

	var timeout int64
	if len(args.Timeout) > 0 {
		if err := json.Unmarshal(args.Timeout, &timeout); err != nil {
			return agent.ToolResult{}, fmt.Errorf("参数无效: timeout 必须是正整数秒: %w", err)
		}
		if timeout < 1 || timeout > shellMaxTimeoutSeconds {
			return agent.ToolResult{}, fmt.Errorf("参数无效: timeout 必须在 1–%d 秒之间", shellMaxTimeoutSeconds)
		}
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

	cmd, err := shellCommand(runCtx, args.Command)
	if err != nil {
		return agent.ToolResult{}, err
	}
	cmd.Dir = workdir

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
		var status string
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			status = "命令已取消"
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			status = "调用上下文已超时"
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			status = fmt.Sprintf("命令超过 %d 秒，已终止", timeout)
		default:
			if exitErr, ok := errors.AsType[*exec.ExitError](runErr); ok && exitErr.ExitCode() >= 0 {
				status = fmt.Sprintf("命令退出码: %d", exitErr.ExitCode())
			} else {
				status = fmt.Sprintf("命令执行失败: %s", runErr)
			}
		}
		text += "\n\n[" + status + "]"
	}

	return agent.ToolResult{
		Content: []agent.Content{agent.TextContent{Text: text}},
		IsError: runErr != nil,
	}, nil
}

// shellOutput 只读取日志尾部；通常保留完整行，超长末行则显示该行尾部。
func shellOutput(file *os.File) (string, bool, error) {
	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	// 多读一个字节，判断返回片段是否从行中间开始。
	offset := max(int64(0), info.Size()-shellMaxOutputBytes-1)
	data := make([]byte, info.Size()-offset)
	if len(data) > 0 {
		if _, err := file.ReadAt(data, offset); err != nil {
			return "", false, err
		}
	}

	start := 0
	truncated := len(data) > shellMaxOutputBytes
	partial := false
	if truncated {
		start = 1
		if data[start-1] != '\n' {
			if newline := bytes.IndexByte(data[start:], '\n'); newline >= 0 && start+newline+1 < len(data) {
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
	if partial {
		// 截断点可能落在 UTF-8 字符内部，跳过其剩余字节。
		for start < len(data) && data[start]&0xc0 == 0x80 {
			start++
		}
	}
	data = data[start:]
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
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
