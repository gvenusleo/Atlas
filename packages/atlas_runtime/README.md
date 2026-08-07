# atlas_runtime

The single Atlas agent engine. It will own thread and run orchestration, model/tool loops, cancellation, context compaction, and skill application.

Entry points and protocol adapters call this package instead of implementing their own agent loops.
