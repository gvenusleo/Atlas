# atlas_composition

Shared process composition for Atlas applications.

## Responsibility

- Constructs the configured providers, tools, storage, prompt builder, and
  single `AgentRuntime` through `composeRuntime`.
- Exposes `composeModels` so CLI and Flutter entry points can present the
  configured model catalog without duplicating provider mapping.

## Allowed dependencies

- `atlas_config`, `atlas_prompt`, `atlas_provider`, `atlas_runtime`,
  `atlas_storage`, and `atlas_tools`.

## Prohibited ownership

- No presentation, protocol handling, or application lifecycle.
- No CLI argument parsing or configuration-file path discovery; composition
  roots locate `~/.atlas/config.yaml` and pass the loaded `AtlasConfig`.
- Features and presentation code must receive the resulting runtime through
  injection rather than depending on this package.
