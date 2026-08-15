# atlas_runtime

The single Atlas agent runtime.

## Responsibility

- Owns Session and Turn domain models, ordered TimelineItem values,
  model/tool ports, the agent loop, cancellation, context compaction, and
  skills.
- Entry points and protocol adapters call this package instead of
  implementing their own agent loops. Provider continuations are represented
  by `ModelContinuation` and persisted checkpoints are linked to assistant
  timeline items.

## Allowed dependencies

- None beyond the Dart SDK; the package defines the ports that adapters
  implement.

## Prohibited ownership

- No persistence, provider, tool, UI, or protocol implementations. Storage,
  provider, tool, and protocol packages depend on and implement runtime
  ports without owning orchestration.
