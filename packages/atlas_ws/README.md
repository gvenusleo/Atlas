# atlas_ws

The WebSocket transport for remote Atlas clients.

> **Status:** Planned. The wire contract and transport are approved but not
> implemented yet; the package currently contains no code.

## Responsibility

- Owns the versioned WebSocket wire contract, codecs, client and server
  connection behavior, and explicit conversion to runtime types.

## Allowed dependencies

- `atlas_runtime` public types. A dedicated WebSocket dependency is added
  only with the first real transport implementation.

## Prohibited ownership

- No runtime composition: the package accepts an injected request handler
  and must not construct providers, tools, storage, or the agent loop.
- No CLI or configuration-file parsing.
