# Atlas

Atlas 是一个本地 coding agent。它通过 OpenAI-compatible Chat Completions Provider 流式调用模型，在 headless agent loop 中维护 transcript、执行工具，并把工具结果继续交给模型，直到得到最终回复或达到最大步数。

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
  },
  "session": {
    "db_path": "~/.atlas/atlas.db"
  }
}
```

`provider.model` 属于 Provider 连接配置，不属于单次 `model.ChatRequest`。`agent.max_steps` 限制一次 turn 中最多执行多少轮模型调用。`session.db_path` 可省略，默认使用 `~/.atlas/atlas.db`。

## 使用

```sh
go run ./cmd/atlas
go run ./cmd/atlas --session 20260608-153012-a1b2c3d4
go run ./cmd/atlas run "读取 README 并总结"
go run ./cmd/atlas run --session 20260608-153012-a1b2c3d4 "继续刚才的问题"
go run ./cmd/atlas sessions
go run ./cmd/atlas session show 20260608-153012-a1b2c3d4
go run ./cmd/atlas session delete 20260608-153012-a1b2c3d4
go run ./cmd/atlas acp
go run ./cmd/atlas version
```

裸 `atlas` 是交互模式入口；当前版本暂未实现 TUI，会提示使用 `atlas run`。`atlas run` 会创建一个 agent，读取配置和指令文件，注册内置工具，并执行一次 `Agent.RunTurn`。模型文本增量会实时输出。

当前版本是 `0.0.1`。CLI 可通过 `atlas version`、`atlas --version` 或 `atlas -v` 查看版本；ACP 会在 initialize 响应的 `AgentInfo.Version` 中报告同一版本。

`atlas run` 默认创建新的 session ID 并保存本轮 transcript。传入 `--session <id>` 时，Atlas 会恢复这个 session；如果它不存在，则使用该 ID 创建新 session。session ID 只允许字母、数字、`.`、`_` 和 `-`。

`atlas acp` 通过 stdin/stdout 启动 Agent Client Protocol 服务，供支持 ACP 的编辑器或客户端连接。当前支持初始化、创建 session、发送 prompt、取消、关闭、恢复、列出和删除本地 session；不支持 ACP auth、权限请求、MCP 连接、图片/音频/嵌入资源输入和历史消息回放。

Atlas 使用 SQLite 保存本地会话。当前支持按 ID 恢复、列出最近会话、查看会话详情和删除会话；不提供全文搜索。

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

核心实现位于 `internal/agent`、`internal/acp`、`internal/runtime`、`internal/model`、`internal/provider/openai`、`internal/tool`、`internal/prompt`、`internal/config`、`internal/session` 和 `internal/transcript`。CLI 位于 `cmd/atlas`。
