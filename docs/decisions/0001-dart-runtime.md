# ADR 0001: Rebuild Atlas with Dart and Flutter

- Status: Accepted
- Date: 2026-08-07

## Context

Atlas previously combined a Go runtime with a separate Flutter application shell. The new product direction is a single Dart ecosystem for the runtime, terminal client, protocol adapters, and Flutter clients. Compatibility with the removed Go implementation would constrain the new domain and protocol design without serving a released Dart runtime.

## Decision

- Use a single Pub workspace for all Dart and Flutter packages.
- Own the agent engine instead of adopting an agent SDK.
- Keep one headless runtime shared by Flutter, Nocterm, ACP, MCP, and CLI entry points.
- Connect presentation clients through a versioned Atlas protocol rather than importing runtime implementation packages.
- Implement ACP and MCP as separate protocol adapters over shared JSON-RPC infrastructure.
- Do not provide compatibility for the previous Go database, configuration, package structure, or internal APIs.

## Consequences

- Flutter is required to resolve and verify the mixed workspace.
- The repository has one dependency resolution and one root lockfile.
- Runtime packages remain usable without Flutter imports even though Flutter supplies the pinned SDK toolchain.
- Go release artifacts and installers are removed until Dart executables have real implementation and release workflows.
- Product behavior worth preserving is documented as runtime contracts rather than copied from the old implementation.
