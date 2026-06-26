# Atlas

用 Go 编写的**通用 Agent**。核心是一个可测试的 headless agent loop，可读写文件、执行 Shell、搜索网页、长期记忆，CLI、ACP（供 Zed 等编辑器连接）和微信通道都通过 `internal/runtime` 调用同一套能力，不重复实现循环逻辑。

[English](README.md)

## 特性

- **Headless agent 核心**：模型 → 工具调用 → 工具结果，按顺序写回 transcript，循环直到完成或达到步数上限。
- **多 Provider 适配**：通过 `chat_completions` 和 `responses` 两种 API 格式适配器接入 OpenAI、DeepSeek 等兼容后端。
- **本地工具集**：文件读写、文本搜索、精确编辑、Shell 执行、网页搜索与提取，开箱即用。
- **上下文压缩**：达到上下文窗口阈值时自动摘要早期对话，保留最近消息继续。
- **长期记忆**：从会话中增量抽取 instruction / fact / workflow 三类记忆，按 global / project 作用域组织，FTS5 检索后注入后续会话。
- **多入口**：CLI 单次执行、ACP 长连接（支持编辑器嵌入终端与文件 diff）、微信扫码远程控制。
- **本地优先**：会话和记忆全部存于本地 SQLite，数据不离开用户机器（除模型 API 和可选的 Tavily 搜索外）。
- **可扩展指令**：通过 `AGENTS.md` 和 skill 文件注入项目级与全局指令，skill 按需加载。

## 快速开始

### 前置要求

- Go 1.26+
- 一个兼容 OpenAI Chat Completions 或 Responses API 的模型后端（如 DeepSeek、OpenAI）

### 安装

```sh
git clone https://github.com/gvenusleo/atlas.git
cd atlas
go build -o dist/atlas ./cmd/atlas
```

或使用 [just](https://github.com/casey/just)：

```sh
just build        # 构建到 dist/atlas
just install      # 构建并安装到 ~/.local/bin
```

### 首次配置

在 `~/.atlas/config.json` 创建配置文件（最小示例）：

```json
{
  "default_model": "deepseek-v4-flash",
  "providers": [
    {
      "name": "deepseek",
      "format": "chat_completions",
      "base_url": "https://api.deepseek.com",
      "api_key": "sk-...",
      "models": [
        {
          "value": "deepseek-v4-flash",
          "name": "DeepSeek V4 Flash",
          "context_window": 1000000,
          "max_tokens": 384000,
          "input_formats": ["text"]
        }
      ]
    }
  ]
}
```

验证配置：

```sh
go run ./cmd/atlas doctor
```

### 运行第一个任务

```sh
go run ./cmd/atlas run "读取 README 并总结"
```

## 使用

```sh
atlas run "<prompt>"                    # 执行单次任务
atlas run --model <value> "<prompt>"    # 指定模型
atlas run --session <id> "<prompt>"     # 恢复或创建指定 session
atlas acp                               # 启动 ACP 服务
atlas doctor                            # 离线诊断
atlas sessions                          # 列出会话
atlas session show <id>                 # 查看会话内容
atlas session compact <id>              # 压缩会话上下文
atlas session delete <id>               # 删除会话
atlas weixin login                      # 微信扫码登录
atlas weixin serve                      # 启动微信通道
atlas weixin accounts                   # 查看已登录账号
atlas weixin logout <account-id>        # 登出微信账号
atlas version                           # 查看版本
```

用户输入以 `!` 开头时，Atlas 跳过模型，直接把后续内容作为 shell 命令执行，比如 `!pwd`。通过 shell 调 CLI 时建议用单引号或转义 `!`：

```sh
go run ./cmd/atlas run '!pwd'
```

## 权限与安全

Atlas 以当前进程的本地权限运行。内置工具可以读写文件、搜索文本并执行 shell 命令；**Atlas 不提供沙箱、权限提示或 approval gate**。请只在可信工作区中运行。

所有会话和记忆数据存储在本地 SQLite，不离开用户机器——除模型 API 调用和可选的 Tavily 搜索外。

## 文档

- [架构](docs/zh-CN/architecture.md) — 分层设计、核心循环、长期记忆
- [配置](docs/zh-CN/configuration.md) — 完整配置参考与字段说明
- [通道](docs/zh-CN/channels.md) — ACP 与微信集成详情
- [工具与 Skill](docs/zh-CN/tools.md) — 内置工具、AGENTS.md、Skill 系统
- [开发](docs/zh-CN/development.md) — 项目结构、构建测试、设计原则

## 许可证

[MIT](LICENSE)
