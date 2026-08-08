# ADR 0002: Session, Turn, and Timeline Domain

- Status: Accepted
- Date: 2026-08-07

## Context

The first executable Dart runtime needs durable history, provider-neutral model
messages, and a persistence boundary without importing Drift into orchestration.
The removed Go implementation is not a compatibility target, so the new domain
can use names and invariants that fit the Dart runtime directly.

## Decision

- A `Session` owns an ordered timeline and contains durable working-directory
  settings and latest usage metadata.
- A `Turn` records one submitted user request and its terminal lifecycle state.
- Every durable message and tool call/result is a typed `TimelineItem` with a
  session-local sequence.
- Provider-specific continuation state is modeled as `ModelContinuation` and
  persisted as a `ModelCheckpoint` linked to its assistant timeline item.
- `SessionStore` is the runtime persistence port. `atlas_storage` maps these
  values to Drift rows and versioned JSON payloads; it does not redefine the
  runtime domain.
- The runtime persists the user item before the first provider request and
  persists each assistant step before executing its tools.

## Consequences

- Provider adapters can restore their own continuation payload without leaking
  provider fields into `ModelRequest`.
- Clients can render the exact occurrence order from one timeline and one event
  stream.
- Schema migrations can evolve storage representations while domain types stay
  independent of Drift.
- A resumed turn can explicitly preserve or replace session directory roots.
