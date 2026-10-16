# Atlas

Atlas 是用 Go 编写的桌面 AI Agent，当前处于重写阶段。Agent 拥有完整的文件与命令权限，
不提供权限确认。

## 开发

使用 [mise](https://mise.jdx.dev) 管理工具链与任务：

```sh
mise run dev       # 启动桌面应用
mise run ci        # 格式检查、vet、测试、编译、漏洞扫描
```

## 配置

配置与数据位于 `~/.atlas/`：

- `settings.json`：当前模型与 provider 清单，不含凭据。
- `auth.json`：以 provider 名为键的 API key，权限必须为 `0600`；也可使用环境变量
  `<PROVIDER>_API_KEY`。
- `AGENTS.md`：全局指令。
- `atlas.db`：会话数据库。

最小的 `settings.json`：

```json
{
  "model": "provider/model-id",
  "providers": {
    "provider": {
      "api": "openai-responses",
      "base_url": "https://api.example.com/v1",
      "models": {
        "model-id": {
          "context_window": 200000,
          "reasoning": true,
          "reasoning_options": [{ "type": "effort", "values": ["low", "high"] }]
        }
      }
    }
  }
}
```

模型的 `context_window`（上下文窗口，token）可选，但建议填写：上下文接近窗口上限时，
Atlas 会自动把较早的对话压缩成摘要，界面里的上下文占用圆环也依赖它。没有填写时只在
请求被拒绝为上下文超限之后才压缩。在输入框里输入 `/` 会列出可用的命令，其中 `/compact`
可以随时手动压缩：除最后一条消息外的对话都会换成摘要，与有没有填写 `context_window` 无关。

## 结构

| 包 | 职责 |
| --- | --- |
| `internal/agent` | 消息模型与 agent 循环，不依赖其他层 |
| `internal/llm` | 把 `agent.StreamFn` 落到具体的模型 API |
| `internal/tools` | read、write、edit、shell 工具 |
| `internal/prompt`、`internal/skills` | 系统提示词、指令与技能发现 |
| `internal/config` | `settings.json` 与凭据读取 |
| `internal/session` | SQLite 会话存储 |
| `internal/host` | 组装以上各层，供 App、ACP 等入口使用 |
