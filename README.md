# Atlas

Atlas 是一个运行在用户机器上的本地通用 Agent。它通过模型 API 格式适配器调用 Provider，在本地 SQLite 中保存会话，具备文件操作、Shell 执行、网页搜索、长期记忆和上下文压缩能力。

## 权限边界

Atlas 以当前进程的本地权限运行。内置工具可以读写文件、搜索文本并执行 shell 命令；Atlas 不提供沙箱、权限提示或 approval gate。请只在可信工作区中运行。

通过 ACP 连接时，Atlas 会优先使用客户端声明的能力：

- terminal capability：`run_shell` 请求客户端 terminal 执行，并嵌入输出。
- filesystem capability：文件工具请求客户端读写文件，并展示 locations/diff。

客户端不支持或调用失败时，Atlas 回退到本地工具执行。

Atlas 会加载两个附加指令文件：

- `~/.atlas/AGENTS.md`
- 当前工作目录下的 `AGENTS.md`

当前用户输入优先于指令文件，当前目录指令优先于全局指令。Atlas 不递归查找父目录或子目录中的 `AGENTS.md`。

Atlas 也会扫描用户级和当前目录级 skill，只把 `name` 和 `description` 摘要放进系统提示词；需要完整指令时，模型通过 `load_skill` 读取对应 `SKILL.md`。

## 配置

Atlas 从 `~/.atlas/config.json` 读取配置：

```json
{
  "active_provider": "deepseek",
  "providers": [
    {
      "name": "deepseek",
      "format": "chat_completions",
      "base_url": "https://api.deepseek.com",
      "api_key": "sk-...",
      "default_model": "deepseek-v4-flash",
      "models": [
        {
          "value": "deepseek-v4-flash",
          "name": "DeepSeek V4 Flash",
          "context_window": 1000000,
          "max_tokens": 384000,
          "reasoning_efforts": [
            {
              "value": "high",
              "name": "High"
            },
            {
              "value": "max",
              "name": "Max"
            }
          ]
        },
        {
          "value": "deepseek-v4-pro",
          "name": "DeepSeek V4 Pro",
          "context_window": 1000000,
          "max_tokens": 384000
        }
      ]
    }
  ],
  "agent": {
    "max_steps": 8,
    "temperature": 0.2,
    "compaction_trigger_ratio": 0.8
  },
  "memory": {
    "enabled": true,
    "model": ""
  },
  "session": {
    "db_path": "~/.atlas/atlas.db"
  },
  "services": {
    "tavily": {
      "api_key": "tvly-..."
    }
  }
}
```

字段说明：

- `active_provider` 必须匹配某个 `providers[].name`，Atlas 只使用当前选中的 Provider。
- `providers` 是 Provider 配置数组；每项都需要完整有效，`name` 不能重复。
- `providers[].default_model` 必须匹配同一项里的 `providers[].models[].value`。
- `providers[].format` 可省略，默认 `chat_completions`；OpenAI Responses API 使用 `responses`。
- `providers[].models[].value` 是发送给 Provider 的模型名，`name` 用于显示。
- `context_window` 用于上下文压缩和用量展示，`max_tokens` 是每次模型请求的最大输出 token 数。
- `providers[].models[].reasoning_efforts` 声明模型支持的思考深度选项；未显式选择时使用第一项。
- `agent.compaction_trigger_ratio` 默认 `0.8`，表示上下文输入达到模型窗口 80% 时自动压缩早期对话。
- `memory.enabled` 默认启用。`memory.model` 为空时，后台记忆任务使用产生该会话的模型。
- `session.db_path` 默认 `~/.atlas/atlas.db`。
- `services.tavily.api_key` 配置后启用 `web_search` 和 `web_fetch`，请求会发送给 Tavily。
- `services.weixin.base_url` 可省略，默认 `https://ilinkai.weixin.qq.com`。

当前项目处于早期阶段，不提供数据库迁移框架。schema 变化后请删除旧的 `~/.atlas/atlas.db` 重新生成。

## 使用

常用命令：

```sh
go run ./cmd/atlas
go run ./cmd/atlas run "读取 README 并总结"
go run ./cmd/atlas run --model deepseek-v4-pro "分析这个问题"
go run ./cmd/atlas run --session 20260608-153012-a1b2c3d4 "继续刚才的问题"
go run ./cmd/atlas doctor
go run ./cmd/atlas sessions
go run ./cmd/atlas session show 20260608-153012-a1b2c3d4
go run ./cmd/atlas session compact 20260608-153012-a1b2c3d4
go run ./cmd/atlas session delete 20260608-153012-a1b2c3d4
go run ./cmd/atlas acp
go run ./cmd/atlas weixin login
go run ./cmd/atlas weixin serve
go run ./cmd/atlas weixin accounts
go run ./cmd/atlas weixin logout <account-id>
go run ./cmd/atlas version
```

裸 `atlas` 是交互模式入口；当前版本暂未实现 TUI，会提示使用 `atlas run`。

`atlas run` 默认创建新 session。传入 `--session <id>` 时恢复或创建指定 session；传入 `--model <value>` 时，本轮使用该模型。session ID 只允许字母、数字、`.`、`_` 和 `-`。

用户输入以 `!` 开头时，Atlas 跳过模型，直接把后续内容作为平台默认 shell 命令执行并返回输出，比如 `!pwd` 或 `!git status`。通过 shell 调 CLI 时，建议使用单引号或转义 `!`，避免 zsh 或 bash 历史展开改写命令：

```sh
go run ./cmd/atlas run '!pwd'
```

`atlas doctor` 只做离线诊断，检查配置、Provider 配置摘要、agent 参数、session 数据库、长期记忆表、Tavily 配置和默认 shell，不调用模型或 Tavily API。

## 会话、压缩和记忆

Atlas 使用 SQLite 保存本地会话和长期记忆。默认路径是 `~/.atlas/atlas.db`。

会话支持创建、恢复、列表、查看、删除和上下文压缩。`/compact` 或 `atlas session compact <id>` 会把较早上下文摘要化，并保留最近消息继续对话。

长期记忆默认启用。Atlas 会在新增消息达到阈值、用户明确要求记住信息或上下文压缩后，把增量抽取任务写入后台队列。ACP 和微信等长连接入口会处理队列，并在后续请求中自动检索相关记忆。

## ACP

`atlas acp` 通过 stdin/stdout 启动 Agent Client Protocol 服务，供 Zed 等客户端连接。

当前支持：

- session 创建、恢复、加载历史回放、列表分页、删除。
- prompt、取消、关闭。
- 模型切换、思考强度切换、思维链流式更新。
- embedded text resource。
- session info 和 usage update。
- 客户端 terminal 展示 `run_shell` 输出。
- 文件工具 locations/diff 展示。
- 长期记忆后台 worker。
- `/compact` slash command。

`additionalDirectories` 会作为 session 元数据保存和返回，但相对路径仍以 `cwd` 为基准。当前不支持 ACP auth、权限请求、MCP 连接，也不支持图片、音频、二进制资源输入。

## 微信

`atlas weixin login` 使用微信扫码登录，并把账号 token 保存到 `~/.atlas/weixin/accounts`。`atlas weixin serve` 连接微信 Bot，长轮询文本消息并调用本地 Atlas runtime。

微信通道拥有与本机 Atlas 进程相同的文件和 shell 权限。首次收到消息时，工作目录使用 `atlas weixin serve` 启动时的当前目录。当前只支持扫码登录的微信用户本人控制 Atlas，不支持群聊、媒体消息或添加其他控制人。

微信聊天支持这些斜杠命令：

- `/help`：查看命令。
- `/status`：查看当前工作目录和 session。
- `/cwd`：查看当前工作目录。
- `/cwd /absolute/path`：切换工作目录，并让下一条普通消息开启新对话。
- `/cwd -`：切回上一个工作目录。
- `/new`：在当前工作目录开启新对话。
- `/sessions`：查看当前工作目录最近会话。
- `/sessions all`：查看全局最近会话。
- `/resume <session-id>`：恢复指定会话，并切换到该会话的工作目录。
- `/compact`：压缩当前会话上下文。
- `/cancel`：取消当前正在运行的 turn。

## 内置工具

- `list_files`：列出文件和子目录，可按深度、glob 和 `.gitignore` 过滤。
- `read_file`：读取文本文件。
- `search_text`：按字面量或正则搜索文本。
- `edit_file`：精确替换一个或多个唯一文本块。
- `write_file`：写入文件内容。
- `run_shell`：使用平台默认 shell 执行命令；Windows 使用 PowerShell，其他平台使用 `/bin/sh`。
- `load_skill`：按名称加载本地 skill 指令。
- `web_search`：使用 Tavily 搜索公网网页，需要配置 `services.tavily.api_key`。
- `web_fetch`：使用 Tavily 提取公网网页内容，需要配置 `services.tavily.api_key`。
