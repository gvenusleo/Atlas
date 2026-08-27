# 配置

[English](configuration.md)

Atlas 通过 `atlas_config` 包从 `~/.atlas/config.yaml` 加载应用配置。
`atlas_cli` 与 `atlas_flutter` 都会定位该文件并交给 `loadConfig`；
`atlas_composition` 再把结果组装成一个 runtime。该包负责解析、校验并映射到
provider 配置对象。

## 示例

```yaml
default_model: anthropic/claude-sonnet

providers:
  - name: anthropic
    type: anthropic
    base_url: https://api.anthropic.com
    api_key: ${ANTHROPIC_API_KEY}
    api_version: "2023-06-01"        # 可选，默认 2023-06-01
    models:
      - value: claude-sonnet
        name: Claude Sonnet           # 可选
        description: ...              # 可选
        context_window: 200000
        max_tokens: 4096
        reasoning_efforts:            # 可选
          - value: high
            name: High                # 可选
        thinking_budget_tokens: 2048  # 可选，默认 0（关闭 thinking）

  - name: openai
    type: responses                 # chat_completions | responses | anthropic
    base_url: https://api.openai.com/v1
    api_key: ${OPENAI_API_KEY}
    user_agent: Atlas                 # 可选
    models:
      - value: gpt-4o
        context_window: 128000
        max_tokens: 4096
        input_capabilities: [text, image]  # 可选，默认 [text]
        prompt_cache: true            # 可选，默认 false

agent:
  max_steps: 20                       # 可选，默认 20
  max_output_tokens: 0                # 可选，使用 provider 默认值
  temperature: 0.7                    # 可选
  compaction:
    threshold: 0.8                    # 可选，默认 0.8

session:
  db_path: ~/.atlas/atlas.db         # 可选，~ 展开为用户主目录

logging:
  level: info                         # debug | info | warn | error
  directory: ~/.atlas/logs            # 可选；省略则关闭文件日志
  retain_days: 7                      # 可选，保留的每日文件数
```

## 规则

- `default_model` 格式为 `"<provider>/<model>"`，必须引用已配置的 provider 与模型。
- `providers` 不能为空；每个 provider 的 `models` 列表也不能为空。provider
  名称必须唯一；provider 内模型 id 必须唯一。
- `type` 为 `chat_completions`、`responses` 或 `anthropic`。前两者选择
  OpenAI-compatible 适配器并使用对应的 API；`anthropic` 选择 Anthropic 适配器。
  OpenAI-compatible 与 Anthropic provider 都支持可选的 `user_agent`。
- `base_url` 必须是 HTTP(S) URL，且不含 query 与 fragment。
- `api_key` 支持 `${ENV_VAR}` 引用；未定义的变量会导致加载失败，错误消息
  中带有变量名。
- `max_tokens`、`context_window`、`thinking_budget_tokens` 不能为负；
  `max_steps` 必须大于 0。
- Anthropic 的 `thinking_budget_tokens` 必须小于最终生效的 `max_tokens`。
  启用 thinking 时，Atlas 会省略 Anthropic 请求中的 `agent.temperature`，
  因为该采样选项不兼容。`input_capabilities` 与 `reasoning_efforts` 适用于
  所有 provider 类型。
- `agent.compaction.threshold` 必须大于 0 且不超过 1；它是触发 turn 结束后
  自动压缩的上下文窗口比例。
- 校验失败抛出 `ConfigLoadException`，消息包含字段路径，例如
  `providers[0].base_url`。
- 配置 `logging.directory` 后会启用脱敏 JSON Lines 文件日志。未配置
  `logging.level` 时可使用 `ATLAS_LOG_LEVEL`；显式配置优先。
