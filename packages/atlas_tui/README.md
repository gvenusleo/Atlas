# atlas_tui

The Nocterm presentation package for Atlas.

## Responsibility

- Renders the Nocterm chat interface: a scrollable message transcript
  (assistant markdown, user, reasoning, tool, and error rows), a text input
  bar, and a turn status indicator.
- Owns `ChatController`, which bridges `atlas_runtime` turn events into
  rendered messages. It receives an injected `AgentRuntime` and never calls
  providers, tools, or storage directly.
- Exposes `runAtlasTui` as the single entry point that boots the Nocterm app
  and its shutdown; `atlas_cli` depends on this entry instead of importing
  the rendering library directly.

## Allowed dependencies

- `atlas_runtime` public types and the `nocterm` rendering library.

## Prohibited ownership

- No composition roots: bootstrap of providers, tools, storage, and the system
  prompt belongs to `atlas_composition`, called from `atlas_cli` and
  `atlas_flutter`.
- No model, provider, storage, or orchestration logic; the agent loop lives in
  `atlas_runtime`.
- No remote client protocol logic; `atlas_ws`, `atlas_acp`, and `atlas_mcp`
  adapters are not owned here.