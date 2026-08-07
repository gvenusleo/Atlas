# ADR 0001: Rebuild Atlas with Dart and Flutter

- Status: Accepted
- Date: 2026-08-07

## Context

Atlas previously combined a Go runtime with a separate Flutter application shell. The new product direction is a single Dart ecosystem for the runtime, terminal client, protocol adapters, and Flutter clients. Compatibility with the removed Go implementation would constrain the new domain and protocol design without serving a released Dart runtime.

## Decision

- Use a single Pub workspace for all Dart and Flutter packages.
- Own the agent engine instead of adopting an agent SDK.
- Keep one headless runtime shared by Flutter, Nocterm, ACP, MCP, and CLI entry points.
- Let local Flutter and Nocterm entry points call the runtime directly. Reserve the versioned Atlas protocol for remote transports.
- Use `atlas_cli` as the composition root for the default TUI and the planned `atlas server` command. Keep WebSocket client and server behavior in `atlas_ws`.
- Implement ACP and MCP as separate protocol adapters that use `json_rpc_2` directly and own their different lifecycle rules.
- Do not provide compatibility for the previous Go database, configuration, package structure, or internal APIs.

## Consequences

- Flutter is required to resolve and verify the mixed workspace.
- The repository has one dependency resolution and one root lockfile.
- Runtime packages remain usable without Flutter imports even though Flutter supplies the pinned SDK toolchain.
- Local clients avoid protocol serialization, separate service lifecycle, and reconnection behavior. Remote access remains explicit through `atlas server`.
- Go release artifacts and installers are removed until Dart executables have real implementation and release workflows.
- Product behavior worth preserving is documented as runtime contracts rather than copied from the old implementation.
