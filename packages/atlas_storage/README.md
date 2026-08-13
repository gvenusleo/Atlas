# atlas_storage

Drift persistence for Atlas sessions, turns, ordered timeline items, provider
continuations, and compaction checkpoints. Native databases execute on a
background isolate.

This package owns tables, queries, migrations, and conversion between generated
Drift rows and runtime domain objects. It implements storage ports from
`atlas_runtime` and must not contain runtime orchestration or UI state. The
database schema is an implementation detail; runtime domain types remain the
source of truth for application behavior. Use `DriftSessionStore.openFile` or
`DriftSessionStore.inMemory` from composition roots instead of depending on
generated Drift tables.
