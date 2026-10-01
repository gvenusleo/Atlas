# atlas_runtime

The single Atlas agent runtime.

## Responsibility

- Owns Session and Turn domain models, ordered TimelineItem values, model and tool ports, the agent loop, cancellation, context compaction, and skills.
- Coalesces transient tool output snapshots from `ToolContext.onOutput` for slow consumers and emits ordered `ToolOutputUpdated` events; only the final `ToolResult` enters the timeline and later model context.
- Entry points and protocol adapters call this package instead of implementing their own agent loops. Provider continuations are represented by `ModelContinuation`, and persisted checkpoints link to assistant timeline items.

## Allowed dependencies

- None beyond the Dart SDK; the package defines the ports that adapters implement.

## Prohibited ownership

- No persistence, provider, tool, UI, or protocol implementations. Those packages implement runtime ports without owning orchestration.
