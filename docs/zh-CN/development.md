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
```

## 构建与测试

```sh
go build ./cmd/atlas           # 构建
go test ./...                  # 运行全部测试
go test ./internal/agent/...   # 运行单个包的测试
go test ./internal/tui         # 运行终端界面测试
just ci                        # 完整且不修改文件的 CI 检查（需安装 just）
```

## 设计原则

- **小而可验证**：agent loop 保持 headless 和依赖注入，Provider 与工具副作用通过窄接口进入；配置、持久化和压缩由 runtime 负责。
- **不提前抽象**：两个真实调用点出现前不抽象，不为"可能以后"保留两套接口。
- **本地权限边界**：不引入权限抽象，工具拥有本机进程的全部权限。
- **单一核心**：TUI、CLI 命令、ACP 和 WebSocket 共享同一个 `runtime.Runtime` 和 agent loop，入口层只适配界面或协议。
