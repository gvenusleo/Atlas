# ACP 协议

[English](../protocol-acp.md)

Atlas 在 `packages/atlas_acp` 中实现 ACP v1。Flutter 应用始终作为 ACP 客户端运行，包括连接进程内托管的 runtime 时。

## 已支持范围

- 会话创建、提示、取消、加载、恢复、列表、关闭、删除与配置选项，以及文本、图片、嵌入文本资源与资源链接提示块。
- 消息、推理、工具、计划、命令、会话信息与用量更新。
- shell 过程输出使用标准 `tool_call_update` 文本内容，状态为 `in_progress`。每个快照替换先前内容，最终结果标记 `completed` 或 `failed`；即使失败或历史回放，`rawOutput` 仍保留捕获文本与最终 metadata，无需终端扩展。
- 部分更新省略工具内容时，ACP 客户端保留原内容（包括仅更新完成状态与历史回放）；显式空内容会清空。
- Atlas 不发起权限请求：工具使用 Atlas 进程权限运行，客户端不应等待审批往返。

## Atlas 扩展

Atlas 扩展使用 `_atlas.dev` 命名空间，在 `agentCapabilities._meta['atlas.dev']` 中声明：

- `_atlas.dev/session/set_title` 重命名持久化会话。
- `compact` 表示支持 Atlas 上下文压缩。
- `permissionModel: none` 表示 Atlas Agent 不发送 `session/request_permission`。

Atlas ACP 客户端仍会处理第三方 Agent 发出的权限请求，Agent 与客户端的权限行为是两个不同的协议角色。运行时会话契约为 `AgentSession`，ACP 专属的标题、命令与模式通过 `PresentationAgentSession` 暴露。

## WebSocket transport

`atlas server` 通过 `atlas_ws` 为 Atlas 移动客户端暴露同一套 ACP 接口：

- 请求携带 `Authorization: Bearer <token>` 时 `GET /acp` 完成 upgrade；token 由 `atlas server` 签发，存于 `~/.atlas/remote_token`，权限 0600。
- 每个 WebSocket text frame 恰好承载一条 ACP JSON-RPC 消息（双向皆可）；binary 或超大 frame 会关闭连接。
- 生命周期、会话、事件、权限请求与 `_atlas.dev` 扩展的行为与 stdio 一致，服务端在共享 runtime 上为每个连接运行一个 `AcpServer`。
- socket 断开不会取消进行中的 turn：runtime 执行完毕并持久化，客户端以指数退避重连后通过 `session/load` 校准现场。

## Planned

客户端文件系统与终端能力，以及 ACP v2 支持。

## 主机配置的 MCP 工具

主机配置的 MCP 工具复用 `tool_call` 与 `tool_call_update`，包括进度与历史恢复；没有内置工具 kind 时保留通用 ACP 工具标题。会话传入的 `mcpServers` 被显式拒绝：主机配置属于整个 runtime，尚未实现 ACP 会话隔离。详见 [MCP 工具](mcp.md)。
