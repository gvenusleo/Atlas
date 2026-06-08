# Atlas 开发指南

## 项目定位

Atlas 是一个用 Go 编写的本地 coding agent。核心是可测试的 headless agent loop；CLI、TUI 或其他界面都是调用核心能力的外壳。

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
- `model`

这些字段属于具体 Provider 的连接配置，不进入 `model.ChatRequest`，也不由 `agent` 包直接关心。`agent` 只依赖 `model.Provider` 接口。

## 系统提示词

系统提示词由 `internal/prompt` 构造，用来定义 Atlas 的模型行为契约。它描述身份、本地访问模型、代码修改纪律、工具使用原则、验证习惯和回复风格。

工具 JSON schema 由 `tool.Registry` 发送给 Provider。系统提示词只说明工具使用原则，不重复 schema。

Atlas 只加载两个附加指令文件：

- `~/.atlas/AGENTS.md`
- 当前工作目录下的 `AGENTS.md`

当前用户请求优先于指令文件，当前目录指令优先于全局指令。Atlas 不递归查找父目录或子目录中的 `AGENTS.md`。

## 架构边界

推荐包结构：

```text
cmd/atlas
internal/agent
internal/config
internal/model
internal/prompt
internal/provider/openai
internal/tool
internal/transcript
```

核心流程：

```text
CLI input
  -> Agent.RunTurn
  -> build model.ChatRequest from transcript
  -> provider.Stream
  -> emit model text deltas
  -> append assistant message
  -> run requested tools
  -> append tool results
  -> repeat until final response or max steps
```

保持 loop 可预测：

- 一个 turn 内包含编号 step。
- 每个模型 tool call 都有一个配对的 tool result。
- tool result 顺序与模型返回的 tool call 顺序一致。
- 工具执行错误作为模型可见的 tool result 写回 transcript。
- `max_steps` 限制一次 turn 的模型调用次数。
- 公共接口接收 `context.Context`，用于 cancellation。
- agent loop 暴露最小 observer 事件，供 CLI、测试和后续 UI 观察执行过程。

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

Transcript 不负责持久化、压缩或摘要。需要这些能力时，通过新的明确边界设计，不把它们混进基础消息容器。

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
