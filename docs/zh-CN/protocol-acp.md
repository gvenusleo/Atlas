# ACP 协议

Atlas 通过 `packages/atlas_acp` 实现 ACP v1。Flutter 应用始终作为 ACP
客户端运行，包括连接进程内托管的 Atlas runtime 时。

## 已支持范围

- 会话创建、提示、取消、加载、恢复、列表、关闭、删除和配置选项。
- 文本、图片、嵌入文本资源和资源链接提示块。
- 消息、推理、工具、计划、命令、会话信息和用量更新。
- Atlas 不发起权限请求。工具使用 Atlas 进程权限运行；客户端不应等待审批往返。

## Atlas 扩展

Atlas 扩展使用 `_atlas.dev` 命名空间，并在
`agentCapabilities._meta['atlas.dev']` 中声明。

- `_atlas.dev/session/set_title` 重命名持久化 Atlas 会话。
- `compact` 表示支持 Atlas 上下文压缩。
- `permissionModel: none` 表示 Atlas Agent 不发送
  `session/request_permission`。

Atlas ACP 客户端仍会处理第三方 Agent 发出的权限请求。Agent 与客户端的权限行为是
两个不同的协议角色。

运行时会话契约为 `AgentSession`；ACP 专属的标题、命令和模式通过
`PresentationAgentSession` 暴露。

## Planned

客户端文件系统与终端能力，以及 ACP v2 支持仍为 Planned。
