# Atlas 开发指南

## 项目定位

Atlas 是用 Go 编写的本地 coding agent。核心是可测试的 headless agent loop，CLI、ACP、微信通道都通过 `internal/runtime` 调用同一套能力。

一次 turn 的核心职责：

- 追加用户输入到 transcript。
- 用系统提示词、历史消息和工具定义调用模型 Provider。
- 按模型返回顺序执行工具调用，并把结果写回 transcript。
- 没有工具调用、遇到错误或达到 `max_steps` 时结束。
- 保存 transcript、用量、上下文压缩和记忆相关元数据。

## 协作原则

用户主要负责写代码。助手负责梳理架构、审查设计、说明权衡，并指出具体文件、接口和测试路径。用户明确要求实现时，可以直接改代码。

提出方案前先说明假设。遇到歧义时列出可能解释。每次改动都选择最小、可验证的切片，不做顺手重构。

## 本地访问模型

Atlas 工具拥有 Atlas 进程本身拥有的本地权限，可以读写文件、搜索文本并执行 shell 命令。Atlas 不提供沙箱、权限提示或 approval gate，这是产品边界，不是缺失实现。除非产品方向变化，不要在代码里引入权限抽象。

ACP 客户端声明 terminal capability 时，`run_shell` 可以请求客户端 terminal 执行并嵌入输出。客户端声明 filesystem capability 时，文件工具可以请求客户端文件系统能力并展示 locations/diff。客户端不支持或调用失败时，回退到 Atlas 本地工具。

ACP 文件工具调用客户端文本文件接口前，只传递本地确认的普通 UTF-8 文本文件；目录、特殊文件和二进制内容回退到本地工具。

README、CLI 文案和测试都要反映这个边界。测试只验证工具行为、回退和错误传播，不验证权限提示。

## 配置和提示词

Atlas 从 `~/.atlas/config.json` 读取配置。Provider 配置包括 `base_url`、`api_key`、`default_model` 和 `models`。模型项至少包含 `value`、`name`、`context_window`、`max_tokens`；`default_model` 必须匹配某个 `value`。

`base_url`、`api_key`、`default_model` 和模型 `value` 只属于 Provider 连接配置，不进入 `model.ChatRequest`。`agent` 包只依赖 `model.Provider` 和通用生成参数。

长期记忆配置包括 `memory.enabled` 和 `memory.model`。`memory.enabled` 未配置时默认启用；启用但 `memory.model` 为空时，后台记忆任务使用产生该会话的模型；配置时必须匹配 Provider 模型列表。

系统提示词由 `internal/prompt` 构造，只说明模型行为、工具使用原则、验证习惯和回复风格，不重复工具 JSON schema。

Atlas 只加载两个附加指令文件：

- `~/.atlas/AGENTS.md`
- 当前工作目录下的 `AGENTS.md`

当前用户请求优先于指令文件，当前目录指令优先于全局指令。Atlas 不递归查找父目录或子目录中的 `AGENTS.md`。

Atlas 会扫描用户级和当前目录级 skill，但系统提示词只注入 `name` 和 `description`。完整 `SKILL.md` 只能通过 `load_skill` 工具按需加载。

## 包结构

推荐包边界：

```text
cmd/atlas
internal/acp
internal/agent
internal/compact
internal/config
internal/memory
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
CLI / ACP / Weixin
  -> runtime.RunTurn
  -> agent.RunTurn
  -> provider.Stream
  -> tool.Registry
  -> transcript + session store
```

`internal/acp` 只做 ACP 协议适配、session/update 通知、客户端 terminal/filesystem 桥接、embedded text resource 解析和 session 状态管理，不复制 agent loop。

`internal/weixin` 只做 iLink Bot 登录、消息轮询、typing、斜杠命令、回复发送和用户到 session 的轻量绑定，不复制 agent loop。

## 核心边界

**Provider**，`model.Provider` 是模型后端唯一接口。Provider 负责连接信息、鉴权、模型名、请求格式、SSE 解析和响应格式转换。`model.ChatRequest` 只表达 Atlas 的通用聊天协议。

**Tool**，工具通过 `tool.Tool` 注册：`Definition()` 返回模型可见定义，`Run(ctx, arguments)` 解析模型给出的 JSON 参数并返回文本结果。工具尽量无状态，注册表负责排序、重复检查和按名称执行。

**Transcript**，`internal/transcript` 只保存当前 agent 实例中的消息序列。读取时返回副本，不负责持久化、压缩或摘要。

**Session**，`internal/session` 用 SQLite 保存 transcript、session 元数据、上下文压缩元数据和记忆抽取边界。默认路径是 `~/.atlas/atlas.db`，可通过 `session.db_path` 覆盖。当前不实现迁移框架；早期 schema 变化由用户删除旧数据库重建。

**Memory**，`internal/memory` 维护长期记忆条目、摘要、FTS 检索和后台任务队列。长期记忆与 session 共用 SQLite 数据库，但使用独立表。运行时按增量阈值、明确记忆指令或上下文压缩触发抽取任务；worker 只处理上次边界后的新增消息，并刷新受影响作用域的摘要。记忆类型保持三种：`instruction`、`fact`、`workflow`。

## 运行约束

保持 agent loop 可预测：

- 一个 turn 内包含编号 step。
- 每个 tool call 都有配对的 tool result。
- tool result 顺序与模型返回顺序一致。
- 工具错误作为模型可见的 tool result 写回 transcript。
- 公共接口接收 `context.Context`。
- observer 事件按发生顺序发送，CLI、ACP 和后续 UI 按这个顺序渲染。

## 取舍原则

Atlas 追求小、清楚、可验证。当前真实使用路径优先于理论完整性。

- 不提前做兼容矩阵。遇到真实 provider 差异后，用测试固定行为，再做局部适配。
- 不为“可能以后”保留两套接口、fallback 或配置开关。
- 两个真实调用点出现前不抽象。
- 不把上游大型 agent 的完整架构搬进来，只吸收能让当前核心更小、更清楚的做法。

## 测试标准

关键行为使用 fake Provider 和临时目录测试。优先覆盖这些面：

- agent loop：纯文本回复、工具调用、错误回写、顺序、`max_steps`。
- 工具：文件读写、文本搜索、shell 成功和失败、Tavily 工具。
- prompt：全局和当前目录指令、skill 摘要、长期记忆注入。
- session：保存、恢复、列表、查看、删除、额外工作目录、上下文压缩。
- memory：schema、检索、入队、增量抽取、摘要刷新、禁用和模型选择。
- ACP：初始化、session 生命周期、历史回放、取消、列表分页、usage、terminal/filesystem capability、`/compact`。
- 微信：扫码登录、账号保存、typing、工作目录切换、会话列表、恢复、压缩、取消。

交付前运行：

```sh
go test ./...
```

## 代码风格

- 保持代码小而明确。
- 优先使用 Go 标准库。
- 只修改与当前任务直接相关的代码。
- 注释必要、简洁，符合 Go 规范。
- 导出类型、函数和包需要规范注释。
- 测试、文档和注释服务行为理解，不堆砌说明。
- 不因为旧 Atlas 实现存在过，就把旧细节重新加回来。
