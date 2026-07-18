# Atlas

用 Go 编写的**通用 Agent**。核心是一个可测试的 headless agent loop，可读写文件、执行 Shell、搜索网页。终端界面、CLI 命令、Zed 等 ACP 客户端和 WebSocket 通道都通过 `internal/runtime` 调用同一套能力，不重复实现循环逻辑。

[English](README.md)

## 特性

- **Headless agent 核心**：模型 → 工具调用 → 工具结果，按顺序写回 transcript，循环直到完成或达到步数上限。
- **多 Provider 适配**：通过 `chat_completions` 和 `responses` 两种 API 格式适配器接入 OpenAI、DeepSeek 等兼容后端。
- **本地工具集**：通过 Shell 完成文件检查、编辑与搜索，并支持网页搜索与提取，开箱即用。
- **上下文压缩**：达到配置阈值时自动压缩，也可手动触发；完整 transcript 保持不变，最近消息继续参与对话。
- **多入口**：交互式终端界面、CLI 单次执行、ACP 长连接和 WebSocket 服务，共享同一个运行时。
- **本地优先存储**：会话记录保存在本地 SQLite；任务内容和结果可能通过已配置的模型 API、Tavily 或已连接的 WebSocket 客户端传输。
- **可扩展指令**：通过 `AGENTS.md` 和 skill 文件注入项目级与全局指令，skill 按需加载。

## 快速开始

### 前置要求

- Go 1.26+
- 一个兼容 OpenAI Chat Completions 或 Responses API 的模型后端（如 DeepSeek、OpenAI）

### 安装

一行命令安装最新版本：

```sh
curl -fsSL https://github.com/gvenusleo/atlas/releases/latest/download/install.sh | bash
```

Windows (PowerShell)：

```powershell
irm https://github.com/gvenusleo/atlas/releases/latest/download/install.ps1 | iex
```

或从源码构建：

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
  "default_model": "deepseek/deepseek-v4-flash",
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

`default_model` 推荐使用 `provider/model` 格式（如 `"deepseek/deepseek-v4-flash"`）。无歧义时也可使用裸模型值（如 `"deepseek-v4-flash"`）；`--model` 参数同样适用。

验证配置：

```sh
go run ./cmd/atlas doctor
```

### 运行第一个任务

启动交互式终端界面：

```sh
atlas
```

从源码运行：

```sh
go run ./cmd/atlas
```

执行单次任务：

```sh
go run ./cmd/atlas run "读取 README 并总结"
```

## 使用

不带子命令运行 `atlas` 会打开本地终端界面。Atlas 也支持 CLI 单次任务、Zed 等 [ACP](https://agentclientprotocol.com/) 客户端和 WebSocket 客户端。详见[通道](docs/zh-CN/channels.md)。

```sh
atlas                                     # 启动交互式终端界面
atlas --session <id>                      # 在 TUI 中恢复或创建 session
atlas run "<prompt>"                      # 执行单次任务
atlas run --model <provider/model> "<prompt>"    # 指定模型，推荐 provider/model 格式
atlas run --session <id> "<prompt>"       # 恢复或创建指定 session
atlas acp                                 # 启动 ACP 服务
atlas serve                               # 启动 WebSocket 服务（默认监听本机回环地址）
atlas doctor                              # 离线诊断
atlas sessions                            # 列出会话
atlas session show <id>                   # 查看会话内容
atlas session compact <id>                # 压缩会话上下文
atlas session delete <id>                 # 删除会话
atlas version                             # 查看版本
```

TUI 或 `atlas run` 中的输入以 `!` 开头时，Atlas 会跳过模型，直接把后续内容作为 shell 命令执行，比如 `!pwd`。通过 shell 调 CLI 时建议用单引号或转义 `!`：

```sh
go run ./cmd/atlas run '!pwd'
```

## 权限与安全

Atlas 以当前进程的本地权限运行。内置工具可以读写文件、搜索文本并执行 shell 命令；**Atlas 不提供沙箱、权限提示或 approval gate**。请只在可信工作区中运行。

会话记录存储在本地 SQLite。Atlas 仍会向已配置的模型 Provider 发送请求上下文，并通过用户启用的服务或通道通信，包括 Tavily 和 WebSocket 客户端。

## 文档

- [架构](docs/zh-CN/architecture.md)：分层设计、核心循环、上下文压缩
- [配置](docs/zh-CN/configuration.md)：完整配置参考与字段说明
- [通道](docs/zh-CN/channels.md)：TUI 用法、ACP 与 WebSocket 集成详情
- [工具与 Skill](docs/zh-CN/tools.md)：内置工具、AGENTS.md、Skill 系统
- [开发](docs/zh-CN/development.md)：项目结构、构建测试、设计原则

## 许可证

[MIT](LICENSE)
