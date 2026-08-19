# Atlas

Atlas 是一个本地通用 AI Agent，当前正在重建为统一的 Dart 与 Flutter 项目。

[English](README.md)

## 当前状态

仓库目前包含：

- 定义 runtime、协议、客户端与适配器边界的 Pub workspace；
- 已接入本地 runtime 的 Flutter 桌面端和移动端客户端，支持会话、agent
  turn、文件浏览器与内嵌终端；
- 可运行的 `atlas_runtime` Agent engine 与 `atlas_storage` Drift 适配器；
- 提供 OpenAI-compatible Chat Completions 和 Responses 以及 Anthropic Messages
  适配器，并通过 composite provider 将多个 provider 路由到同一个 runtime 的
  `atlas_provider` 包；
- 构建系统提示词并加载 AGENTS.md 指令文件的 `atlas_prompt` 包；
- 把 config、provider、工具、存储与系统提示词组装进同一个 runtime，供
  `atlas_cli` 与 `atlas_flutter` 共用的 `atlas_composition` 包；
- `atlas_tui` 中作为默认 `atlas` 终端入口运行的 Nocterm 聊天界面，支持
  斜杠命令（`/model`、`/new`、`/resume`、`/compact`、`/quit`）与 skill
  注入；`/compact` 可带可选指令，`/model` 之后可选择 reasoning effort；
- `atlas_acp` 中的 ACP 服务端适配器，由 `atlas acp` 通过 NDJSON stdio
  提供，覆盖会话生命周期、模型与 effort 配置、斜杠命令、turn 流式输出、
  agent plan，以及 Zed 展示增强（实时 shell 终端、文件 diff 与
  follow-along 位置）；
- Dart 实现需要遵守的架构与开发规范。

WebSocket transport 与 MCP 集成尚未实现。CLI 可通过 `mise run build-cli`
构建为单文件可执行程序（`build/bundle/bin/atlas`）。

## 安装

一条命令安装最新版本：

```sh
curl -fsSL https://github.com/gvenusleo/atlas/releases/latest/download/install.sh | bash
```

Windows（PowerShell）：

```powershell
irm https://github.com/gvenusleo/atlas/releases/latest/download/install.ps1 | iex
```

或从源码构建并安装：

```sh
mise run build-cli    # build/bundle/bin/atlas
mise run install      # 安装到 ~/.local/bin
```

## 开发

开发环境需要 Git 和 [mise](https://mise.jdx.dev/)。

```sh
mise install
mise run deps
mise run ci
```

在 macOS 上运行 Flutter 客户端：

```sh
mise run app-run --device macos
```

Workspace 命令见[开发文档](docs/zh-CN/development.md)，runtime 边界见[架构文档](docs/zh-CN/architecture.md)，配置 schema 见[配置文档](docs/zh-CN/configuration.md)，工具行为见[内置工具](docs/zh-CN/tools.md)。

## 安全模型

Atlas 设计为使用本地进程的权限执行工具，不提供沙箱、权限提示或 approval gate。
当前 runtime、工具、存储、TUI、ACP 与 Flutter 客户端均遵循这一边界。
WebSocket transport 与 MCP 尚未实现。

## 许可证

[MIT](LICENSE)
