# Atlas

Atlas 是一个本地通用 AI Agent，以统一的 Dart 与 Flutter 项目构建。

[English](README.md)

## 当前状态

仓库是一个 Pub workspace，定义了 runtime、协议、客户端与适配器的边界。以下部分已经可用：

- `atlas_runtime` Agent engine，以及 `atlas_storage` 的 Drift 持久化；
- `atlas_provider`：OpenAI-compatible Chat Completions 与 Responses 适配器、Anthropic Messages 适配器，以及把多个 provider 路由到同一 runtime 的 composite provider；
- `atlas_composition`：把 config、provider、工具、存储与系统提示词组装为同一个 runtime，供 `atlas_cli` 与 `atlas_flutter` 使用；
- `atlas_tui` 的 Nocterm 聊天界面，作为默认 `atlas` 终端入口，支持斜杠命令（`/model`、`/new`、`/resume`、`/compact`、`/quit`）与 skill 注入；
- `atlas_acp` 的 ACP 服务端适配器（由 `atlas acp` 经 NDJSON stdio 提供），以及由 `atlas server` 提供的 `atlas_ws` WebSocket transport，Flutter 移动 App 可据此在私有网络上驱动电脑端的 Atlas；
- Flutter 桌面端与移动端客户端，始终通过 ACP 连接，具备会话、agent turn、文件浏览器与内嵌终端；
- 面向已配置 stdio 与 Streamable HTTP 服务器的 MCP 客户端工具，详见 [MCP 工具](docs/zh-CN/mcp.md)；
- `atlas cache` 子命令，从 session 数据库报告 prompt cache 复用情况；
- Pi 风格的 `settings.json`、`models.json`、`auth.json` 与 `mcp.json`，支持同一中转站使用多种协议、离线 models.dev 元数据，以及 `atlas config validate`、`atlas models list|refresh`、`atlas auth set|remove` 命令。

架构与工程规范见[文档指南](docs/zh-CN/README.md)。

## 安装

安装最新版本：

```sh
curl -fsSL https://github.com/gvenusleo/atlas/releases/latest/download/install.sh | bash
```

Windows（PowerShell）：

```powershell
irm https://github.com/gvenusleo/atlas/releases/latest/download/install.ps1 | iex
```

或从源码构建并安装：

```sh
mise run cli-build    # build/bundle/bin/atlas
mise run cli-install  # 安装到 ~/.local/bin
```

用 `atlas server` 启动远程端点（每次启动都会打印配对 token），再在 App 的远程连接页接入。

## 开发

开发环境需要 Git 和 [mise](https://mise.jdx.dev/)。

```sh
mise install
mise run deps
mise run ci
```

在 macOS 上运行 Flutter 客户端：`mise run app-run --device macos`。Workspace 命令见[开发文档](docs/zh-CN/development.md)，runtime 边界见[架构文档](docs/zh-CN/architecture.md)，配置 schema 见[配置文档](docs/zh-CN/configuration.md)，工具行为见[内置工具](docs/zh-CN/tools.md)。

## 安全模型

Atlas 使用本地进程的权限运行工具，不提供沙箱、权限提示或 approval gate，MCP 工具沿用同一边界。OAuth 登录与 ACP 会话传入的 MCP 配置仍为 Planned。

## 许可证

[MIT](LICENSE)
