# ADR 0003: OpenAI-Compatible Provider Protocols

## Status

Accepted

## Context

Atlas needs a real provider adapter before the CLI, TUI, and Flutter
composition roots can execute model turns. OpenAI-compatible services expose
two materially different streaming contracts: Chat Completions and Responses.
Provider authentication, wire fields, SSE framing, retry behavior, and
continuation state must stay outside `atlas_runtime`.

## Decision

- Implement one `OpenAICompatibleProvider` that routes configured models by
  `ModelRef` and selects either Chat Completions or Responses per provider.
- Use Dio for streaming HTTP requests. Retry network failures, HTTP 429, and
  HTTP 5xx only before the first streamed event, with bounded exponential
  backoff and numeric `Retry-After` support. Configure a 10-second connection,
  30-second send, and 2-minute idle receive timeout. Cancellation maps to
  Dio's `CancelToken` and remains active until the stream closes.
- Emit incremental text/reasoning events and exactly one terminal runtime event
  for each request. Invalid or incomplete streamed responses become failures;
  partial completed responses are not returned.
- Store Responses output items in `ModelContinuation` and replay those raw
  items explicitly on the next request when the same provider is selected.
  Do not depend on a server-side `previous_response_id`; this keeps replay
  deterministic across compatible services.
- Keep provider configuration programmatic. CLI/config-file parsing belongs to
  a future composition-root change.

## Consequences

Provider adapters can evolve their wire formats without adding provider fields
to runtime requests. Responses continuation payloads remain provider-owned and
are only interpreted by the matching provider. Local tests can exercise the
complete stream path without API credentials or external network access.
