# Atlas

Atlas 是一个本地通用 AI Agent，当前正在重建为统一的 Dart 与 Flutter 项目。

[English](README.md)

## 当前状态

仓库目前包含：

- 定义 runtime、协议、客户端与适配器边界的 Pub workspace；
- 初步完成的 Flutter 桌面端和移动端应用外壳；
- 可执行的 `atlas_runtime` Agent engine 与 `atlas_storage` Drift 适配器；
- 提供 OpenAI-compatible Chat Completions 和 Responses 以及 Anthropic Messages
  适配器，并通过 composite provider 将多个 provider 路由到同一个 runtime 的
  `atlas_provider` 包；
- 构建系统提示词并加载 AGENTS.md 指令文件的 `atlas_prompt` 包；
- 把 config、provider、工具、存储与系统提示词组装进同一个 runtime 的
  `atlas_cli` 组合根；
- `atlas_tui` 中作为默认 `atlas` 终端入口运行的 Nocterm 聊天界面，支持
  斜杠命令（`/model`、`/new`、`/resume`、`/compact`、`/quit`）与 skill
  注入；
- `atlas_acp` 中的 ACP 服务端适配器，由 `atlas acp` 通过 NDJSON stdio
  提供，覆盖会话生命周期、模型与 effort 配置、斜杠命令、turn 流式输出
  与 agent plan；
- Dart 实现需要遵守的架构与开发规范。

WebSocket transport 与 MCP 集成尚未实现。CLI 可通过 `just build-cli`
构建为单文件可执行程序（`build/bundle/bin/atlas`）。

## 开发

开发环境需要 Git、[mise](https://mise.jdx.dev/) 和 [just](https://github.com/casey/just)。

```sh
mise install
just deps
just ci
```

在 macOS 上运行现有 Flutter 应用外壳，或构建单文件 CLI 可执行程序：

```sh
just app-run macos
just build-cli   # build/bundle/bin/atlas
```

Workspace 命令见[开发文档](docs/zh-CN/development.md)，runtime 边界见[架构文档](docs/zh-CN/architecture.md)。

## 安全模型

Atlas 设计为使用本地进程的权限执行工具，不提供沙箱、权限提示或 approval gate。当前已实现的 runtime 与 storage 遵循这一边界；Provider、工具和客户端集成仍在开发中。

## 许可证

[MIT](LICENSE)
