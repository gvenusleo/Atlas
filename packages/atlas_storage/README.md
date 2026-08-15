# atlas_storage

Drift persistence for Atlas sessions, turns, ordered timeline items, provider
continuations, and compaction checkpoints. Native databases execute on a
background isolate.

## Responsibility

- Owns tables, queries, migrations, and conversion between generated Drift
  rows and runtime domain objects.
- Implements the storage ports from `atlas_runtime`. The database schema is
  an implementation detail; runtime domain types remain the source of truth
  for application behavior.

## Allowed dependencies

- `atlas_runtime` public types, `drift`, and the native SQLite bindings it
  requires.

## Prohibited ownership

- No runtime orchestration or UI state.
- Composition roots use `DriftSessionStore.openFile` or
  `DriftSessionStore.inMemory` instead of depending on generated Drift
  tables.
