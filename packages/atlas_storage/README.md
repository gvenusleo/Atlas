# atlas_storage

Drift persistence for Atlas threads, runs, timeline items, and compaction
checkpoints. Native databases execute on a background isolate.

This package owns tables, queries, migrations, and conversion between generated
Drift rows and runtime domain objects. It implements storage ports from
`atlas_runtime` and must not contain runtime orchestration or UI state.
