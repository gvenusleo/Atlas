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
- 文件工具 locations/diff 展示
- 图片输入
- 长期记忆后台 worker
- `/compact` slash command
- skill slash command，例如 `/think ...`
- 计划更新：`todo_write` 工具调用映射为 `plan_update` session update，在编辑器中渲染为结构化计划面板

通过 ACP 连接时，Atlas 会优先使用客户端声明的能力：

- **terminal capability**：`run_shell` 请求客户端 terminal 执行，并嵌入输出
- **filesystem capability**：文件工具请求客户端读写文件，并展示 locations/diff

客户端不支持或调用失败时，Atlas 回退到本地工具执行。

`additionalDirectories` 会作为 session 元数据保存和返回，但相对路径仍以 `cwd` 为基准。当前不支持 ACP auth、权限请求、MCP 连接，也不支持音频和非图片二进制资源输入。

## 微信

`atlas weixin login` 使用微信扫码登录，并把账号 token 保存到 `~/.atlas/weixin/accounts`。`atlas weixin serve` 连接微信 Bot，长轮询文本和图片消息并调用本地 Atlas runtime。

当模型通过 `todo_write` 更新任务列表时，`in_progress` 状态的条目会作为进度消息发送给用户。

微信通道拥有与本机 Atlas 进程相同的文件和 shell 权限。首次收到消息时，工作目录使用 `atlas weixin serve` 启动时的当前目录。当前只支持扫码登录的微信用户本人控制 Atlas，不支持群聊、音频、视频或添加其他控制人。

微信聊天支持的斜杠命令：

| 命令 | 说明 |
|---|---|
| `/help` | 查看命令 |
| `/status` | 查看当前工作目录和 session |
| `/cwd` | 查看当前工作目录 |
| `/cwd /absolute/path` | 切换工作目录，下一条普通消息开启新对话 |
| `/cwd -` | 切回上一个工作目录 |
| `/new` | 在当前工作目录开启新对话 |
| `/sessions` | 查看当前工作目录最近会话 |
| `/sessions all` | 查看全局最近会话 |
| `/resume <session-id>` | 恢复指定会话，并切换到该会话的工作目录 |
| `/compact` | 压缩当前会话上下文 |
| `/cancel` | 取消当前正在运行的 turn |
