# Data Model

Atlas persists sessions as ordered turns and timeline items. Every timeline
item belongs to the same session and turn as its enclosing operation.

SQLite stores session, turn, and timeline timestamps as UTC Unix milliseconds
(`INTEGER`). The schema requires a fresh database after this precision change;
there is no migration from the earlier seconds-based timestamps.

## Turn Flow

1. `beginTurn` atomically creates or updates the session, creates the turn,
   and stores the user item.
2. Model steps append assistant content and tool calls in occurrence order.
3. Every persisted tool call receives a result before the model continues.
4. A terminal turn records completed, cancelled, or failed exactly once.

## Compaction

A compaction checkpoint stores a summary and the final compacted timeline
sequence. Active model context begins after that sequence. A checkpoint never
splits an assistant/tool/result group; a long single turn may compact its
oldest safe prefix while preserving the newest item.

## Failures

Persisted and user-visible failures contain redacted summaries. Provider
credentials, request headers, complete request bodies, and model output must
never enter failure messages or structured logs.
