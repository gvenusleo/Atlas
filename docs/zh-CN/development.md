# 开发

[English](../development.md)

## 当前状态

仓库目前是 Dart 与 Flutter workspace。`atlas_runtime`、`atlas_storage`、Provider 适配器、`atlas_config`、`atlas_tools`、`atlas_prompt`、`atlas_tui` Nocterm 聊天界面与 ACP 服务端适配器（`atlas_acp`）已是带聚焦测试的可执行 package；MCP 适配器与 WebSocket transport 仍处于规划阶段。

## Workspace 结构

```text
packages/atlas_runtime    Session/Turn 领域、timeline、ports 与 Agent engine
packages/atlas_storage    Drift 持久化与 runtime 行映射
packages/atlas_provider   模型 Provider 适配器
packages/atlas_tools      内置工具
packages/atlas_ws         版本化 WebSocket 协议与 transport
packages/atlas_acp        ACP 适配器
packages/atlas_mcp        MCP 适配器
packages/atlas_tui        Nocterm 展示 package
apps/atlas_cli            atlas CLI、TUI、server 与其他命令
apps/atlas_flutter        Flutter 桌面端与移动端应用
```

根 Pub workspace 维护唯一的 `pubspec.lock`。所有成员使用 `resolution: workspace`，不得增加成员级 lockfile。

## 工具链

根 `mise.toml` 固定 Flutter 3.47.0，其中包含 Dart 3.13.0。

```sh
mise install
just deps
```

只有在有意调整依赖约束或 lockfile 时才使用 `just deps-update`。

## 验证

```sh
just fmt          # 格式化 Dart 源码
just fmt-check    # 不改文件，仅检查格式
just analyze      # 分析整个 workspace
just test         # 运行已有 Dart 与 Flutter 测试
just ci           # 完整仓库验证
```

使用 `just app-run macos` 运行当前 Flutter 外壳。各平台 debug 构建使用对应的 `just app-build-*` recipe。

使用 `just build-cli` 构建单文件 CLI 可执行程序。Dart 3.13 的
`dart build cli` 产物为 `build/bundle/bin/atlas`；带 build hooks 的 package
（sqlite3）不能使用 `dart compile exe`。

## Package 规则

- 领域概念与 runtime ports 放在 `atlas_runtime`；Provider、存储、工具、UI 和协议实现分别放在其所属 package。
- 只有真实适配器或测试需要时才增加公共抽象。
- 不要为规划中的代码预先声明依赖。实现代码首次需要某个 package 时，在所属 Dart package 中运行 `dart pub add`；`atlas_flutter` 使用 `flutter pub add`。
- 所有 HTTP 请求统一使用 Dio，不得添加 `package:http` 或第二套 HTTP client。只有 `atlas_ws` 出现真实实现时才添加 WebSocket 依赖。
- 公共 Dart API 必须有简明文档注释。
- Runtime 与协议 package 不得导入 Flutter。
- 展示 package 不得导入 Provider、工具或存储实现。
- `atlas_cli` 与 `atlas_flutter` 的应用 bootstrap 负责组装这些适配器并注入 runtime。
- `atlas_ws` 只负责 WebSocket transport，并接收注入的 request handler。
- 生成的序列化文件与源文件放在一起，仅在所选生成器要求时提交。
- 行为实现必须添加聚焦测试；空骨架 package 不需要占位测试。

## 文档规则

- 根 README 只描述产品状态和可用命令，不写内部架构。
- 架构和依赖边界写入 `docs/architecture.md`。
- 不可用行为必须标记为 `Planned`；功能移除时同步删除失效示例。
- 英文与中文对应文档必须同步更新。
