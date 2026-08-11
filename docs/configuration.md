# Configuration

[中文](zh-CN/configuration.md)

Atlas loads its application configuration from `~/.atlas/config.yaml` through
the `atlas_config` package. Composition roots (`atlas_cli`, `atlas_flutter`)
locate the file and pass it to `loadConfig`; the package parses, validates, and
maps it onto provider configuration objects.

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
        thinking_budget_tokens: 4096  # optional, default 0 (thinking off)

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
        prompt_cache: true            # optional, default false

agent:
  max_steps: 100                      # optional, default 100
  temperature: 0.7                    # optional
  compaction:
    threshold: 0.8                    # optional, default 0.8

session:
  db_path: ~/.atlas/atlas.db         # optional, ~ expands to home
```

## Rules

- `default_model` is `"<provider>/<model>"` and must reference a configured
  provider and model.
- Provider names must be unique; model ids must be unique within a provider.
- `type` is `chat_completions`, `responses`, or `anthropic`. The first two
  select the OpenAI-compatible adapter with the matching API; `anthropic`
  selects the Anthropic adapter.
- `base_url` must be an HTTP(S) URL without a query or fragment.
- `api_key` supports `${ENV_VAR}` references; an undefined variable fails
  loading with the variable name in the message.
- `max_tokens`, `context_window`, `max_steps`, and `thinking_budget_tokens`
  must not be negative.
- `agent.compaction.threshold` must be greater than 0 and at most 1; it is the
  context window fraction that triggers automatic compaction after a turn.
- Validation failures raise `ConfigLoadException` with a field path such as
  `providers[0].base_url`.