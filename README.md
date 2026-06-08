# Atlas

Atlas 是一个本地 coding agent。它通过 OpenAI-compatible Chat Completions Provider 调用模型，在 headless agent loop 中维护 transcript、执行工具，并把工具结果继续交给模型，直到得到最终回复或达到最大步数。

## 行为边界

Atlas 以当前进程的本地权限运行。内置工具可以读取文件、写入文件、搜索文本并执行 shell 命令；Atlas 不提供沙箱、权限提示或 approval gate。请只在可信工作区中运行 Atlas。

Atlas 从两个位置加载附加指令：

- `~/.atlas/AGENTS.md`
- 当前工作目录下的 `AGENTS.md`

Atlas 不递归查找父目录或子目录中的 `AGENTS.md`。当前用户输入优先于指令文件，当前目录指令优先于全局指令。

## 配置

Atlas 从用户主目录下的 `.atlas/config.json` 读取应用配置：

```json
{
  "provider": {
    "base_url": "https://api.deepseek.com",
    "api_key": "sk-...",
    "model": "deepseek-v4-flash"
  },
  "agent": {
    "max_steps": 8,
    "temperature": 0.2
  }
}
```

`provider.model` 属于 Provider 连接配置，不属于单次 `model.ChatRequest`。`agent.max_steps` 限制一次 turn 中最多执行多少轮模型调用。

## 使用

```sh
go run ./cmd/atlas "读取 README 并总结"
```

CLI 会为每次调用创建一个 agent，读取配置和指令文件，注册内置工具，并执行一次 `Agent.RunTurn`。

## 内置工具

- `list_files`：列出目录中的文件。
- `read_file`：读取文本文件。
- `search_text`：在目录中按字面量搜索文本。
- `write_file`：写入文件内容。
- `run_shell`：执行 shell 命令。

## 开发

```sh
go test ./...
```

核心实现位于 `internal/agent`、`internal/model`、`internal/provider/openai`、`internal/tool`、`internal/prompt`、`internal/config` 和 `internal/transcript`。CLI 位于 `cmd/atlas`。
