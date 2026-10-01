# atlas_config

YAML configuration loading for Atlas composition roots.

## Responsibility

- Defines the `~/.atlas/config.yaml` schema, parses it into `AtlasConfig` values, and maps them onto ready-to-use provider configuration objects.
- Validates the document and reports `ConfigLoadException` failures with field paths.
- Expands `${ENV_VAR}` references and leading `~/` paths for Atlas settings and MCP server entries, skipping secret substitution for disabled entries.
- Defines stdio and Streamable HTTP `mcp_servers` DTOs without opening connections or importing the MCP SDK.

## Allowed dependencies

`yaml`, `atlas_runtime`, and `atlas_provider` public types.

## Prohibited ownership

- No CLI parsing, path discovery, or argument handling; composition roots locate the configuration file.
- No provider, storage, or orchestration logic; the package only builds configuration objects for other adapters.
