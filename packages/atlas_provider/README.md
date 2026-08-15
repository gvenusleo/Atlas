# atlas_provider

Model provider adapters and provider-specific request/response conversion.

`OpenAICompatibleProvider` supports the streaming `/chat/completions` and
`/responses` endpoints, and `AnthropicProvider` implements the Messages API.
Configure providers and models programmatically, then inject the providers
into `AgentRuntime`, optionally through `CompositeModelProvider` so multiple
providers share one runtime instance.

```dart
final composite = CompositeModelProvider({
  ProviderId('openai'): OpenAICompatibleProvider([
    OpenAIProviderConfiguration(
      id: ProviderId('openai'),
      protocol: OpenAIProtocol.responses,
      baseUrl: Uri.parse('https://api.openai.com/v1'),
      apiKey: apiKey,
      models: [
        OpenAIModelConfiguration(
          descriptor: ModelDescriptor(
            ref: ModelRef(
              providerId: ProviderId('openai'),
              modelId: ModelId('gpt-5.6'),
            ),
          ),
        ),
      ],
    ),
  ]),
  ProviderId('anthropic'): AnthropicProvider([
    AnthropicProviderConfiguration(
      id: ProviderId('anthropic'),
      baseUrl: Uri.parse('https://api.anthropic.com'),
      apiKey: anthropicKey,
      models: [
        AnthropicModelConfiguration(
          descriptor: ModelDescriptor(
            ref: ModelRef(
              providerId: ProviderId('anthropic'),
              modelId: ModelId('claude-sonnet-4-5'),
            ),
          ),
        ),
      ],
    ),
  ]),
});
```

The package owns endpoint authentication, request mapping, SSE parsing, bounded
error-body handling, retries before streaming starts, cancellation bridging,
usage normalization, and provider continuation replay. OpenAI and Anthropic
providers share `HttpStreamClient` for the retry/timeout/cancellation policy
and `decodeSse` for SSE framing. Provider-specific fields must not leak into
`atlas_runtime` domain requests. CLI configuration parsing, tool
implementations, and application composition remain outside this package.

Dio uses a 10-second connection timeout, a 30-second request-body send timeout,
and a 2-minute idle receive timeout for streaming connections. There is no
overall response deadline because model generation is streamed; cancellation is
the explicit way to stop a long-running turn.

API keys may be empty for local compatible test servers; in that case OpenAI
sends no authorization header. The default user agent is `Atlas`.

## Allowed dependencies

- `atlas_runtime` public types and `dio` for HTTP.

## Prohibited ownership

- No CLI or configuration-file parsing: providers are configured
  programmatically and selected by `ModelRef`.
- No tool implementations or application composition; composition roots
  construct and inject providers.
- Provider-specific fields must not leak into `atlas_runtime` domain
  requests.