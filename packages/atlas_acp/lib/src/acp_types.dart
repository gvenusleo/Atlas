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
  List<JsonObject>? locations,
  List<JsonObject>? content,
  Map<String, Object?>? meta,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'tool_call',
  'toolCallId': toolCallId,
  'title': title,
  'kind': kind,
  'status': 'pending',
  'rawInput': ?rawInput,
  if (locations != null && locations.isNotEmpty) 'locations': locations,
  if (content != null && content.isNotEmpty) 'content': content,
  if (meta != null && meta.isNotEmpty) '_meta': meta,
});

/// A tool call progress or result update (`tool_call_update`).
SessionUpdate toolCallUpdate(
  SessionId sessionId, {
  required String toolCallId,
  required String status,
  List<JsonObject>? content,
  List<JsonObject>? locations,
  Map<String, Object?>? rawOutput,
  Map<String, Object?>? meta,
}) => SessionUpdate(sessionId, {
  'sessionUpdate': 'tool_call_update',
  'toolCallId': toolCallId,
  'status': status,
  if (content != null && content.isNotEmpty) 'content': content,
  if (locations != null && locations.isNotEmpty) 'locations': locations,
  'rawOutput': ?rawOutput,
  if (meta != null && meta.isNotEmpty) '_meta': meta,
});

/// A standard text content block for tool call output.
List<JsonObject> textToolCallContent(String text) => [
  {
    'type': 'content',
    'content': {'type': 'text', 'text': text},
  },
];

/// The display-only terminal id for a shell tool call. Derived from the
/// tool call id so it stays unique within the session.
String shellTerminalId(String toolCallId) => 'term-$toolCallId';

/// The terminal content block for a shell tool call, which makes Zed render
/// the call as a live terminal instead of a collapsed card.
List<JsonObject> shellTerminalContent(String toolCallId) => [
  {'type': 'terminal', 'terminalId': shellTerminalId(toolCallId)},
];

/// The `_meta` registration for Zed's display-only terminal: a v1 extension
/// (not part of the ACP v1 spec) that lets a locally-executed command render
/// as a live terminal in Zed. ACP v2 standardizes the same capability as
/// `terminal_update` / `terminal_output_chunk`; migrate there when Atlas
/// moves to v2.
Map<String, Object?> shellTerminalInfo(
  String toolCallId,
  Map<String, Object?> arguments,
) {
  final cwd = arguments['cwd'];
  return {
    'terminal_info': {
      'terminal_id': shellTerminalId(toolCallId),
      if (cwd is String && cwd.isNotEmpty) 'cwd': cwd,
    },
  };
}

/// The `_meta` payload for a finished shell tool call: the captured output
/// and exit status, streamed to Zed's display-only terminal. [output] is the
/// raw text (Zed's v1 extension carries it as a JSON string, not base64).
Map<String, Object?> shellTerminalUpdateMeta(
  String toolCallId,
  String output,
  int? exitCode,
) => {
  'terminal_output': {
    'terminal_id': shellTerminalId(toolCallId),
    'data': output,
  },
  'terminal_exit': {
    'terminal_id': shellTerminalId(toolCallId),
    'exit_code': ?exitCode,
  },
};

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

/// The maximum length of a shell command shown in a `tool_call` title.
const shellTitleLimit = 1000;

/// The short human-readable `tool_call` title for [name], shown by clients as
/// the headline of the tool call. ACP titles describe what the tool is doing
/// rather than duplicating the model-facing description.
///
/// Shell calls use the command itself as the title (truncated to
/// [shellTitleLimit] code units with an ellipsis); file tools prefix their
/// target path; and the plan tool reports completed steps over the total.
/// Titles fall back to a fixed phrase or the tool name when the arguments do
/// not carry enough information.
String toolCallTitle(String name, Map<String, Object?> arguments) =>
    switch (name) {
      'shell' => switch (arguments['command']) {
        final String command when command.trim().isNotEmpty => _truncateCommand(
          command,
        ),
        _ => 'Run shell command',
      },
      'read' || 'write' || 'edit' => switch (arguments['path']) {
        final String path when path.trim().isNotEmpty =>
          '${_titleCase(name)}: $path',
        _ => '${_titleCase(name)} file',
      },
      'plan' => switch (planEntries(arguments['plan'])) {
        final List<JsonObject> plan =>
          'Plan: '
              '${plan.where((entry) => entry['status'] == 'completed').length}'
              '/${plan.length} completed',
        _ => 'Update plan',
      },
      _ => name,
    };

/// Capitalizes the first letter of [name] for display titles.
String _titleCase(String name) => name[0].toUpperCase() + name.substring(1);

/// Truncates [command] to [shellTitleLimit] code units, appending an
/// ellipsis (U+2026) when it is longer.
String _truncateCommand(String command) {
  if (command.codeUnits.length <= shellTitleLimit) {
    return command;
  }
  return '${command.substring(0, shellTitleLimit)}…';
}

/// A `diff` content block for a file modification shown by clients as a
/// before/after view. [oldText] is null for newly created files.
List<JsonObject> diffToolCallContent({
  required String path,
  required String? oldText,
  required String newText,
}) => [
  {'type': 'diff', 'path': path, 'oldText': oldText, 'newText': newText},
];

/// The absolute file locations affected by a tool call, extracted from its
/// arguments for ACP `locations` follow-along support.
///
/// Only tools that act on a single `path` argument report locations; shell
/// commands and plan updates operate on no file and return an empty list.
/// Relative paths are resolved against [workingDirectory] when one is known;
/// reads that start at an explicit `offset` also report the start line.
List<JsonObject> toolCallLocations(
  String name,
  Map<String, Object?> arguments, {
  String? workingDirectory,
}) {
  if (name != 'read' && name != 'write' && name != 'edit') {
    return const [];
  }
  final path = arguments['path'];
  if (path is! String || path.isEmpty) {
    return const [];
  }
  final absolute = _absolutePath(path, workingDirectory);
  final offset = arguments['offset'];
  if (name == 'read' && offset is num && offset >= 1) {
    return [
      {'path': absolute, 'line': offset.toInt()},
    ];
  }
  return [
    {'path': absolute},
  ];
}

/// Resolves [path] against [workingDirectory] when it is relative and a
/// working directory is known; absolute paths pass through unchanged.
String _absolutePath(String path, String? workingDirectory) {
  if (workingDirectory == null || workingDirectory.isEmpty) {
    return path;
  }
  return path.startsWith('/') ? path : '$workingDirectory/$path';
}

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
