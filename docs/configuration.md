# Configuration

[中文](zh-CN/configuration.md)

Atlas reads configuration from `~/.atlas/config.json`. Full example:

```json
{
  "default_model": "deepseek/deepseek-v4-flash",
  "providers": [
    {
      "name": "deepseek",
      "format": "chat_completions",
      "base_url": "https://api.deepseek.com",
      "api_key": "sk-...",
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
    },
    "ws": {
      "host": "0.0.0.0",
      "port": 8765
    }
  }
}
```

## Field Reference

### Top-level

| Field | Description |
|---|---|
| `default_model` | Recommended in `provider/model` format (e.g. `"openai/gpt-5"`). A bare value (e.g. `"gpt-5"`) is also accepted when unambiguous. Used when no model is explicitly selected. |

### Provider

| Field | Description |
|---|---|
| `providers[].name` | Provider name, must be unique. |
| `providers[].format` | Optional, defaults to `chat_completions`. Use `responses` for OpenAI Responses API. |
| `providers[].base_url` | Provider API URL. |
| `providers[].api_key` | Authentication key. |
| `providers[].user_agent` | Optional, overrides the `User-Agent` header sent to the provider API. Defaults to `atlas/<version>`. |

### Models

| Field | Description |
|---|---|
| `models[].value` | Model name sent to the provider. Must be unique within a provider; the same value may appear in multiple providers. |
| `models[].name` | Display name. |
| `models[].context_window` | Context window size, used for compaction and usage display. |
| `models[].max_tokens` | Maximum output tokens per model request, must be ≤ `context_window`. |
| `models[].input_formats` | Supported input formats: `text` and `image`. Must include `text`. |
| `models[].prompt_cache.enabled` | Optional, defaults to off. When `true`, sends a stable `prompt_cache_key` to compatible providers within the same session. |
| `models[].reasoning_efforts` | Declares supported reasoning depth options. Uses the first option when not explicitly selected. |

Only enable `prompt_cache.enabled` after confirming the provider accepts the corresponding field. OpenAI-compatible services vary in compatibility; if requests return unknown field errors or 400s after enabling, remove the `prompt_cache` config for that model to fall back.

### Agent

| Field | Default | Description |
|---|---|---|
| `agent.max_steps` | `20` | Maximum loop steps per turn. |
| `agent.temperature` | `0` | Sampling temperature, range 0–2. |
| `agent.compaction_trigger_ratio` | `0.8` | Auto-compaction triggers when context input reaches this ratio of the window. |

### Memory

| Field | Default | Description |
|---|---|---|
| `memory.model` | empty | Model used for background memory tasks. Uses the session's model when empty. Accepts the same `provider/model` or bare value format as `default_model`. |

### Session

| Field | Default | Description |
|---|---|---|
| `session.db_path` | `~/.atlas/atlas.db` | Session database path. |

### Services

| Field | Description |
|---|---|
| `services.tavily.api_key` | Enables `web_search` and `web_fetch` when configured. |
| `services.weixin.base_url` | Optional, defaults to `https://ilinkai.weixin.qq.com`. |
| `services.weixin.cdn_base_url` | Optional, defaults to `https://novac2c.cdn.weixin.qq.com/c2c`. Used for WeChat image downloads. |
| `services.ws.host` | WebSocket server bind address. Defaults to `0.0.0.0`. |
| `services.ws.port` | WebSocket server port. Defaults to `8765`. |

> **Database migration**: The project is in early stage and does not provide a migration framework. After schema changes, delete the old `~/.atlas/atlas.db` to recreate.
