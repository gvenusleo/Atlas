# 开发

[English](../development.md)

## 当前状态

仓库目前是 Dart 与 Flutter workspace 骨架。现有 Flutter 应用外壳可以运行；runtime、daemon、CLI、TUI、ACP 与 MCP 行为仍处于规划阶段。

## Workspace 结构

```text
packages/atlas_core       领域模型与 ports
packages/atlas_runtime    共享 Agent engine
packages/atlas_storage    SQLite 持久化
packages/atlas_provider   模型 Provider 适配器
packages/atlas_tools      内置工具
packages/atlas_rpc        通用 JSON-RPC 支持
packages/atlas_protocol   客户端 wire protocol
packages/atlas_acp        ACP 适配器
packages/atlas_mcp        MCP 适配器
packages/atlas_tui        Nocterm 客户端
apps/atlasd               本地 runtime host
apps/atlas_cli            CLI 与终端入口
apps/atlas_flutter        Flutter 桌面端与移动端客户端
```

根 Pub workspace 维护唯一的 `pubspec.lock`。所有成员使用 `resolution: workspace`，不得增加成员级 lockfile。

## 工具链

根 `mise.toml` 固定 Flutter 3.44.9，其中包含 Dart 3.12.2。

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

## Package 规则

- 代码放在真正拥有该行为的 package 中，不要把无关 helper 堆进 `atlas_core`。
- 只有真实适配器或测试需要时才增加公共抽象。
- 公共 Dart API 必须有简明文档注释。
- Runtime 与协议 package 不得导入 Flutter。
- 客户端 package 不得导入 Provider、工具或存储实现。
- 生成的序列化文件与源文件放在一起，仅在所选生成器要求时提交。
- 行为实现必须添加聚焦测试；空骨架 package 不需要占位测试。

## 文档规则

- 根 README 只描述产品状态和可用命令，不写内部架构。
- 架构和依赖边界写入 `docs/architecture.md`。
- 不可用行为必须标记为 `Planned`；功能移除时同步删除失效示例。
- 英文与中文对应文档必须同步更新。
- 高影响且难以回退的决策记录在 `docs/decisions`。
