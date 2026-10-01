# atlas_provider

Model provider adapters and provider-specific request/response conversion.

## Responsibility

- Request mapping and response conversion for the OpenAI-compatible streaming `/chat/completions` and `/responses` endpoints and the Anthropic Messages API, including authentication, SSE parsing, error-body discarding, retries before streaming starts, cancellation bridging, usage normalization, and continuation replay.
- Shared plumbing for every adapter: `HttpStreamClient` carries the retry, timeout, and cancellation policy, and `decodeSse` handles SSE framing. `CompositeModelProvider` routes by provider identifier so several providers share one runtime.

Providers are configured programmatically and injected into `AgentRuntime`:

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

Dio uses a 10-second connection timeout, a 30-second send timeout, and a 2-minute idle receive timeout; cancellation, not a deadline, stops a long-running turn. API keys may be empty for local compatible test servers, in which case OpenAI sends no authorization header, and the default user agent is `Atlas`.

## Allowed dependencies

- `atlas_runtime` public types and `dio` for HTTP.

## Prohibited ownership

- No CLI or configuration-file parsing: providers are configured programmatically and selected by `ModelRef`.
- No tool implementations or application composition.
- Provider-specific fields must not leak into `atlas_runtime` domain requests.
