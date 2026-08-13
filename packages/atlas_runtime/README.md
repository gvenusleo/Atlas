# atlas_runtime

The single Atlas agent runtime. It owns Session and Turn domain models, ordered
TimelineItem values, model/tool ports, the agent loop, and cancellation. Context
compaction and skill application remain runtime responsibilities as they are
implemented.

Entry points and protocol adapters call this package instead of implementing
their own agent loops. Provider continuations are represented by
`ModelContinuation` and persisted checkpoints are linked to assistant timeline
items.
