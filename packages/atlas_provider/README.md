# atlas_provider

Model provider adapters and provider-specific request/response conversion.

`OpenAICompatibleProvider` currently supports both streaming
`/chat/completions` and `/responses` endpoints. Configure providers and models
programmatically with `OpenAIProviderConfiguration` and
`OpenAIModelConfiguration`, then inject the provider into `AgentRuntime`.

```dart
final provider = OpenAICompatibleProvider([
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
]);
```

The package owns endpoint authentication, request mapping, SSE parsing, bounded
error-body handling, retries before streaming starts, cancellation bridging,
usage normalization, and provider continuation replay. Provider-specific fields
must not leak into `atlas_runtime` domain requests. CLI configuration parsing,
tool implementations, and application composition remain outside this package.

Dio uses a 10-second connection timeout, a 30-second request-body send timeout,
and a 2-minute idle receive timeout for streaming connections. There is no
overall response deadline because model generation is streamed; cancellation is
the explicit way to stop a long-running turn.

API keys may be empty for local compatible test servers; in that case no
authorization header is sent. The default user agent is `Atlas`.
