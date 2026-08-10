# ADR 0004: Provider Package Split and Anthropic Support

- Status: Accepted
- Date: 2026-08-08

## Context

`atlas_provider` grew to a single 948-line file implementing two OpenAI
protocols. Supporting Anthropic's Messages API as a third protocol would push
the file well past a maintainable single-unit size, but Anthropic is not an
OpenAI protocol variant: it has a different path, authentication headers
(`x-api-key` and `anthropic-version`), streaming event contract, tool block
format, and cache model. Adding it to the OpenAI protocol enum would conflate
two unrelated wire formats.

## Decision

- Split `atlas_provider` horizontally into a shared streaming layer and
  per-provider adapters, not vertically into OpenAI protocol subdirectories.
- Extract the retry/timeout/cancellation/error-body handling from the OpenAI
  stream open into `HttpStreamClient` with a Dio-backed default. Both OpenAI
  and Anthropic providers consume it; `sse.dart` is shared as-is.
- Implement `AnthropicProvider` for the Messages API as its own adapter with
  its own configuration, request mapping, and streaming parser. Streaming tool
  `input_json_delta` fragments are accumulated per block and parsed once the
  block completes.
- Add `CompositeModelProvider`, which routes `ModelRef` requests to the
  provider owning the provider identifier, so OpenAI and Anthropic coexist in
  one runtime instance.
- Cross-file shared symbols are public (no underscore) but remain internal to
  the package: `lib/atlas_provider.dart` exports only the public API surface.
  `part` files are not used.
- Anthropic extended thinking is configured per model with a token budget;
  a reasoning effort on the request enables thinking when the budget is
  positive. Assistant tool use replays as standard content blocks; no raw
  provider item replay is needed.

## Consequences

- Each provider adapter stays under roughly 400 lines with a clear
  configuration/request/parser split, and the shared streaming policy has one
  implementation instead of per-provider copies.
- Adding future providers (for example, a local or compatible endpoint) means
  implementing the `ModelProvider` port against `HttpStreamClient` and
  `decodeSse` without touching existing adapters.
- `CompositeModelProvider` lets composition roots register several providers
  without changing the runtime's single-provider port.
- Public-but-internal symbols require documentation comments and discipline to
  keep out of the package export; this matches Dart's `src/` convention.