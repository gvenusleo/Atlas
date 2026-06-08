# Atlas 开发指南

## 项目方向

Atlas 是一个从干净 Go module 重新开始的 coding agent。第一阶段目标是做出一个小而可测的 headless agent core。CLI 和 TUI 都只是套在 core 外面的薄壳，不应该反过来主导核心设计。

## 协作方式

用户负责写代码。助手负责指导架构、审查设计、解释权衡，并指出具体文件、接口和测试路径。除非用户明确要求改代码，否则不要直接做实现改动。

提出实现建议前：

- 明确说明假设。
- 遇到歧义时把不同解释说出来，不要默默选择。
- 优先选择最小、可独立验证的开发切片。
- 每一步都必须服务当前项目目标。

## 当前产品范围

第一版要先做：

- headless `Agent.RunTurn` 循环。
- OpenAI-compatible 模型 provider 接口。
- 最小内存 transcript。
- 小型 tool registry。
- 内置本地工具：列文件、读文件、搜索文本、写文件、执行 shell 命令。
- 一次处理一个 prompt 的简单 CLI。
- 用 fake provider 覆盖 loop 和工具行为的测试。

第一版不要做：

- 权限或审批系统。
- 沙箱。
- SQLite 或持久化 session 存储。
- TUI。
- MCP、ACP、插件系统、skills、subagents、后台任务、goal mode、compaction。
- 多 provider catalog 或 provider 自动发现。

## 访问模型

Atlas 工具默认拥有 Atlas 进程本身拥有的本地权限。没有权限提示、没有沙箱边界、没有 approval gate。这是刻意的产品选择，不是遗漏的功能。

因此：

- 工具实现可以在进程环境内读、写、执行命令。
- CLI 和 README 必须清楚说明 Atlas 拥有完整本地访问权限。
- 测试只验证行为，不验证权限提示。
- 除非用户明确改变方向，否则不要为了“以后灵活”添加权限抽象。

## 应用配置

Atlas 从用户主目录下的 `.atlas/config.json` 读取应用配置。第一版配置只需要覆盖 OpenAI-compatible Provider 和 agent loop 参数。

Provider 配置包括 `base_url`、`api_key` 和 `model`。这些字段属于具体 Provider 的连接配置，不应该放进 `model.ChatRequest`，也不应该由 `agent` 包直接关心。

第一版不要做：

- 环境变量配置入口。
- 多 profile。
- provider catalog 或自动发现。
- 配置热加载。
- 密钥加密存储。

## 系统提示词

第一版系统提示词是内置的模型行为契约，不是可扩展 prompt 框架。它应该说明 Atlas 的身份、本地访问模型、代码修改纪律、工具使用原则、验证习惯和回复风格。

系统提示词不要重复工具 JSON schema。工具定义由 `tool.Registry` 发送给 provider，系统提示词只描述何时、为何使用工具。

Atlas 会把 `~/.atlas/AGENTS.md` 和当前工作目录下的 `AGENTS.md` 作为附加指令组合进系统提示词。第一版不要向父目录或子目录递归查找 `AGENTS.md`。

第一版不要做：

- prompt 模板系统。
- prompt profile。
- skills 或 subagent 指令拼接。
- 自动加载大型项目上下文或层级 AGENTS.md。

## 架构形态

第一版推荐包结构：

```text
cmd/atlas
internal/agent
internal/config
internal/model
internal/provider/openai
internal/tool
internal/transcript
```

核心流程：

```text
CLI input
  -> Agent.RunTurn
  -> 从 transcript 构建模型消息
  -> provider stream
  -> 追加 assistant 输出
  -> 执行模型请求的工具
  -> 追加 tool results
  -> 重复直到没有 tool calls 或达到 max steps
```

保持 loop 可预测：

- 一个 turn 有一个单调递增的 turn ID。
- 一个 turn 内包含编号 step。
- 每个模型 tool call 都必须有一个配对的 tool result。
- tool result 顺序必须跟模型返回的 tool call 顺序一致。
- 必须有 max-step 限制。
- 公共接口要支持 cancellation，即使第一版 CLI 不重度使用。
- agent loop 应提供最小 observer 事件，方便 CLI、测试和后续 UI 观察工具执行过程。

## 测试标准

所有关键行为都应该用 fake model provider 写小测试。优先覆盖：

- 纯文本回复。
- 一次工具调用后继续输出最终文本。
- 未知工具返回模型可见的错误。
- 工具参数 JSON 非法时返回模型可见的错误。
- max-step 耗尽。
- 在临时目录中验证文件读写工具。
- shell 命令成功和失败。

交付前运行：

```sh
go test ./...
```

## 设计偏好

- 代码保持小而明确。
- 不要在两个真实调用点出现前添加抽象。
- 优先使用 Go 标准库。
- 只有外部依赖能明显降低复杂度时才引入。
- 注释要简明且有用，新增长函数和长类型需要说明意图。
- 不要因为旧 Atlas 实现曾经存在，就把旧细节重新加回来。

## 参考其他 agent 的经验

值得吸收：

- 先做 headless core，UI mode 只是外壳。
- provider 接口和 turn loop 分离。
- tool registry 和具体 tool 实现分离。
- tool call 和 tool result 要有稳定的事件顺序。
- 用 fake provider 写确定性测试。

第一版不要吸收：

- 大型插件生态。
- 权限引擎。
- 持久化 session 数据库。
- 后台任务系统。
- 复杂 TUI 状态机。
- 远程控制协议。
