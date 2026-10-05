# 配置

[English](../configuration.md)

Atlas 在 `~/.atlas` 中使用 Pi 风格的 JSON 配置。CLI、TUI、ACP、WebSocket server 和 Flutter 本地模式加载同一组文件。Flutter 远程客户端使用宿主的模型配置。

| 文件 | 职责 |
| --- | --- |
| `settings.json` | 默认 provider、模型、思考档位，以及 agent、压缩、会话存储和日志设置 |
| `models.json` | 自定义 provider、模型定义和内置模型覆盖 |
| `auth.json` | 按 provider ID 保存的 API key |
| `mcp.json` | 宿主 MCP server，见 [MCP 工具](mcp.md) |
| `cache/model-catalog.json` | 可重建的 models.dev 缓存 |
| `atlas.db` | 会话、turn、用量、消息和 provider continuation |

JSON 文件接受 `//` 和 `/* */` 注释，与 Pi 的模型加载器一致；不接受尾随逗号。缺失文件使用默认值，格式错误和不支持的字段会报告文件或字段路径。旧的 `config.yaml` 若仍存在会被拒绝，不会自动迁移或删除用户文件。

## 设置

```json
{
  "defaultProvider": "relay",
  "defaultModel": "gpt-4o",
  "defaultThinkingLevel": "high",
  "agent": {
    "maxSteps": 20,
    "maxOutputTokens": 4096,
    "temperature": 0.7
  },
  "compaction": {
    "threshold": 0.8,
    "keepRecentTokens": 20000,
    "reserveTokens": 16384
  },
  "session": {
    "dbPath": "~/.atlas/atlas.db"
  },
  "logging": {
    "level": "info",
    "directory": "~/.atlas/logs",
    "retainDays": 7
  }
}
```

所有设置都可省略。`defaultProvider` 默认为 `openai`；省略 `defaultModel` 时，选择该 provider 在解析后目录中的第一个模型。显式设置两者可固定启动选择。所选模型必须存在，但缺少凭据不会阻止配置加载。`defaultThinkingLevel` 接受 `off`、`minimal`、`low`、`medium`、`high`、`xhigh`、`max`，每个模型只展示它支持的选项，不支持的启动偏好会调整到可用档位。

`agent.maxSteps` 默认为 20。`maxOutputTokens` 默认为 0，此时 adapter 使用最多 4096 token 的输出预算，并受模型 `maxTokens` 限制；显式正数预算超过模型上限时，在发送 HTTP 请求前报错。默认不发送 `temperature`，开启 OpenAI reasoning 或 Anthropic thinking 时也不发送该设置。

`keepRecentTokens` 和 `reserveTokens` 必须为正数。压缩阈值为 `contextWindow - reserveTokens`；预留量大于等于上下文窗口时，回退到 `threshold` 比例，取值必须大于 0 且不超过 1。会话和日志路径支持开头的 `~/`。省略 `logging.directory` 时禁用文件日志；未显式配置日志级别时使用 `ATLAS_LOG_LEVEL`。

## 第三方中转站与本地端点

Provider ID 标识一个服务，可以是中转站或本地服务，不必存在于 models.dev，也没有额外的 connection 层级。

```json
{
  "providers": {
    "relay": {
      "baseUrl": "https://relay.example.com/v1",
      "api": "openai-completions",
      "apiKey": "${RELAY_API_KEY}",
      "headers": {
        "X-Account": "team"
      },
      "models": [
        {
          "id": "gpt-4o",
          "name": "GPT-4o",
          "input": ["text", "image"],
          "contextWindow": 128000,
          "maxTokens": 16384
        },
        {
          "id": "claude-sonnet-4-5",
          "api": "anthropic-messages",
          "baseUrl": "https://relay.example.com/v1",
          "contextWindow": 200000,
          "maxTokens": 64000
        }
      ]
    },
    "ollama": {
      "baseUrl": "http://localhost:11434/v1",
      "api": "openai-completions",
      "apiKey": "ollama",
      "models": [{ "id": "qwen2.5-coder:7b" }]
    }
  }
}
```

支持的 `api` 为 `openai-completions`（Chat Completions）、`openai-responses` 和 `anthropic-messages`。模型级 `api` 与 `baseUrl` 覆盖 provider 默认值，同一个中转站可以使用多种协议。三种协议的 `baseUrl` 都表示包含版本前缀的 API 基础地址。官方 OpenAI 和 Anthropic 预设分别为 `https://api.openai.com/v1`、`https://api.anthropic.com/v1`。适配器只追加 `/chat/completions`、`/responses` 或 `/messages`，保留自定义代理前缀。按之前根地址约定编写的 Anthropic 配置，需要在端点要求时补上 `/v1`。模型 ID 原样发送，允许包含 `/`。本地服务可以像 Pi 一样使用占位 key。

自定义模型需要 `id` 和可解析的 API、地址。与 Pi 一样，省略的元数据默认为：显示名称使用 ID、文本输入、不启用 reasoning、上下文 128000 token、最大输出 16384 token。这些回退值不是对端点的测量结果，实际限制不同时应显式配置。同一 provider 内模型 ID 必须唯一；provider ID 不允许包含 `/`。

## 内置模型与覆盖

OpenAI 和 Anthropic 有内置协议、地址预设以及随包分发的 models.dev 目录。通过 `OPENAI_API_KEY` 或 `ANTHROPIC_API_KEY` 使用它们时，不需要创建 `models.json`。可覆盖已有 provider，将其模型请求转到兼容代理：

```json
{
  "providers": {
    "openai": {
      "baseUrl": "https://relay.example.com/v1",
      "apiKey": "${RELAY_API_KEY}",
      "modelOverrides": {
        "gpt-4o": {
          "contextWindow": 64000,
          "cost": { "input": 1.0 }
        }
      }
    }
  }
}
```

`models` 新增或替换完整模型定义；替换时可继承已有 API、地址，其他未指定的元数据使用自定义模型默认值。`modelOverrides` 只合并显式提供的字段，包括嵌套的价格、兼容选项和采样设置。未知模型 ID 的 override 与 Pi 一样被忽略。Override 不修改 `api` 或 `baseUrl`，这些变更使用 `models` 定义。Provider 级 `api` 为 `models` 定义提供默认值，不改写内置模型列表。

模型字段为 `id`、`name`、`api`、`baseUrl`、`reasoning`、`thinkingLevelMap`、`input`、`cost`、`contextWindow`、`maxTokens`、`samplingParams`、`samplingParamsByThinkingLevel`、`headers`、`compat`。Provider 字段为 `name`、`baseUrl`、`apiKey`、`api`、`headers`、`compat`、`authHeader`、`models`、`modelOverrides`。Headers 按不区分大小写的名称合并，模型级值优先。传输层专用 header 和包含换行的值会被拒绝。

`cost` 保留每百万 token 的美元价格，包括 `input`、`output`、`cacheRead`、`cacheWrite`，以及含 `inputTokensAbove` 的可选 `tiers`。缺失价格保持未知。费用展示和历史金额核算为 Planned；token 用量继续写入 SQLite。

## 认证

```sh
atlas auth set relay
atlas auth remove relay
```

`set` 从终端读取 key 时关闭回显，管道模式下读取标准输入。`auth.json` 使用 Pi 的 `{ "relay": { "type": "api_key", "key": "..." } }` 结构。写入在进程内串行、跨进程加锁，保留其他 provider，并原子替换文件。POSIX 文件权限为 `0600`，Windows 使用继承的文件系统 ACL。这不提供加密存储或系统钥匙串集成。

凭据优先级为运行时覆盖、`auth.json` 保存的 key、配置中的 `apiKey`、内置 provider 的环境变量。每次请求重新解析 key 和自定义 headers，因此修改保存的 key 无须重启。`authHeader: true` 额外要求解析出 key，并设置 `Authorization: Bearer ...`；设为 false 不会关闭 API adapter 自身的正常认证。

Key 和 header 值支持字面值、`$NAME`、`${NAME}` 和开头的 `!command`。非命令值中的 `$$` 转义为 `$`，`$!` 转义为 `!`。命令使用宿主 shell：POSIX 为 `/bin/sh`，Windows 为 `cmd.exe`；结果不缓存，执行限制为 10 秒、输出限制为 64 KiB。命令失败和变量缺失返回脱敏错误。列举模型和校验配置不会执行凭据命令。

## 兼容选项

Atlas 接受已实现的 Pi `compat` 字段子集。未知字段、不支持的 API 类型和 thinking 格式会明确报错。

| API | 已实现字段 |
| --- | --- |
| `openai-completions` | `supportsStore`、`supportsDeveloperRole`、`supportsReasoningEffort`、`supportsUsageInStreaming`、`supportsFinishReason`、`maxTokensField`、`requiresToolResultName`、`requiresAssistantAfterToolResult`、`requiresThinkingAsText`、`requiresReasoningContentOnAssistantMessages`、`thinkingFormat`、`supportsStrictMode`、`sendSessionAffinityHeaders`、`sessionAffinityFormat` |
| `openai-responses` | `supportsDeveloperRole`、`supportsMaxOutputTokens`、`supportsStrictMode`、`sessionAffinityFormat` |
| `anthropic-messages` | `supportsCacheControlOnTools`、`supportsTemperature`、`forceAdaptiveThinking`、`allowEmptySignature`、`supportsStrictTools`、`sendSessionAffinityHeaders` |

`maxTokensField` 为 `max_completion_tokens` 或 `max_tokens`。`thinkingFormat` 当前支持 `openai` 与 `deepseek`。`sessionAffinityFormat` 支持 `openai`、`openai-nosession`、`openrouter`。兼容选项应对应已经验证的端点行为。

`thinkingLevelMap` 将 Atlas/Pi 档位名称映射到 provider effort 字符串，null 表示省略 effort 字段。OpenAI 采样默认值支持数值型的 `temperature`、`top_p`、`top_k`、`min_p`、`frequency_penalty`、`presence_penalty`、`repetition_penalty`、`seed`；档位专属值覆盖模型默认值，未选择思考档位时使用 `off` 采样配置。这些映射不能替换模型 ID、消息、工具或 streaming 字段。Anthropic 支持 token budget thinking 和显式配置的 adaptive thinking，不接受 OpenAI 采样映射。手动 thinking 至少需要 1024 个思考 token，并为正文保留至少 1024 个 token，因此开启思考且输出预算不足 2048 时，会在发送 HTTP 前报错。较大的思考预算会被调低，以保留正文空间。支持 effort 不代表支持 adaptive thinking：Opus 4.5 使用带 effort 的手动 thinking，经过验证的 adaptive 模型才使用 `type: adaptive`。

`supportsStrictTools` 只对符合要求的 schema 启用 strict 采样。Atlas 检查嵌套 schema；开放对象、不支持的约束、引用和其他未验证的 schema 特性继续使用普通工具调用，原始工具 schema 保持不变。

OAuth、provider 扩展、其他 Pi API，以及其余 Pi 兼容与输入限制选项为 Planned。Atlas 不宣称完整兼容全部 Pi 配置。

## 模型目录与命令

```sh
atlas config validate
atlas models list
atlas models refresh
```

`validate` 校验 settings、models 和 MCP，不连接模型或 MCP server。`list` 输出模型引用、上下文和输出上限，也包括尚未提供凭据的模型。`refresh` 通过 Dio 下载 `https://models.dev/api.json`，校验受支持的 provider 后原子替换缓存。修改模型配置或刷新目录后，需要重启正在运行的宿主。后台自动刷新与进程内配置重载为 Planned。

启动时使用有效缓存，否则使用内置离线快照。缓存损坏会回退，刷新失败保留已有文件。远端元数据不能修改 Atlas 内置协议、地址预设，也不能根据 effort 控制推断 adaptive thinking。模型名称、ID 和阶梯价格在写入前进行校验，恢复缓存时使用同一套规则，无效条目不能替换上一份有效缓存。不支持工具调用、缺少正数上下文或输出上限、不输出文本，以及标记为 deprecated 的模型会被过滤。用户定义和覆盖最后应用。任意中转站仍通过配置显式定义，不根据模型名称继承官方端点能力。
