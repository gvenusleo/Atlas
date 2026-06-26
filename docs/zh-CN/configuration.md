# 配置

[English](../configuration.md)

Atlas 从 `~/.atlas/config.json` 读取配置。完整示例：

```json
{
  "active_provider": "deepseek",
  "providers": [
    {
      "name": "deepseek",
      "format": "chat_completions",
      "base_url": "https://api.deepseek.com",
      "api_key": "sk-...",
      "default_model": "deepseek-v4-flash",
      "models": [
        {
          "value": "deepseek-v4-flash",
          "name": "DeepSeek V4 Flash",
          "context_window": 1000000,
          "max_tokens": 384000,
          "input_formats": ["text"],
          "reasoning_efforts": [
            {
              "value": "high",
              "name": "High"
            },
            {
              "value": "max",
              "name": "Max"
            }
          ]
        },
        {
          "value": "deepseek-v4-pro",
          "name": "DeepSeek V4 Pro",
          "context_window": 1000000,
          "max_tokens": 384000,
          "input_formats": ["text", "image"]
        }
      ]
    },
    {
      "name": "openai",
      "format": "responses",
      "base_url": "https://api.openai.com/v1",
      "api_key": "sk-...",
      "default_model": "gpt-5",
      "models": [
        {
          "value": "gpt-5",
          "name": "GPT-5",
          "context_window": 400000,
          "max_tokens": 128000,
          "input_formats": ["text", "image"],
          "prompt_cache": {
            "enabled": true
          }
        }
      ]
    }
  ],
  "agent": {
    "max_steps": 20,
    "temperature": 0.2,
    "compaction_trigger_ratio": 0.8
  },
  "memory": {
    "enabled": true,
    "model": ""
  },
  "session": {
    "db_path": "~/.atlas/atlas.db"
  },
  "services": {
    "tavily": {
      "api_key": "tvly-..."
    },
    "weixin": {
      "cdn_base_url": "https://novac2c.cdn.weixin.qq.com/c2c"
    }
  }
}
```

## 字段说明

### Provider

| 字段 | 说明 |
|---|---|
| `active_provider` | 必须匹配某个 `providers[].name`，Atlas 只使用当前选中的 Provider |
| `providers[].format` | 可省略，默认 `chat_completions`；OpenAI Responses API 使用 `responses` |
| `providers[].base_url` | Provider API 地址 |
| `providers[].api_key` | 鉴权密钥 |
| `providers[].default_model` | 必须匹配同一 Provider 下的某个 `models[].value` |

### 模型

| 字段 | 说明 |
|---|---|
| `models[].value` | 发送给 Provider 的模型名 |
| `models[].name` | 显示名 |
| `models[].context_window` | 上下文窗口，用于压缩和用量展示 |
| `models[].max_tokens` | 每次模型请求的最大输出 token 数，需 ≤ `context_window` |
| `models[].input_formats` | 支持的输入格式，当前支持 `text` 和 `image`，且必须包含 `text` |
| `models[].prompt_cache.enabled` | 可省略，默认关闭；设为 `true` 时，同一 Atlas session 会向兼容 Provider 发送稳定的 `prompt_cache_key` |
| `models[].reasoning_efforts` | 声明支持的思考深度选项；未显式选择时使用第一项 |

`prompt_cache.enabled` 只应在确认 Provider 接受对应字段后开启。OpenAI-compatible 服务兼容性不一致；如果开启后请求返回未知字段或 400 错误，删除该模型的 `prompt_cache` 配置即可回退。

### Agent

| 字段 | 默认值 | 说明 |
|---|---|---|
| `agent.max_steps` | `20` | 单次 turn 最大循环步数 |
| `agent.temperature` | `0` | 采样温度，范围 0–2 |
| `agent.compaction_trigger_ratio` | `0.8` | 上下文输入达到窗口的该比例时自动压缩 |

### 记忆

| 字段 | 默认值 | 说明 |
|---|---|---|
| `memory.enabled` | `true` | 是否启用长期记忆，未配置时默认启用 |
| `memory.model` | 空 | 后台记忆任务使用的模型；为空时使用产生该会话的模型 |

### Session

| 字段 | 默认值 | 说明 |
|---|---|---|
| `session.db_path` | `~/.atlas/atlas.db` | 会话数据库路径 |

### Services

| 字段 | 说明 |
|---|---|
| `services.tavily.api_key` | 配置后启用 `web_search` 和 `web_fetch` |
| `services.weixin.base_url` | 可省略，默认 `https://ilinkai.weixin.qq.com` |
| `services.weixin.cdn_base_url` | 可省略，默认 `https://novac2c.cdn.weixin.qq.com/c2c`，用于微信图片下载 |

> **数据库迁移**：当前项目处于早期阶段，不提供迁移框架。schema 变化后请删除旧的 `~/.atlas/atlas.db` 重新生成。
