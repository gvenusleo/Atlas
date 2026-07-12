# 通道

[English](../channels.md)

## ACP

`atlas acp` 通过 stdin/stdout 启动 [Agent Client Protocol](https://agentclientprotocol.com/) 服务，供 Zed 等 ACP Client 连接。

在 Zed 中添加 Atlas：

```json
"agent_servers": {
  "Atlas": {
    "type": "custom",
    "command": "~/.local/bin/atlas",
    "args": ["acp"]
  }
}
```

ACP 支持的功能：

- session 创建、恢复、加载历史回放、列表分页、删除
- prompt、取消、关闭
- 模型切换、思考强度切换、思维链流式更新
- embedded text resource
- session info 和 usage update
- 客户端 terminal 展示 `run_shell` 输出
- `apply_patch` locations/diff 展示
- 图片输入
- 长期记忆后台 worker
- `/compact` slash command
- skill slash command，例如 `/think ...`
- 计划更新：`todo_write` 工具调用映射为 `plan_update` session update，在编辑器中渲染为结构化计划面板

通过 ACP 连接时，`run_shell` 优先请求客户端 terminal 执行并嵌入输出；terminal 不可用时回退到 Atlas 本地 shell。`apply_patch` 始终修改 Atlas 进程可见的文件系统，并向客户端发送 locations/diff。

`additionalDirectories` 会作为 session 元数据保存和返回，但相对路径仍以 `cwd` 为基准。当前不支持 ACP auth、权限请求、MCP 连接，也不支持音频和非图片二进制资源输入。

## WebSocket

`atlas serve` 启动 WebSocket 服务，供手机 App 等客户端连接。默认监听 `127.0.0.1:8765`，可通过 `~/.atlas/config.json` 中的 `services.ws.host` 和 `services.ws.port` 配置。

loopback 连接无需认证。绑定 `0.0.0.0` 等非 loopback 地址时必须配置 `services.ws.token`，客户端通过 `Authorization: Bearer <token>` 请求头发送；WebSocket 握手同时执行同源校验。该通道暴露与其他通道相同的 Agent 能力：

- 支持文本和图片输入的对话
- 流式事件：模型增量、推理增量、工具调用
- 会话列表、详情、删除、压缩
- 模型切换和列表
- 思考强度切换
- Skill 摘要
- Turn 取消
- 长期记忆后台 worker

### 多会话并发

单个 WebSocket 连接支持多个会话同时进行。每个会话维护独立的工作目录、所选模型和 turn 状态。不同会话的 turn 可以并行执行；同一会话在 turn 运行时拒绝第二个 prompt，包括来自另一条连接的 prompt。

所有消息通过 `session_id` 路由：

- `prompt` 带 `session_id` — 在该会话上下文中执行。不带 `session_id` 时创建新会话，分配的 ID 通过 `turn_finished` 回传。
- `cancel` — 必须带 `session_id`，仅取消该会话的 turn，不影响其他会话。
- `set_model` — 必须带 `session_id`，仅为该会话设置模型。
- 所有流式事件（`turn_started`、`model_delta`、`tool_started` 等）都携带 `session_id`，客户端据此将事件路由到正确的会话 UI。
