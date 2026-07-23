# 通道

[English](../channels.md)

## 终端界面

不带子命令运行 `atlas`，会在当前工作目录启动全屏终端界面：

```sh
atlas
atlas --session <id>
```

可选的 `--session` 参数会加载已有对话记录；如果 session 不存在，则在第一轮创建。界面会流式显示模型输出并在终端中渲染 Markdown，按发生顺序展示工具调用和结果，支持粘贴多行输入，并在恢复 session 时加载已保存的历史。turn 或手动压缩执行期间，输入框上方会临时显示状态与耗时：收到流式 reasoning 时显示 `Thinking`，模型回复、执行工具或压缩上下文时显示 `Working`。底栏显示当前模型、思考深度，以及最近一次上下文用量占 `context_window` 的百分比。

按键：

- 没有激活命令候选项时，`Enter` 发送当前输入。
- `Shift+Enter` 插入换行；如果终端无法区分 `Shift+Enter` 和 `Enter`，可使用 `Ctrl+J`。
- 在输入开头键入 `/` 可查看支持的命令和当前可用的 skills；使用方向键选择候选项，再按 `Tab` 或 `Enter` 补全。
- 在 token 边界输入 `@` 可浏览当前工作目录下的文件，继续输入则会模糊搜索文件路径。使用方向键选择候选项，再按 `Tab` 或 `Enter` 插入相对路径；按 `Esc` 关闭选择器。
- 输入 `/model` 选择已配置的模型及其思考深度；使用方向键和 `Enter` 选择。
- 输入 `/resume` 选择已保存的 session。直接输入文本可按标题、ID 或工作目录搜索；使用左右方向键切换当前目录与全部 session，再按 `Enter` 恢复。`/resume <session-id>` 可按精确 ID 直接恢复。
- 输入 `/compact [instruction]` 总结早期上下文并保留最近消息；可选指令用于指定压缩时需要重点保留的内容。
- 输入 `/quit` 退出 TUI。
- `Page Up`、`Page Down` 和鼠标滚轮用于滚动对话历史。
- 在对话文本上拖动鼠标即可选择并复制到剪贴板。
- `Esc` 中断正在执行的 turn 或手动压缩；在 Resume 选择器中用于返回当前会话，其他空闲状态下不起作用。
- `Ctrl+C` 不起作用。

Resume 选择器默认列出保存在当前工作目录下的 session，也可切换到全部 session。恢复其他目录下的 session 时需要确认；确认后，后续 instructions、skills 和工具会使用该 session 保存的工作目录。由于 session 不持久化模型设置，当前 TUI 选择的模型和思考深度会保持不变。

TUI 启动时使用 `default_model` 和 `reasoning_efforts` 中的第一项。通过 `/model` 做出的选择会应用于当前 TUI 进程中的后续 turn，但不会改写配置文件，也不会跨重启保存。手动压缩会保留完整 transcript，只改写后续 turn 使用的已保存上下文摘要；压缩命令和结果提示不会作为对话消息持久化。输入 `/think plan this change` 之类的可用 skill 命令时，TUI 会只为当前 turn 注入对应 skill，并在 transcript 中保留原始输入。TUI 暂不支持图片输入。

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
- 图片输入
- `/compact [instruction]` 斜杠命令；可选指令用于指定压缩时需要重点保留的内容
- skill slash command，例如 `/think ...`
- 计划更新：`todo_write` 工具调用映射为 `plan_update` session update，在编辑器中渲染为结构化计划面板

通过 ACP 连接时，`run_shell` 优先请求客户端 terminal 执行并嵌入输出。ACP terminal 不支持标准输入，因此带有非空 `stdin` 的调用通过 Atlas 本地 shell 执行；terminal 不可用时也会回退到本地 shell。

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

### 多会话并发

单个 WebSocket 连接支持多个会话同时进行。每个会话维护独立的工作目录、所选模型和 turn 状态。不同会话的 turn 可以并行执行；同一会话在 turn 运行时拒绝第二个 prompt，包括来自另一条连接的 prompt。

所有消息通过 `session_id` 路由：

- `prompt` 带 `session_id`：在该会话上下文中执行。不带 `session_id` 时创建新会话，分配的 ID 通过 `turn_finished` 回传。
- `cancel`：必须带 `session_id`，仅取消该会话的 turn，不影响其他会话。
- `set_model`：必须带 `session_id`，仅为该会话设置模型。
- 所有流式事件（`turn_started`、`model_delta`、`tool_started` 等）都携带 `session_id`，客户端据此将事件路由到正确的会话 UI。
