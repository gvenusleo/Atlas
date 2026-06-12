# Atlas 开发指南

## 项目定位

Atlas 是一个用 Go 编写的本地 coding agent。核心是可测试的 headless agent loop；CLI、TUI 或其他界面都通过 `internal/runtime` 调用核心能力。

Atlas 的核心职责：

- 接收用户输入并追加到 transcript。
- 基于系统提示词、历史消息和工具定义调用模型 Provider。
- 按模型返回顺序执行工具调用。
- 将每个工具结果追加回 transcript。
- 在没有工具调用、遇到错误或达到最大步数时结束 turn。

## 协作方式

用户主要负责写代码。助手负责指导架构、审查设计、解释权衡，并指出具体文件、接口和测试路径。用户明确要求改代码时，助手可以直接实现。

提出实现建议前：

- 明确说明假设。
- 遇到歧义时列出不同解释。
- 选择最小、可独立验证的开发切片。
- 每一步都服务当前项目目标。

## 本地访问模型

Atlas 工具拥有 Atlas 进程本身拥有的本地权限。工具可以在进程权限范围内读文件、写文件、搜索文本并执行 shell 命令。

Atlas 不提供沙箱、权限提示或 approval gate。这是产品行为边界，不是缺失实现。除非产品方向明确变化，代码中不引入权限抽象。

README 和 CLI 文案需要清楚表达这一点；测试只验证工具行为和错误传播，不验证权限提示。

## 应用配置

Atlas 从用户主目录下的 `.atlas/config.json` 读取应用配置。配置文件包含 Provider 连接参数和 agent loop 参数。

Provider 配置包括：

- `base_url`
- `api_key`
- `default_model`
- `models`

`base_url`、`api_key`、`default_model` 和模型 `value` 属于具体 Provider 的连接配置，不进入 `model.ChatRequest`，也不由 `agent` 包直接关心。`agent` 只依赖 `model.Provider` 接口和通用生成参数。

`models` 中每个模型至少包含 `value`、`name`、`context_window` 和 `max_tokens`；`value` 是传给 Provider 的模型名，`name` 是展示名，`context_window` 是模型上下文窗口，`max_tokens` 是每次模型请求使用的最大输出 token 数。`default_model` 必须匹配其中一个 `value`。

## 系统提示词

系统提示词由 `internal/prompt` 构造，用来定义 Atlas 的模型行为契约。它描述身份、本地访问模型、代码修改纪律、工具使用原则、验证习惯和回复风格。

工具 JSON schema 由 `tool.Registry` 发送给 Provider。系统提示词只说明工具使用原则，不重复 schema。

Atlas 只加载两个附加 `AGENTS.md` 指令文件：

- `~/.atlas/AGENTS.md`
- 当前工作目录下的 `AGENTS.md`

当前用户请求优先于指令文件，当前目录指令优先于全局指令。Atlas 不递归查找父目录或子目录中的 `AGENTS.md`。

Atlas 会扫描用户级和当前目录级 skill，但系统提示词只注入 `name` / `description` 摘要。完整 `SKILL.md` 只能通过 `load_skill` 工具按需加载；Atlas 不递归查找父目录中的 skill。

## 架构边界

推荐包结构：

```text
cmd/atlas
internal/acp
internal/agent
internal/compact
internal/config
internal/model
internal/prompt
internal/provider/openai
internal/runtime
internal/session
internal/skill
internal/tool
internal/transcript
internal/version
internal/weixin
```

核心流程：

```text
CLI input
  -> runtime.RunTurn
  -> Agent.RunTurn
  -> build model.ChatRequest from transcript
  -> provider.Stream
  -> emit model text deltas
  -> append assistant message
  -> run requested tools
  -> append tool results
  -> repeat until final response or max steps
  -> save transcript to session store
```

ACP 输入通过 `internal/acp` 走同一个 `internal/runtime`。ACP 适配层只负责协议方法、session/update 通知和活动 session 状态；不复制 agent loop、工具执行或 transcript 持久化逻辑。

微信远程控制输入通过 `internal/weixin` 走同一个 `internal/runtime`。微信适配层只负责 iLink Bot 登录、消息轮询、typing 状态、斜杠命令、发送回复和发送人与本地 session 的轻量绑定；不复制 agent loop、工具执行或 transcript 持久化逻辑。

保持 loop 可预测：

- 一个 turn 内包含编号 step。
- 每个模型 tool call 都有一个配对的 tool result。
- tool result 顺序与模型返回的 tool call 顺序一致。
- 工具执行错误作为模型可见的 tool result 写回 transcript。
- `max_steps` 限制一次 turn 的模型调用次数。
- 公共接口接收 `context.Context`，用于 cancellation。
- agent loop 暴露最小 observer 事件，供 CLI、测试和后续 UI 观察执行过程。
- UI 和 ACP 必须按 observer 事件发生顺序渲染模型文本和工具调用状态。

## 取舍原则

Atlas 追求极简、可理解、可验证，不追求覆盖所有 provider、终端、平台和边缘场景。当前真实使用路径优先于理论完整性。

- 不提前做兼容矩阵。遇到真实 provider 差异后，用最小测试固定行为，再做局部适配。
- 不为了“可能以后”保留两套接口、fallback 或配置开关。
- 不为单一调用点创建抽象。
- 不把上游大型 agent 的完整架构搬进 Atlas，只吸收能让当前核心更小、更清楚的做法。

## Provider 边界

`model.Provider` 是 agent 与模型后端之间的唯一接口。Provider 只暴露流式调用，具体实现负责处理连接信息、鉴权、模型名、请求格式、SSE 解析和响应格式转换。

`model.ChatRequest` 表达 Atlas 的通用聊天协议：系统提示词、消息、工具定义和通用生成参数。Provider 负责把它转换成具体后端 API 请求，并在 stream 结束后返回累计完成的 `model.ChatResponse`。

## Tool 边界

工具通过 `tool.Tool` 接口注册：

- `Definition()` 返回发给模型的工具定义。
- `Run(ctx, arguments)` 接收模型提供的 JSON 字符串并返回文本结果。

工具实现保持无状态，参数解析放在 `Run` 内部。`tool.Registry` 负责稳定排序、重复名称检查、定义导出和按名称执行。

内置工具使用 Go 标准库实现。引入外部依赖前，先确认它能明显降低复杂度。

## Transcript 边界

`internal/transcript` 保存当前 agent 实例中的消息序列。调用方读取消息时得到副本，不能直接修改内部状态。

Transcript 不负责持久化、压缩或摘要。`internal/session` 负责把 transcript 和压缩元数据保存到本地 SQLite，并按 session ID 恢复消息。

## Session 边界

Atlas 使用 SQLite 保存本地会话，默认路径为 `~/.atlas/atlas.db`。`session.db_path` 可以在应用配置中覆盖。

`atlas run` 默认每次调用创建新的 session ID；传入 `--session <id>` 时恢复或创建指定 session。裸 `atlas` 和 `atlas --session <id>` 是交互模式入口，TUI 接入前只输出占位提示。session ID 只允许字母、数字、`.`、`_` 和 `-`。

当前 session 能力覆盖创建、恢复、保存 transcript、列出最近会话、查看会话详情、删除会话和上下文压缩。历史全文搜索和迁移框架不提前实现。

## 测试标准

关键行为使用 fake Provider 和临时目录测试。优先覆盖：

- 纯文本回复。
- 工具调用后继续生成最终回复。
- 未知工具错误对模型可见。
- 工具参数非法时错误对模型可见。
- tool result 顺序稳定。
- max-step 耗尽。
- 文件读写工具行为。
- shell 命令成功和失败。
- 系统提示词加载全局和当前目录指令。
- session 保存和恢复 transcript。
- session 列表、查看和删除命令。
- session 上下文压缩、摘要元数据保存和压缩后继续对话。
- ACP 初始化、session 创建、prompt、取消、关闭、恢复、列表和删除能力。
- ACP `/compact` slash command 暴露和手动压缩能力。
- ACP session/update 事件顺序与 agent observer 事件顺序一致。
- 微信通道扫码登录、账号保存、typing 状态、文本回复、工作目录切换、会话列表、会话恢复、上下文压缩和取消能力。

交付前运行：

```sh
go test ./...
```

## 代码风格

- 保持代码小而明确。
- 两个真实调用点出现前不抽象。
- 优先使用 Go 标准库。
- 只修改与当前任务直接相关的代码。
- 注释保持必要、简洁、符合 Go 规范。
- 导出类型、函数和包需要有规范注释。
- 测试、文档和注释服务行为理解，不堆砌说明。
- 不因为旧 Atlas 实现存在过，就把旧细节重新加回来。
