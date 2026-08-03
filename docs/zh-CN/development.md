# 开发

[English](../development.md)

## 项目结构

```text
cmd/atlas              CLI 入口
internal/acp           ACP 协议适配与客户端能力桥接
internal/agent         headless agent loop（核心循环）
internal/compact       上下文压缩规划与摘要
internal/config        配置加载与校验
internal/model         通用聊天协议与 Provider 接口
internal/prompt        系统提示词构造
internal/provider      按 API 格式实现的 Provider 适配器
  ├── chatcompletions  Chat Completions API
  └── responses        OpenAI Responses API
internal/runtime       编排层，串联 agent、工具和 session
internal/session       SQLite 会话持久化
internal/skill         skill 扫描与加载
internal/tool          工具注册表与内置工具
internal/transcript    内存消息序列
internal/tui           交互式终端界面
internal/version       版本信息
internal/ws            WebSocket 通道
app                    Flutter 桌面端与移动端客户端
  ├── lib/app          应用根节点、路由和平台集成
  ├── lib/features     按功能组织的页面、布局和组件
  └── lib/shared       应用级主题和共享 UI
```

## 构建与测试

```sh
go build ./cmd/atlas           # 构建
go test ./...                  # 运行全部测试
go test ./internal/agent/...   # 运行单个包的测试
go test ./internal/tui         # 运行终端界面测试
just ci                        # 完整且不修改文件的 CI 检查（需安装 just）
```

### Flutter App

Flutter 客户端使用 FVM 固定的 SDK。在 `app/` 目录运行：

```sh
fvm flutter run -d macos          # 运行 macOS 客户端
fvm flutter analyze               # 静态分析
fvm flutter test                  # Widget 与单元测试
fvm flutter build macos --debug   # 构建 macOS Debug App
```

当前客户端只实现了响应式应用外壳，尚未接入 Atlas runtime。

## 从源码运行

```sh
go run ./cmd/atlas                              # 启动终端界面
go run ./cmd/atlas run "读取 README 并总结"          # 执行单次任务
go run ./cmd/atlas doctor                       # 验证配置
```

## 设计原则

- **小而可验证**：agent loop 保持 headless 和依赖注入，Provider 与工具副作用通过窄接口进入；配置、持久化和压缩由 runtime 负责。
- **不提前抽象**：两个真实调用点出现前不抽象，不为"可能以后"保留两套接口。
- **本地权限边界**：不引入权限抽象，工具拥有本机进程的全部权限。
- **单一核心**：TUI、CLI 命令、ACP 和 WebSocket 共享同一个 `runtime.Runtime` 和 agent loop，入口层只适配界面或协议。
- **轻量 Flutter 客户端**：Flutter 只负责客户端展示和交互状态；agent 编排、工具、Provider 和 session 持久化仍由 Go runtime 负责，后续通过 WebSocket 通道使用。
