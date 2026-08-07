# Atlas

Atlas 是一个本地通用 AI Agent，当前正在重建为统一的 Dart 与 Flutter 项目。

[English](README.md)

## 当前状态

仓库目前包含：

- 定义 runtime、协议、客户端与适配器边界的 Pub workspace；
- 初步完成的 Flutter 桌面端和移动端应用外壳；
- 可执行的 `atlas_runtime` Agent engine 与 `atlas_storage` Drift 适配器；
- Dart 实现需要遵守的架构与开发规范。

Provider、CLI、WebSocket transport、Nocterm TUI、ACP 与 MCP 集成尚未实现。当前分支暂不提供可用的 Atlas 命令行发行版。

## 开发

开发环境需要 Git、[mise](https://mise.jdx.dev/) 和 [just](https://github.com/casey/just)。

```sh
mise install
just deps
just ci
```

在 macOS 上运行现有 Flutter 应用外壳：

```sh
just app-run macos
```

Workspace 命令见[开发文档](docs/zh-CN/development.md)，runtime 边界见[架构文档](docs/zh-CN/architecture.md)。

## 安全模型

Atlas 设计为使用本地进程的权限执行工具，不提供沙箱、权限提示或 approval gate。当前已实现的 runtime 与 storage 遵循这一边界；Provider、工具和客户端集成仍在开发中。

## 许可证

[MIT](LICENSE)
