# atlas_tui

The Nocterm presentation package for Atlas.

## Responsibility

- Renders and interacts with the Nocterm terminal interface over an injected
  `atlas_runtime` interface.
- Consumes runtime events for display only; it never owns the agent loop.

## Allowed dependencies

- `atlas_runtime` public types and the `nocterm` rendering library when the
  terminal UI is implemented.

## Prohibited ownership

- No composition roots: bootstrap of providers, tools, storage, and the system
  prompt belongs to `atlas_cli` (and `atlas_flutter`).
- No model, provider, storage, or orchestration logic.
- No remote client protocol logic; `atlas_ws`, `atlas_acp`, and `atlas_mcp`
  adapters are not owned here.