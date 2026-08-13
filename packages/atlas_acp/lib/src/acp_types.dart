import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';

/// The ACP protocol version implemented by this adapter.
const acpProtocolVersion = 1;

/// The agent version reported during initialization.
const acpAgentVersion = '0.1.0';

/// One `session/update` notification payload.
final class SessionUpdate {
  /// Creates a session update notification.
  const SessionUpdate(this.sessionId, this.update);

  /// The owning session.
  final SessionId sessionId;

  /// The typed update object.
  final JsonObject update;

  /// Serializes the full JSON-RPC notification for the wire.
  JsonObject toJson() => {
    'jsonrpc': '2.0',
    'method': 'session/update',
    'params': {'sessionId': sessionId.value, 'update': update},
  };

  /// The JSON-encoded notification.
  String toJsonString() => jsonEncode(toJson());
}

/// Builds the `initialize` result with the capabilities Atlas provides.
JsonObject initializeResult() => {
  'protocolVersion': acpProtocolVersion,
  'agentCapabilities': {
    'loadSession': true,
    'sessionCapabilities': {
      'resume': <String, Object?>{},
      'list': <String, Object?>{},
      'close': <String, Object?>{},
      'additionalDirectories': <String, Object?>{},
    },
    'promptCapabilities': <String, Object?>{},
  },
  'agentInfo': {'name': 'atlas', 'title': 'Atlas', 'version': acpAgentVersion},
  'authMethods': <Object?>[],
};

/// A replayed user message (`user_message_chunk`).
SessionUpdate userMessageChunk(
  SessionId sessionId, {
  required String messageId,
  required String text,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'user_message_chunk',
  'messageId': messageId,
  'content': {'type': 'text', 'text': text},
});

/// An assistant text chunk (`agent_message_chunk`).
SessionUpdate agentMessageChunk(
  SessionId sessionId, {
  required String messageId,
  required String text,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'agent_message_chunk',
  'messageId': messageId,
  'content': {'type': 'text', 'text': text},
});

/// A model reasoning chunk (`agent_thought_chunk`).
SessionUpdate agentThoughtChunk(
  SessionId sessionId, {
  required String messageId,
  required String text,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'agent_thought_chunk',
  'messageId': messageId,
  'content': {'type': 'text', 'text': text},
});

/// A pending tool call (`tool_call`).
SessionUpdate toolCall(
  SessionId sessionId, {
  required String toolCallId,
  required String title,
  required String kind,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'tool_call',
  'toolCallId': toolCallId,
  'title': title,
  'kind': kind,
  'status': 'pending',
});

/// A tool call progress or result update (`tool_call_update`).
SessionUpdate toolCallUpdate(
  SessionId sessionId, {
  required String toolCallId,
  required String status,
  String? content,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'tool_call_update',
  'toolCallId': toolCallId,
  'status': status,
  if (content != null && content.isNotEmpty)
    'content': [
      {
        'type': 'content',
        'content': {'type': 'text', 'text': content},
      },
    ],
});

/// A complete plan replacement (`plan`).
SessionUpdate planUpdate(SessionId sessionId, List<JsonObject> entries) =>
    SessionUpdate(sessionId, {'sessionUpdate': 'plan', 'entries': entries});

/// Maps an Atlas tool name to the ACP tool kind.
String toolCallKind(String name) => switch (name) {
  'read' => 'read',
  'write' || 'edit' => 'edit',
  'shell' => 'execute',
  'plan' => 'think',
  _ => 'other',
};

/// Converts the `plan` tool argument list into ACP plan entries, or returns
/// `null` when [rawPlan] is not a well-formed plan payload.
///
/// Atlas plans carry no priority, so entries are reported with the neutral
/// `medium` priority.
List<JsonObject>? planEntries(Object? rawPlan) {
  if (rawPlan is! List || rawPlan.isEmpty) {
    return null;
  }
  final entries = <JsonObject>[];
  for (final raw in rawPlan) {
    if (raw is! Map) {
      return null;
    }
    final step = raw['step'];
    final status = raw['status'];
    if (step is! String || step.trim().isEmpty) {
      return null;
    }
    if (status != 'pending' &&
        status != 'in_progress' &&
        status != 'completed') {
      return null;
    }
    entries.add({
      'content': step.trim(),
      'priority': 'medium',
      'status': status,
    });
  }
  return entries;
}
