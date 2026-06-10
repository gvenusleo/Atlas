# Atlas

Atlas 是一个极简本地 coding agent，支持一次性 CLI 调用和 ACP stdio 接入。它通过 OpenAI-compatible Chat Completions Provider 调用模型，并在本地会话中保存对话记录。

## 行为边界

Atlas 以当前进程的本地权限运行。内置工具可以读取文件、写入文件、搜索文本并执行 shell 命令；Atlas 不提供沙箱、权限提示或 approval gate。请只在可信工作区中运行 Atlas。

Atlas 从两个位置加载附加 `AGENTS.md` 指令：

- `~/.atlas/AGENTS.md`
- 当前工作目录下的 `AGENTS.md`

Atlas 不递归查找父目录或子目录中的 `AGENTS.md`。当前用户输入优先于指令文件，当前目录指令优先于全局指令。

Atlas 还会扫描用户级和当前目录级 skill，只把 `name` 和 `description` 摘要放入系统提示词；模型需要完整指令时，通过 `load_skill` 按名称读取对应 `SKILL.md`。

## 配置

Atlas 从用户主目录下的 `.atlas/config.json` 读取应用配置：

```json
{
  "provider": {
    "base_url": "https://api.deepseek.com",
    "api_key": "sk-...",
    "default_model": "deepseek-v4-flash",
    "models": [
      {
        "value": "deepseek-v4-flash",
        "name": "DeepSeek V4 Flash",
        "context_window": 1000000,
        "max_tokens": 384000
      },
      {
        "value": "deepseek-v4-pro",
        "name": "DeepSeek V4 Pro",
        "context_window": 1000000,
        "max_tokens": 384000
      }
    ]
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

`provider.default_model` 必须匹配 `provider.models` 中某个模型的 `value`。`name` 用于界面显示，`value` 是实际发送给 Provider 的模型名。`context_window` 描述模型上下文窗口，`max_tokens` 会作为每次模型请求的最大输出 token 数发送给 Provider。`agent.max_steps` 限制单次请求最多执行多少轮模型调用。`session.db_path` 可省略，默认使用 `~/.atlas/atlas.db`。

## 使用

```sh
go run ./cmd/atlas
go run ./cmd/atlas --session 20260608-153012-a1b2c3d4
go run ./cmd/atlas run "读取 README 并总结"
go run ./cmd/atlas run --model deepseek-v4-pro "用 Pro 模型分析这个问题"
go run ./cmd/atlas run --session 20260608-153012-a1b2c3d4 "继续刚才的问题"
go run ./cmd/atlas sessions
go run ./cmd/atlas session show 20260608-153012-a1b2c3d4
go run ./cmd/atlas session delete 20260608-153012-a1b2c3d4
go run ./cmd/atlas acp
go run ./cmd/atlas version
```

裸 `atlas` 是交互模式入口；当前版本暂未实现 TUI，会提示使用 `atlas run`。`atlas run` 执行一次模型请求，并实时输出模型文本。

`atlas run` 默认创建新的 session ID 并保存本轮 transcript。传入 `--session <id>` 时，Atlas 会恢复这个 session；如果它不存在，则使用该 ID 创建新 session。传入 `--model <value>` 时，本轮使用该模型。session ID 只允许字母、数字、`.`、`_` 和 `-`。

`atlas acp` 通过 stdin/stdout 启动 Agent Client Protocol 服务，供支持 ACP 的编辑器或客户端连接。当前支持 session 创建、prompt、取消、关闭、恢复、列表、删除和模型切换；不支持 ACP auth、权限请求、MCP 连接、多模态输入和历史消息回放。

Atlas 使用 SQLite 保存本地会话。当前支持按 ID 恢复、列出最近会话、查看会话详情和删除会话；不提供全文搜索。

## 内置工具

- `list_files`：列出目录中的文件。
- `read_file`：读取文本文件。
- `search_text`：在目录中按字面量搜索文本。
- `write_file`：写入文件内容。
- `run_shell`：执行 shell 命令。
- `load_skill`：按名称加载本地 skill 指令。
