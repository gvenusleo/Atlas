# Configuration

[中文](zh-CN/configuration.md)

Atlas loads `~/.atlas/config.yaml` through the `atlas_config` package. `atlas_cli` and `atlas_flutter` locate the file and pass it to `loadConfig`, and `atlas_composition` maps the result onto one runtime.

## Example

```yaml
default_model: anthropic/claude-sonnet

providers:
  - name: anthropic
    type: anthropic
    base_url: https://api.anthropic.com
    api_key: ${ANTHROPIC_API_KEY}
    api_version: "2023-06-01"        # optional, default 2023-06-01
    models:
      - value: claude-sonnet
        name: Claude Sonnet           # optional
        description: ...              # optional
        context_window: 200000
        max_tokens: 4096
        reasoning_efforts:            # optional
          - value: high
            name: High                # optional
        thinking_budget_tokens: 2048  # optional, default 0 (thinking off)

  - name: openai
    type: responses                 # chat_completions | responses | anthropic
    base_url: https://api.openai.com/v1
    api_key: ${OPENAI_API_KEY}
    user_agent: Atlas                 # optional
    models:
      - value: gpt-4o
        context_window: 128000
        max_tokens: 4096
        input_capabilities: [text, image]  # optional, default [text]

agent:
  max_steps: 20                       # optional, default 20
  max_output_tokens: 0                # optional, provider default
  temperature: 0.7                    # optional
  compaction:
    threshold: 0.8                    # optional, default 0.8 (legacy fallback)
    keep_recent_tokens: 20000         # optional, default 20000
    reserve_tokens: 16384             # optional, default 16384

session:
  db_path: ~/.atlas/atlas.db         # optional, ~ expands to home

logging:
  level: info                         # debug | info | warn | error
  directory: ~/.atlas/logs            # optional; omitted disables file logs
  retain_days: 7                      # optional, daily files to retain
```

## Rules

- `default_model` is `"<provider>/<model>"` and must reference a configured provider and model. `providers` and each provider's `models` list must not be empty, provider names must be unique, and model ids must be unique within a provider.
- `type` is `chat_completions`, `responses`, or `anthropic`: the first two select the OpenAI-compatible adapter with the matching API, and both provider families accept an optional `user_agent`.
- `base_url` must be an HTTP(S) URL without a query or fragment.
- `api_key` supports `${ENV_VAR}` references, and an undefined variable fails loading with the variable name in the message.
- `max_tokens`, `context_window`, and `thinking_budget_tokens` must not be negative, and `max_steps` must be greater than zero.
- Anthropic `thinking_budget_tokens` must be less than the effective `max_tokens`. With thinking enabled, Atlas omits `agent.temperature` from Anthropic requests because the sampling option is incompatible. `input_capabilities` and `reasoning_efforts` apply to every provider type.
- `agent.compaction.keep_recent_tokens` must be greater than 0 and caps the newest context kept verbatim during compaction, clamped to one third of the model context window.
- `agent.compaction.reserve_tokens` must be greater than 0. Automatic compaction runs after terminal turns and before each model request once the estimated context reaches `context_window - reserve_tokens`; when `reserve_tokens` is greater than or equal to `context_window`, the legacy `agent.compaction.threshold` fraction triggers instead and must be greater than 0 and at most 1.
- Validation failures raise `ConfigLoadException` with a field path such as `providers[0].base_url`.
- `logging.directory` enables redacted JSON-lines file logging. `ATLAS_LOG_LEVEL` supplies the level when `logging.level` is omitted, and explicit configuration takes precedence.

## MCP servers

The optional `mcp_servers` list configures external tools for the host runtime, with stdio subprocesses and Streamable HTTP endpoints with static headers. Empty configuration keeps only the built-in tools, changes require a restart, and any enabled server that fails to connect aborts startup after cleaning up opened connections. See [MCP tools](mcp.md) for the schema, examples, and content limits.
