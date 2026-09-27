# atlas_config

YAML configuration loading for Atlas composition roots.

## Responsibility

- Defines the `~/.atlas/config.yaml` schema and parses it into
  `AtlasConfig` values.
- Maps configuration onto ready-to-use provider configuration objects
  (`OpenAIProviderConfiguration` / `AnthropicProviderConfiguration`).
- Validates the document and reports `ConfigLoadException` failures with
  field paths.
- Expands `${ENV_VAR}` references in `api_key` and a leading `~/` in
  `session.db_path`. MCP `env` and `headers` support the same substitutions;
  stdio `cwd` expands a leading `~/`. Disabled entries skip secret substitution.
- Defines stdio and Streamable HTTP `mcp_servers` DTOs without opening connections
  or importing the MCP SDK.

## Allowed dependencies

`yaml`, `atlas_runtime`, `atlas_provider` public types.

## Prohibited ownership

- No CLI parsing, path discovery, or argument handling; composition roots
  locate the configuration file.
- No provider, storage, or orchestration logic; the package only builds
  configuration objects for other adapters.