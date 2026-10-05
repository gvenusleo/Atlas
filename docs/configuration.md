# Configuration

[中文](zh-CN/configuration.md)

Atlas uses Pi-shaped JSON configuration in `~/.atlas`. CLI, TUI, ACP, the WebSocket server, and Flutter local mode load the same documents. Remote Flutter clients use their host's model configuration.

| File | Responsibility |
| --- | --- |
| `settings.json` | Default provider/model, thinking preference, agent, compaction, session storage, and logging |
| `models.json` | Custom providers, model definitions, and overrides of built-in models |
| `auth.json` | Saved API keys indexed by provider ID |
| `mcp.json` | Host MCP servers; see [MCP tools](mcp.md) |
| `cache/model-catalog.json` | Replaceable models.dev cache |
| `atlas.db` | Sessions, turns, usage, messages, and provider continuations |

JSON documents accept `//` and `/* */` comments, as Pi's model loader does. Trailing commas are rejected. Missing documents use defaults; malformed documents and unsupported fields fail with a file or field path. The removed `config.yaml` is rejected if present. There is no automatic migration or deletion of user files.

## Settings

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

All settings are optional. `defaultProvider` defaults to `openai`; an omitted `defaultModel` selects the first model of that provider in the resolved catalog. Set both explicitly for a stable startup choice. The selected model must exist, but missing credentials do not prevent loading the configuration. `defaultThinkingLevel` accepts `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, or `max`; models expose their supported selections, and an unsupported startup preference is clamped to a supported level.

`agent.maxSteps` defaults to 20. `maxOutputTokens` defaults to 0, selecting an adapter budget of up to 4096 tokens bounded by the model's `maxTokens`; a positive explicit budget above the model limit fails before HTTP. `temperature` is omitted by default and is not sent alongside active OpenAI reasoning or Anthropic thinking.

`keepRecentTokens` and `reserveTokens` must be positive. Compaction uses `contextWindow - reserveTokens`, with `threshold` as the legacy fraction fallback when the reserve is at least the context window. `threshold` must be greater than 0 and at most 1. Session and logging paths expand a leading `~/`. Omitted `logging.directory` disables file logging; `ATLAS_LOG_LEVEL` supplies the level unless explicitly configured.

## Third-party relays and local endpoints

A provider ID names a service, including a relay or local server. It need not exist in models.dev. There is no separate connection hierarchy.

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

Supported `api` values are `openai-completions` (Chat Completions), `openai-responses`, and `anthropic-messages`. Model-level `api` and `baseUrl` override provider defaults, so one relay can serve multiple protocols. `baseUrl` is the API base including any version prefix for all three protocols. The official OpenAI and Anthropic presets are `https://api.openai.com/v1` and `https://api.anthropic.com/v1`. Adapters append only `/chat/completions`, `/responses`, or `/messages`; custom proxy prefixes are preserved. Anthropic configurations written for the previous root-only convention must add `/v1` where their endpoint requires it. Model IDs are sent unchanged, including IDs containing `/`. Local servers can use a dummy key, as in Pi.

Each custom model needs `id` and a resolvable API/address. Like Pi, omitted metadata defaults to the ID as the display name, text input, no reasoning, 128000 context tokens, and 16384 maximum output tokens. These fallback limits are not a measurement of the endpoint: configure its actual limits when they differ. Models are unique within a provider. Provider IDs cannot contain `/`.

## Built-ins and overrides

OpenAI and Anthropic have built-in protocol/address presets and a bundled models.dev catalog. No `models.json` is needed to use their catalog with `OPENAI_API_KEY` or `ANTHROPIC_API_KEY`. Override an existing provider to send its models through a compatible proxy:

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

`models` adds or replaces complete model definitions; replacement inherits API/address when available, but other omitted metadata uses the custom-model defaults. `modelOverrides` merges only supplied fields into existing models, including nested cost, compatibility, and sampling settings. Overrides for unknown model IDs are ignored, matching Pi. Override entries do not change `api` or `baseUrl`; use a `models` definition for those changes. Provider-level `api` supplies defaults for `models` definitions and does not rewrite the built-in model list.

Model fields are `id`, `name`, `api`, `baseUrl`, `reasoning`, `thinkingLevelMap`, `input`, `cost`, `contextWindow`, `maxTokens`, `samplingParams`, `samplingParamsByThinkingLevel`, `headers`, and `compat`. Provider fields are `name`, `baseUrl`, `apiKey`, `api`, `headers`, `compat`, `authHeader`, `models`, and `modelOverrides`. Header names merge case-insensitively with model values winning. Transport-owned headers and newline-containing header values are rejected.

`cost` retains USD-per-million-token rates (`input`, `output`, `cacheRead`, `cacheWrite`) and optional `tiers` with `inputTokensAbove`. Missing prices remain unknown. Cost display and historical monetary accounting are Planned; token usage continues to be recorded in SQLite.

## Authentication

```sh
atlas auth set relay
atlas auth remove relay
```

`set` reads a key without terminal echo, or reads standard input when piped. It writes Pi's `{ "relay": { "type": "api_key", "key": "..." } }` shape to `auth.json`. Writes serialize in-process, lock across processes, preserve other providers, and atomically replace the file. POSIX files use mode `0600`; Windows uses inherited filesystem ACLs. No encryption or system-keychain integration is implied.

Credential precedence is runtime override, saved `auth.json` key, configured `apiKey`, then the built-in provider's environment variables. The key and custom headers resolve for each request, so a saved-key change takes effect without restarting. `authHeader: true` additionally requires a resolved key and sets `Authorization: Bearer ...`; setting it to false does not disable an API adapter's normal authentication.

Keys and headers accept literals, `$NAME`, `${NAME}`, and a leading `!command`. In non-command values, `$$` escapes `$` and `$!` escapes `!`. Commands use the host shell (`/bin/sh` on POSIX, `cmd.exe` on Windows), are not cached, and have a 10-second/64-KiB output bound. Failed commands and missing variables produce redacted errors. Model listing and configuration validation do not execute credential commands.

## Compatibility options

Atlas accepts the implemented subset of Pi's `compat` fields. Unknown fields, unsupported API types, and unsupported thinking formats fail explicitly.

| API | Implemented fields |
| --- | --- |
| `openai-completions` | `supportsStore`, `supportsDeveloperRole`, `supportsReasoningEffort`, `supportsUsageInStreaming`, `supportsFinishReason`, `maxTokensField`, `requiresToolResultName`, `requiresAssistantAfterToolResult`, `requiresThinkingAsText`, `requiresReasoningContentOnAssistantMessages`, `thinkingFormat`, `supportsStrictMode`, `sendSessionAffinityHeaders`, `sessionAffinityFormat` |
| `openai-responses` | `supportsDeveloperRole`, `supportsMaxOutputTokens`, `supportsStrictMode`, `sessionAffinityFormat` |
| `anthropic-messages` | `supportsCacheControlOnTools`, `supportsTemperature`, `forceAdaptiveThinking`, `allowEmptySignature`, `supportsStrictTools`, `sendSessionAffinityHeaders` |

`maxTokensField` is `max_completion_tokens` or `max_tokens`. `thinkingFormat` currently supports `openai` and `deepseek`. `sessionAffinityFormat` supports `openai`, `openai-nosession`, and `openrouter`. Enable compatibility options only for verified endpoint behavior.

`thinkingLevelMap` maps Atlas/Pi level names to provider effort strings or null to omit the effort field. OpenAI sampling defaults support numeric `temperature`, `top_p`, `top_k`, `min_p`, `frequency_penalty`, `presence_penalty`, `repetition_penalty`, and `seed`; level-specific values override model defaults, and an unselected thinking level uses the `off` sampling entry. These maps cannot replace model IDs, messages, tools, or streaming fields. Anthropic supports budget-based thinking and explicitly configured adaptive thinking; it rejects OpenAI sampling maps. Manual thinking requires at least 1024 thinking tokens and reserves at least 1024 tokens for the answer, so output budgets below 2048 are rejected before HTTP when thinking is enabled. A large thinking budget is reduced to preserve that answer space. Effort support does not imply adaptive thinking: Opus 4.5 uses manual thinking with effort, while verified adaptive models use `type: adaptive`.

`supportsStrictTools` enables strict sampling only for eligible schemas. Atlas checks nested schemas and keeps ordinary tool use for open objects, unsupported constraints, references, and other schema features outside its verified subset. Original tool schemas are preserved.

OAuth, provider extensions, other Pi APIs, and the remaining Pi compatibility/input-limit options are Planned. Atlas does not claim complete Pi configuration compatibility.

## Catalog and commands

```sh
atlas config validate
atlas models list
atlas models refresh
```

`validate` checks settings, models, and MCP without opening model or MCP connections. `list` prints model references and context/output limits, including models whose credentials have not been supplied. `refresh` downloads `https://models.dev/api.json` through Dio, validates supported providers, and atomically replaces the cache. Restart active hosts after changing model configuration or refreshing the catalog. Automatic background refresh and in-process configuration reload are Planned.

Startup uses a valid cached catalog or the bundled offline snapshot. Invalid caches fall back to the bundle; failed refreshes preserve the existing file. Remote metadata cannot change Atlas's built-in protocol/address presets or infer adaptive thinking from effort controls. Names, model IDs, and tier prices are validated before caching with the same rules used when restoring the cache; invalid entries cannot replace the last valid cache. Models that lack tool calling, positive context/output limits, or text output, and models marked deprecated, are omitted. User definitions and overrides are applied after the catalog. Arbitrary relays remain configured explicitly and do not inherit official model capabilities by name.
