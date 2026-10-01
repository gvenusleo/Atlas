# 开发

[English](../development.md)

## Workspace 结构

```text
packages/atlas_runtime       Session/Turn 领域、timeline、ports 与 Agent engine
packages/atlas_storage       Drift 持久化与行映射
packages/atlas_provider      模型 Provider 适配器
packages/atlas_config        YAML 配置加载与校验
packages/atlas_prompt        系统提示词与 skill catalog 加载
packages/atlas_composition   CLI 与 Flutter 共用的 runtime 组装
packages/atlas_tools         内置工具
packages/atlas_ws            版本化 WebSocket 协议与 transport
packages/atlas_acp           ACP 适配器
packages/atlas_mcp           MCP 客户端工具（stdio 与 Streamable HTTP）
packages/atlas_tui           Nocterm 展示 package
apps/atlas_cli               atlas CLI 与 TUI，另有 `atlas acp`、`atlas server`、`atlas cache`
apps/atlas_flutter           Flutter 桌面端与移动端应用
```

根 Pub workspace 维护唯一的 `pubspec.lock`；所有成员使用 `resolution: workspace`，不得增加成员级 lockfile。各 package 的职责见[架构文档](architecture.md)。

## 工具链

根 `mise.toml` 固定 Flutter 3.47.4，其中提供 Dart 3.13。所有 package 声明 `sdk: ^3.13.0`，因此代码使用的 Dart 3.13 特性（主构造函数、sealed 类型 switch 的穷尽性检查）无需实验开关。

```sh
mise install
mise run deps
```

只有在有意调整依赖约束或 lockfile 时才使用 `mise run deps-update`。

## 验证

```sh
mise run fmt          # 格式化 Dart 源码
mise run fmt-check    # 不改文件，仅检查格式
mise run analyze      # 分析整个 workspace
mise run test         # 运行已有 Dart 与 Flutter 测试
mise run ci           # 完整仓库验证
```

使用 `mise run app-run --device macos` 运行 Flutter 客户端。各平台 release 构建使用对应的 `mise run app-build-*` 任务；`mise run app-install-macos` 把本地构建的 macOS 应用安装到 `/Applications`（可用 `APP_INSTALL_DIR` 覆盖）。

使用 `mise run cli-build` 构建单文件 CLI。Dart 3.13 的 `dart build cli` 产物为 `build/bundle/bin/atlas`；带 build hooks 的 package（sqlite3）不能使用 `dart compile exe`。`mise run cli-install` 把本地构建的二进制安装到 `~/.local/bin`；终端用户通过 `install.sh`（macOS/Linux）或 `install.ps1`（Windows）安装 release 二进制，脚本会按平台与架构选择产物，并支持 `ATLAS_INSTALL_DIR`。

推送 `v*.*.*` tag 即发布新版本：`.github/workflows/release.yml` 用 `dart build cli` 构建 linux（amd64/arm64）、macOS（amd64/arm64）与 Windows（amd64）二进制，连同安装脚本一起上传。Release notes 在 GitHub Release 页自动生成，仓库不维护独立 changelog。

## CLI 行为

`bin/atlas.dart` 只转发参数并设置返回的退出码。`--help`、`help <command>` 和 `<command> --help` 均不依赖配置；参数错误在加载配置或打开存储之前写入 stderr。退出码为：成功 0、用法错误 64、配置失败 78、意外失败 70；`--verbose` 会附加精简堆栈。

默认 TUI 要求 stdin/stdout 都是终端、支持 ANSI，且未设置 `NO_COLOR`（即使值为空也会禁用 TUI）。不支持的终端会被明确拒绝且不输出转义序列，非交互子命令仍可用；TUI 退出时先恢复输入模式与光标，再释放 stdin。各命令关闭自身资源后自然返回，不调用 `exit()`。

CLI package 声明了 `atlas` 可执行入口，在 workspace 根目录可运行 `dart run atlas_cli:atlas --help`。版本由 `build_version` 从 `apps/atlas_cli/pubspec.yaml` 生成：修改版本后运行 `mise run cli-version` 并提交 `apps/atlas_cli/lib/src/version.dart`。`mise run cli-build` 会自动生成该文件，release 构建会校验 `--version` 与 tag 一致。

`mise run cli-integration-test` 已纳入 `mise run ci`：它在 `.dart_tool/atlas_cli/` 下构建隔离 bundle，检查真实进程输出、退出码、ACP EOF 与资源清理；设置 `ATLAS_TEST_BINARY` 可直接验证已有产物。macOS/Linux 上用 FFI 探针创建真实 PTY，并在退出前后比较终端状态，覆盖整段与逐字符输入的 `/quit`、信号、终端恢复与 `NO_COLOR`（Windows 跳过 POSIX 专属用例）。Release job 对各平台产物运行同一套测试。

### 阅读 `atlas cache`

`atlas cache --limit 200` 从同一个数据库快照采样最近 200 个 turn 及其已持久化的对话回复，包括被上下文压缩隐藏的历史，并按 Provider/model 与 session 分组。压缩摘要调用和未产生回复记录的请求尝试不计入，因此不是完整账单。

- **Token hit rate**：可测请求的缓存读取 token 总和除以完整输入 token 总和；缓存写入与输出 token 不算命中。
- **Requests with hits**：可测请求中发生过缓存读取的比例，不代表整个 prompt 全部命中。
- **Cache-data coverage**：可测请求数除以已记录回复数。未知、数据不一致、已中断及零输入记录分别说明且不参与命中率计算；未知命中率显示 `n/a`，不算缓存未命中。

Provider 适配器在原始计数之外持久化归一化的完整输入量与缓存字段可用性，旧记录仍可读取，但不会依赖当前配置重新解释。输出为纯文本、适配终端宽度，管道输出或设置 `NO_COLOR` 时同样没有转义序列。

## Package 规则

- 领域概念与 runtime ports 放在 `atlas_runtime`；Provider、存储、工具、UI 与协议实现放在各自所属 package。
- 只有真实适配器或测试需要时才增加公共抽象。
- 用 `dart pub add` 在拥有该行为的 package 中声明依赖，`atlas_flutter` 使用 `flutter pub add`；不要在 workspace 根预声明。
- Atlas 自有 HTTP 请求使用 Dio。MCP 集成采用 `mcp_dart` 及其 `package:http` 依赖处理 MCP 传输与认证；该例外仅限 MCP 适配器。只有 `atlas_ws` 出现实现时才添加 WebSocket 依赖。
- 公共 Dart API 需要简明文档注释。Runtime 与协议 package 不得导入 Flutter，展示 package 不得导入 Provider、工具或存储实现。
- 应用 bootstrap 负责组装适配器并注入 runtime；两个应用根都使用 `atlas_composition`，`atlas_ws` 接收注入的 request handler。
- 生成的序列化文件与源文件放在一起，仅在生成器要求时提交。行为实现需要聚焦测试；空骨架 package 不需要占位测试。

文档规则见[文档指南](README.md)。
