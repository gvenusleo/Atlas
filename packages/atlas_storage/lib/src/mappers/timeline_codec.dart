import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';

/// The encoded database representation of a timeline item.
final class EncodedTimelineItem {
  /// Creates an encoded timeline item.
  const EncodedTimelineItem({
    required this.kind,
    required this.version,
    required this.payload,
  });

  /// The stable discriminant.
  final String kind;

  /// The payload schema version.
  final int version;

  /// The JSON payload.
  final String payload;
}

/// Encodes and decodes runtime timeline variants.
final class TimelineCodec {
  /// Encodes one timeline item with a stable kind and version; the optional
  /// [checkpoint] is embedded in the assistant payload when present.
  EncodedTimelineItem encode(TimelineItem item, {ModelCheckpoint? checkpoint}) {
    final JsonObject payload;
    final String kind;
    switch (item) {
      case UserMessageItem(:final content):
        kind = 'user_message';
        payload = {'content': _encodeContent(content)};
      case AssistantMessageItem(
        :final content,
        :final model,
        :final stopReason,
        :final usage,
      ):
        kind = 'assistant_message';
        payload = {
          'content': _encodeContent(content),
          'provider_id': model.providerId.value,
          'model_id': model.modelId.value,
          'stop_reason': stopReason.name,
          'usage': _encodeUsage(usage),
          if (checkpoint != null)
            'continuation': _encodeContinuation(checkpoint),
        };
      case ToolCallItem(:final call):
        kind = 'tool_call';
        payload = {
          'call_id': call.id.value,
          'name': call.name,
          'arguments': call.arguments,
        };
      case ToolResultItem(
        :final callId,
        :final content,
        :final isError,
        :final metadata,
      ):
        kind = 'tool_result';
        payload = {
          'call_id': callId.value,
          'content': content,
          'is_error': isError,
          'metadata': metadata,
        };
    }
    return EncodedTimelineItem(
      kind: kind,
      version: 1,
      payload: jsonEncode(payload),
    );
  }

  /// Decodes one versioned database payload and its embedded continuation.
  ({TimelineItem item, ModelCheckpoint? checkpoint}) decode({
    required TimelineItemId id,
    required SessionId sessionId,
    required TurnId turnId,
    required int sequence,
    required DateTime occurredAt,
    required String kind,
    required int version,
    required String payload,
  }) {
    if (version != 1) {
      throw FormatException('Unsupported timeline payload version: $version');
    }
    final decoded = jsonDecode(payload);
    if (decoded is! Map<String, Object?>) {
      throw const FormatException('Timeline payload must be a JSON object');
    }
    switch (kind) {
      case 'user_message':
        return (
          item: UserMessageItem(
            id: id,
            sessionId: sessionId,
            turnId: turnId,
            sequence: sequence,
            occurredAt: occurredAt.toUtc(),
            content: _decodeContent(decoded['content']),
          ),
          checkpoint: null,
        );
      case 'assistant_message':
        return (
          item: AssistantMessageItem(
            id: id,
            sessionId: sessionId,
            turnId: turnId,
            sequence: sequence,
            occurredAt: occurredAt.toUtc(),
            content: _decodeContent(decoded['content']),
            model: ModelRef(
              providerId: ProviderId(_string(decoded, 'provider_id')),
              modelId: ModelId(_string(decoded, 'model_id')),
            ),
            stopReason: _enumByName(
              StopReason.values,
              _string(decoded, 'stop_reason'),
              'stop_reason',
            ),
            usage: _decodeUsage(decoded['usage']),
          ),
          checkpoint: _decodeContinuation(decoded, id, occurredAt),
        );
      case 'tool_call':
        return (
          item: ToolCallItem(
            id: id,
            sessionId: sessionId,
            turnId: turnId,
            sequence: sequence,
            occurredAt: occurredAt.toUtc(),
            call: ToolCall(
              id: ToolCallId(_string(decoded, 'call_id')),
              name: _string(decoded, 'name'),
              arguments: _jsonObject(decoded['arguments'], 'arguments'),
            ),
          ),
          checkpoint: null,
        );
      case 'tool_result':
        return (
          item: ToolResultItem(
            id: id,
            sessionId: sessionId,
            turnId: turnId,
            sequence: sequence,
            occurredAt: occurredAt.toUtc(),
            callId: ToolCallId(_string(decoded, 'call_id')),
            content: _string(decoded, 'content'),
            isError: _bool(decoded, 'is_error'),
            metadata: _jsonObject(decoded['metadata'], 'metadata'),
          ),
          checkpoint: null,
        );
      default:
        throw FormatException('Unsupported timeline item kind: $kind');
    }
  }

  static List<JsonObject> _encodeContent(List<ContentPart> content) => [
    for (final part in content)
      switch (part) {
        TextContent(:final text) => {'type': 'text', 'text': text},
        ImageContent(:final source, :final mimeType, :final detail) => {
          'type': 'image',
          'source': source,
          'mime_type': ?mimeType,
          'detail': detail.name,
        },
        ResourceContent(:final uri, :final mimeType, :final text) => {
          'type': 'resource',
          'uri': uri,
          'mime_type': ?mimeType,
          'text': text,
        },
      },
  ];

  static List<ContentPart> _decodeContent(Object? value) {
    if (value is! List<Object?>) {
      throw const FormatException('content must be a JSON array');
    }
    return List<ContentPart>.unmodifiable(
      value.map((item) {
        final object = _jsonObject(item, 'content item');
        switch (_string(object, 'type')) {
          case 'text':
            return TextContent(_string(object, 'text'));
          case 'image':
            return ImageContent(
              source: _string(object, 'source'),
              mimeType: object['mime_type'] as String?,
              detail: _enumByName(
                ImageDetail.values,
                _string(object, 'detail'),
                'detail',
              ),
            );
          case 'resource':
            return ResourceContent(
              uri: _string(object, 'uri'),
              mimeType: object['mime_type'] as String?,
              text: _stringOrDefault(object, 'text'),
            );
          default:
            throw FormatException(
              'Unsupported content type: ${object['type']}',
            );
        }
      }),
    );
  }

  static JsonObject? _encodeContinuation(ModelCheckpoint checkpoint) => {
    'provider_id': checkpoint.continuation.providerId.value,
    'reasoning_summary': checkpoint.continuation.reasoningSummary,
    'payload': checkpoint.continuation.opaquePayload,
  };

  static ModelCheckpoint? _decodeContinuation(
    Map<String, Object?> object,
    TimelineItemId itemId,
    DateTime occurredAt,
  ) {
    final value = object['continuation'];
    if (value == null) {
      return null;
    }
    final continuation = _jsonObject(value, 'continuation');
    return ModelCheckpoint(
      timelineItemId: itemId,
      continuation: ModelContinuation(
        providerId: ProviderId(_string(continuation, 'provider_id')),
        reasoningSummary: _stringOrDefault(continuation, 'reasoning_summary'),
        opaquePayload: _jsonObject(continuation['payload'], 'continuation'),
      ),
      createdAt: occurredAt.toUtc(),
    );
  }

  static JsonObject _encodeUsage(TokenUsage usage) => {
    'input_tokens': usage.inputTokens,
    'output_tokens': usage.outputTokens,
    'total_tokens': usage.totalTokens,
    'cache_read_input_tokens': usage.cacheReadInputTokens,
    'cache_write_input_tokens': usage.cacheWriteInputTokens,
  };

  static TokenUsage _decodeUsage(Object? value) {
    final object = _jsonObject(value, 'usage');
    return TokenUsage(
      inputTokens: _int(object, 'input_tokens'),
      outputTokens: _int(object, 'output_tokens'),
      totalTokens: _int(object, 'total_tokens'),
      cacheReadInputTokens: _int(object, 'cache_read_input_tokens'),
      cacheWriteInputTokens: _int(object, 'cache_write_input_tokens'),
    );
  }

  static JsonObject _jsonObject(Object? value, String field) {
    if (value is! Map<String, Object?>) {
      throw FormatException('$field must be a JSON object');
    }
    return immutableJsonObject(value);
  }

  static String _string(Map<String, Object?> object, String field) {
    final value = object[field];
    if (value is! String || value.isEmpty) {
      throw FormatException('$field must be a non-empty string');
    }
    return value;
  }

  static String _stringOrDefault(Map<String, Object?> object, String field) {
    final value = object[field];
    if (value == null) {
      return '';
    }
    if (value is String) {
      return value;
    }
    throw FormatException('$field must be a string');
  }

  static bool _bool(Map<String, Object?> object, String field) {
    final value = object[field];
    if (value is! bool) {
      throw FormatException('$field must be a boolean');
    }
    return value;
  }

  static int _int(Map<String, Object?> object, String field) {
    final value = object[field];
    if (value is! int) {
      throw FormatException('$field must be an integer');
    }
    return value;
  }

  static T _enumByName<T extends Enum>(
    List<T> values,
    String name,
    String field,
  ) {
    for (final value in values) {
      if (value.name == name) {
        return value;
      }
    }
    throw FormatException('Unsupported $field value: $name');
  }
}
