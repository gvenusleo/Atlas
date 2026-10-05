# atlas_composition

Shared process composition for Atlas applications.

## Responsibility

- Constructs the configured providers, tools, storage, prompt builder, and single `AgentRuntime` through `composeRuntime`, and exposes `composeModels` so CLI and Flutter entry points can present the model catalog without duplicating provider mapping.
- Accepts an optional `shellEnvironment` snapshot from the composition root for its default shell tool; shell initialization belongs to the application.
- Discovers MCP tools through asynchronous `composeTools`, returning an owned tool registry that application roots close after runtime shutdown.
- Shares `composeLogger` between runtime and tool adapters.

## Allowed dependencies

- `atlas_config`, `atlas_prompt`, `atlas_provider`, `atlas_runtime`, `atlas_storage`, `atlas_tools`, and `atlas_mcp`.

## Prohibited ownership

- No presentation, protocol handling, or application lifecycle.
- No CLI argument parsing or configuration-file path discovery; composition roots locate the JSON configuration directory at `~/.atlas` and pass the loaded `AtlasConfig`.
- Feature and presentation code receives the resulting runtime through injection instead of depending on this package.
