# atlas_config

Pi-shaped JSON configuration loading for Atlas composition roots.

## Responsibility

- Loads `settings.json`, `models.json`, and `mcp.json` from the supplied Atlas directory; accepts JSON comments and rejects removed YAML configuration.
- Resolves built-in model metadata, custom provider/model definitions, and `modelOverrides` into one immutable application snapshot.
- Validates implemented protocol and compatibility fields with redacted field-path diagnostics. Model API keys and headers remain references for request-time resolution in `atlas_provider`.
- Defines MCP configuration DTOs, expands enabled MCP environment/header references and home paths, and never connects to a server.

## Allowed dependencies

`atlas_runtime`, `atlas_provider`, and `path` public APIs, plus Dart's JSON and file libraries.

## Prohibited ownership

No CLI parsing, provider requests, credential command execution, session storage, UI, or agent orchestration. Composition roots locate the directory and inject the resolved snapshot.
