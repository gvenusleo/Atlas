import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';

/// The ACP protocol version implemented by this adapter.
const acpProtocolVersion = 1;

/// The config option identifier for model selection.
const acpConfigIdModel = 'model';

/// The config option identifier for reasoning effort selection.
const acpConfigIdReasoningEffort = 'reasoning_effort';

/// Builds the session `configOptions` list for [models] with [currentModel]
/// selected and [currentEffort] as the current reasoning effort.
///
/// Model values use the `<provider>/<model>` reference so clients can select
/// across providers. The reasoning effort option is only offered when the
/// current model declares supported efforts, matching the ACP `thought_level`
/// category.
List<JsonObject> sessionConfigOptions(
  List<ModelDescriptor> models,
  ModelRef currentModel,
  String? currentEffort,
) => [
  {
    'id': acpConfigIdModel,
    'name': 'Model',
    'description': 'The model used for this session',
    'category': 'model',
    'type': 'select',
    'currentValue': currentModel.toString(),
    'options': [
      for (final model in _catalogFor(models, currentModel))
        {
          'value': model.ref.toString(),
          'name': model.name.isEmpty ? model.ref.modelId.value : model.name,
          if (model.description.isNotEmpty) 'description': model.description,
        },
    ],
  },
  ?_reasoningEffortOption(models, currentModel, currentEffort),
];

/// The model catalog with [currentModel] guaranteed present, so the select
/// option always offers a matching entry for its current value even when the
/// catalog is empty or omits the default model.
List<ModelDescriptor> _catalogFor(
  List<ModelDescriptor> models,
  ModelRef currentModel,
) {
  if (models.any((model) => model.ref == currentModel)) {
    return models;
  }
  return [...models, ModelDescriptor(ref: currentModel)];
}

/// Builds the reasoning effort config option for [currentModel], or returns
/// null when the model declares no supported efforts.
JsonObject? _reasoningEffortOption(
  List<ModelDescriptor> models,
  ModelRef currentModel,
  String? currentEffort,
) {
  final descriptor = models.firstWhere(
    (model) => model.ref == currentModel,
    orElse: () => ModelDescriptor(ref: currentModel),
  );
  if (descriptor.reasoningEfforts.isEmpty) {
    return null;
  }
  return {
    'id': acpConfigIdReasoningEffort,
    'name': 'Reasoning effort',
    'description': 'Controls model reasoning depth',
    'category': 'thought_level',
    'type': 'select',
    'currentValue': currentEffort ?? descriptor.reasoningEfforts.first.value,
    'options': [
      for (final effort in descriptor.reasoningEfforts)
        {
          'value': effort.value,
          'name': effort.name.isEmpty ? effort.value : effort.name,
          if (effort.description.isNotEmpty) 'description': effort.description,
        },
    ],
  };
}

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
      'delete': <String, Object?>{},
      'additionalDirectories': <String, Object?>{},
    },
    'promptCapabilities': {'image': true, 'embeddedContext': true},
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
  Map<String, Object?>? rawInput,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'tool_call',
  'toolCallId': toolCallId,
  'title': title,
  'kind': kind,
  'status': 'pending',
  'rawInput': ?rawInput,
});

/// A tool call progress or result update (`tool_call_update`).
SessionUpdate toolCallUpdate(
  SessionId sessionId, {
  required String toolCallId,
  required String status,
  String? content,
  Map<String, Object?>? rawOutput,
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
  'rawOutput': ?rawOutput,
});

/// A complete plan replacement (`plan`).
SessionUpdate planUpdate(SessionId sessionId, List<JsonObject> entries) =>
    SessionUpdate(sessionId, {'sessionUpdate': 'plan', 'entries': entries});

/// A session metadata change (`session_info_update`).
SessionUpdate sessionInfoUpdate(SessionId sessionId, {required String title}) =>
    SessionUpdate(sessionId, {
      'sessionUpdate': 'session_info_update',
      'title': title,
    });

/// A context usage report (`usage_update`).
SessionUpdate usageUpdate(
  SessionId sessionId, {
  required int used,
  required int size,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'usage_update',
  'used': used,
  'size': size,
});

/// The slash commands available in a session (`available_commands_update`).
SessionUpdate availableCommandsUpdate(
  SessionId sessionId,
  List<JsonObject> commands,
) => SessionUpdate(sessionId, {
  'sessionUpdate': 'available_commands_update',
  'availableCommands': commands,
});

/// The built-in slash command that manually compacts the session context.
const compactCommandName = 'compact';

/// Builds the slash commands offered in one session: the built-in `/compact`
/// command followed by one command per available skill, skipping names that
/// collide with `/compact` or cannot be represented as slash commands.
///
/// `/compact` carries no input hint: Atlas forwards any trailing instruction
/// text to the compaction summary request.
List<JsonObject> availableCommandsFor(List<SkillSummary> skills) => [
  {
    'name': compactCommandName,
    'description': 'Compact earlier conversation context.',
  },
  for (final summary in skills)
    if (summary.name != compactCommandName &&
        validSlashCommandName(summary.name))
      {
        'name': summary.name,
        'description': summary.description,
        'input': {'hint': 'task'},
      },
];

/// Whether [name] can be safely exposed as a slash command name.
///
/// Restricted to ASCII letters, digits, `_`, `-`, and `.` so a command token
/// can always be parsed out of a prompt without escaping.
bool validSlashCommandName(String name) {
  if (name.isEmpty) {
    return false;
  }
  for (final code in name.codeUnits) {
    final isLetter =
        (code >= 0x41 && code <= 0x5A) || (code >= 0x61 && code <= 0x7A);
    final isDigit = code >= 0x30 && code <= 0x39;
    if (!isLetter && !isDigit && code != 0x5F && code != 0x2D && code != 0x2E) {
      return false;
    }
  }
  return true;
}

/// Parses the name from a `/name` token, or returns null when [text] is not
/// a well-formed slash command token.
String? slashCommandName(String text) {
  final trimmed = text.trim();
  if (!trimmed.startsWith('/')) {
    return null;
  }
  final withoutSlash = trimmed.substring(1);
  if (withoutSlash.isEmpty) {
    return null;
  }
  final index = withoutSlash.indexOf(RegExp(r'\s'));
  final name = index < 0 ? withoutSlash : withoutSlash.substring(0, index);
  if (!validSlashCommandName(name)) {
    return null;
  }
  return name;
}

/// Parses a `/compact [instruction]` command from [text].
///
/// Returns the trailing instruction text (possibly empty) when [text] is a
/// compact command, or null when it is a regular prompt. The instruction is
/// forwarded to the runtime compaction summary request when non-empty.
String? compactCommandInstruction(String text) {
  final trimmed = text.trim();
  if (trimmed == '/$compactCommandName') {
    return '';
  }
  if (RegExp('^/$compactCommandName\\s').hasMatch(trimmed)) {
    return trimmed.substring(compactCommandName.length + 1).trim();
  }
  return null;
}

/// Scans [text] for whitespace-separated `/name` tokens matching a known
/// command name, in order of first appearance and deduplicated.
List<String> matchedCommandNames(String text, Set<String> known) {
  final result = <String>[];
  final seen = <String>{};
  for (final field in text.split(RegExp(r'\s+'))) {
    final name = slashCommandName(field);
    if (name == null || !known.contains(name) || !seen.add(name)) {
      continue;
    }
    result.add(name);
  }
  return result;
}

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
