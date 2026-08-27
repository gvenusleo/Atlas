# ACP Protocol

Atlas implements ACP v1 through `packages/atlas_acp`. The Flutter application
always uses an ACP client, including for an Atlas runtime hosted in-process.

## Supported Surface

- Session creation, prompting, cancellation, loading, resume, listing, close,
  deletion, and configuration options.
- Text, image, embedded text resource, and resource-link prompt blocks.
- Message, reasoning, tool, plan, command, session-info, and usage updates.
- Atlas does not initiate permission requests. Tools run with the permissions
  of the Atlas process; clients must not wait for an approval round trip.

## Atlas Extensions

Atlas extensions use the `_atlas.dev` namespace and are declared in
`agentCapabilities._meta['atlas.dev']`.

- `_atlas.dev/session/set_title` renames a persisted Atlas session.
- `compact` advertises support for Atlas context compaction.
- `permissionModel: none` declares that the Atlas agent never sends
  `session/request_permission`.

The Atlas ACP client still handles permission requests from third-party
agents. Agent and client permission behavior are separate protocol roles.

The runtime-facing contract is `AgentSession`; ACP-only presentation members
(titles, commands, and modes) are exposed through `PresentationAgentSession`.

## Planned

Structured `_atlas.dev/session/compact`, client filesystem and terminal
capabilities, and ACP v2 support remain planned.
