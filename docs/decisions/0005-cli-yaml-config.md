# ADR 0005: YAML Configuration Loading

- Status: Accepted
- Date: 2026-08-10

## Context

Two composition roots (`atlas_cli` and `atlas_flutter`) must construct provider,
storage, and runtime adapters before executing model turns. ADR-0003 deferred
configuration-file parsing to a future composition-root change and kept
`atlas_provider` configuration programmatic. The removed Go implementation
loaded `~/.atlas/config.json` with an inline `api_key` and no environment
variable support; the Dart rewrite intentionally dropped that file.

Both composition roots need the same loading, validation, and mapping logic.
Keeping it inside `atlas_cli` would force `atlas_flutter` to either depend on
another application or duplicate the mapping.

## Decision

- Add a new `atlas_config` package that parses `~/.atlas/config.yaml` into
  ready-to-use provider configuration objects.
- Use YAML instead of JSON: the schema is two levels deep, and comments are
  required to annotate providers and API key sources. The `yaml` package is
  maintained by the Dart team and is pure Dart.
- Loading produces `ConfiguredOpenAI` / `ConfiguredAnthropic` wrappers holding
  already-constructed `OpenAIProviderConfiguration` /
  `AnthropicProviderConfiguration` values, so composition roots never remap
  configuration fields.
- `api_key` values support `${ENV_VAR}` references expanded from the process
  environment; referencing an undefined variable is a configuration error.
- A leading `~/` in `session.db_path` expands to the user home directory.
- Validation errors are `ConfigLoadException` values whose message includes
  the failing field path (for example `providers[0].base_url`).
- Provider, runtime, and storage packages are unchanged; `atlas_config` only
  depends on their public types.

## Consequences

Composition roots obtain validated provider and session configuration without
duplicating mapping or validation. Credentials stay out of configuration files
through environment variable references. The schema is the single source of
truth documented in `docs/configuration.md`; adding future sections (tools,
services) is a backward-compatible extension of optional fields.