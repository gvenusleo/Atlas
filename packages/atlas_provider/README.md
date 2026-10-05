# atlas_provider

Model provider adapters and provider-specific request/response conversion.

## Responsibility

- Request mapping and response conversion for the OpenAI-compatible streaming `/chat/completions` and `/responses` endpoints and the Anthropic Messages API, including authentication, SSE parsing, error-body discarding, retries before streaming starts, cancellation bridging, usage normalization, and continuation replay.
- Shared plumbing for every adapter: `HttpStreamClient` carries the retry, timeout, and cancellation policy, and `decodeSse` handles SSE framing. `CompositeModelProvider` routes by full model reference so one relay can expose several API protocols through one runtime.
- Owns request-time Pi-style API-key/header resolution, locked `auth.json` updates, provider compatibility settings, and models.dev normalization/cache refresh. OpenAI and Anthropic ship with an offline catalog; the catalog never supplies executable code or changes built-in endpoint presets.

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

- `atlas_runtime` public types, `dio` for HTTP, and `path` for local authentication and catalog files.

Regenerate the offline snapshot from this package with `dart run tool/update_catalog.dart`, or supply a saved models.dev JSON path. Source attribution is in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Prohibited ownership

- No CLI or configuration-file parsing: providers are configured programmatically and selected by `ModelRef`.
- No tool implementations or application composition.
- Provider-specific fields must not leak into `atlas_runtime` domain requests.
