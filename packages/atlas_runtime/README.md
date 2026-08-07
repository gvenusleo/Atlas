# atlas_runtime

The single Atlas agent runtime. It will own domain models, run events, ports, thread and run orchestration, model/tool loops, cancellation, context compaction, and skill application.

Entry points and protocol adapters call this package instead of implementing their own agent loops.
