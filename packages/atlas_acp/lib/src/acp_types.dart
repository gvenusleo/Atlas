import 'dart:io';

import 'package:acpd/acpd.dart' hide PlanEntry, ToolCall;
import 'package:atlas_runtime/atlas_runtime.dart' as rt;

/// The ACP protocol version implemented by this adapter.
const acpProtocolVersion = 1;

/// The config option identifier for model selection.
const acpConfigIdModel = 'model';

/// The config option identifier for reasoning effort selection.
const acpConfigIdEffort = 'effort';

/// The config option identifier for session mode selection.
const acpConfigIdMode = 'mode';

/// Atlas extension method for renaming a session. ACP v1 has no rename
/// request; clients that do not implement this method keep a local title
/// overlay instead.
const acpSessionSetTitleMethod = '_atlas.dev/session/set_title';

/// The agent version reported during initialization.
const acpAgentVersion = '0.1.0';

/// The client name reported during initialization.
const acpClientName = 'atlas';

/// Builds the session `configOptions` list for [models] with [currentModel]
/// selected and [currentEffort] as the current reasoning effort.
///
/// Model values use the `<provider>/<model>` reference so clients can select
/// across providers. The reasoning effort option is only offered when the
/// current model declares supported efforts, matching the ACP `thought_level`
/// category.
List<SessionConfigOption> sessionConfigOptions(
  List<rt.ModelDescriptor> models,
  rt.ModelRef currentModel,
  String? currentEffort,
) => [
  _modelOption(models, currentModel),
  ..._effortOptions(models, currentModel, currentEffort),
];

/// The model select option, with [currentModel] guaranteed present so the
/// current value always has a matching option even when the catalog is empty
/// or omits the default model.
SessionConfigOption _modelOption(
  List<rt.ModelDescriptor> models,
  rt.ModelRef currentModel,
) {
  final catalog = _catalogFor(models, currentModel);
  return SessionConfigSelectOptionValue(
    id: acpConfigIdModel,
    name: 'Model',
    description: 'The model used for this session',
    category: SessionConfigOptionCategory.model,
    currentValue: currentModel.toString(),
    options: SessionConfigUngroupedOptions([
      for (final model in catalog)
        SessionConfigSelectOption(
          value: model.ref.toString(),
          name: model.name.isEmpty ? model.ref.modelId.value : model.name,
          description: model.description.isEmpty ? null : model.description,
        ),
    ]),
  );
}

/// The reasoning effort select option for [currentModel], or an empty list
/// when the model declares no supported efforts.
List<SessionConfigOption> _effortOptions(
  List<rt.ModelDescriptor> models,
  rt.ModelRef currentModel,
  String? currentEffort,
) {
  final descriptor = models.firstWhere(
    (model) => model.ref == currentModel,
    orElse: () => rt.ModelDescriptor(ref: currentModel),
  );
  if (descriptor.reasoningEfforts.isEmpty) {
    return const [];
  }
  return [
    SessionConfigSelectOptionValue(
      id: acpConfigIdEffort,
      name: 'Effort',
      description: 'Available effort levels for this model',
      category: SessionConfigOptionCategory.thoughtLevel,
      currentValue: currentEffort ?? descriptor.reasoningEfforts.first.value,
      options: SessionConfigUngroupedOptions([
        for (final effort in descriptor.reasoningEfforts)
          SessionConfigSelectOption(
            value: effort.value,
            name: effort.name.isEmpty ? effort.value : effort.name,
            description: effort.description.isEmpty ? null : effort.description,
          ),
      ]),
    ),
  ];
}

/// The model catalog with [currentModel] guaranteed present.
List<rt.ModelDescriptor> _catalogFor(
  List<rt.ModelDescriptor> models,
  rt.ModelRef currentModel,
) {
  if (models.any((model) => model.ref == currentModel)) {
    return models;
  }
  return [...models, rt.ModelDescriptor(ref: currentModel)];
}

/// Builds the `initialize` result with the capabilities Atlas provides.
InitializeResponse initializeResult() => InitializeResponse(
  protocolVersion: ProtocolVersion.v1,
  agentCapabilities: AgentCapabilities(
    loadSession: true,
    sessionCapabilities: SessionCapabilities(
      resume: SessionResumeCapabilities(),
      list: SessionListCapabilities(),
      close: SessionCloseCapabilities(),
      delete: SessionDeleteCapabilities(),
      additionalDirectories: SessionAdditionalDirectoriesCapabilities(),
    ),
    promptCapabilities: PromptCapabilities(image: true, embeddedContext: true),
    meta: {
      'atlas.dev': {
        'setTitle': true,
        'compact': true,
        'permissionModel': 'none',
      },
    },
  ),
  authMethods: const [],
  agentInfo: Implementation(
    name: 'atlas',
    title: 'Atlas',
    version: acpAgentVersion,
  ),
);

/// The display-only terminal id for a shell tool call. Derived from the
/// tool call id so it stays unique within the session.
String shellTerminalId(String toolCallId) => 'term-$toolCallId';

/// The terminal content block for a shell tool call, which makes clients
/// render the call as a live terminal instead of a collapsed card.
ToolCallContent terminalToolCallContent(String toolCallId) =>
    ToolCallTerminal(terminalId: shellTerminalId(toolCallId));

/// The `_meta` registration for Zed's display-only terminal: a v1 extension
/// (not part of the ACP v1 spec) that lets a locally-executed command render
/// as a live terminal in Zed.
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
/// and exit status, streamed to the display-only terminal.
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

/// The built-in slash command that manually compacts the session context.
const compactCommandName = 'compact';

/// Builds the slash commands offered in one session: the built-in `/compact`
/// command followed by one command per available skill, skipping names that
/// collide with `/compact` or cannot be represented as slash commands.
List<AvailableCommand> availableCommandsFor(List<rt.SkillSummary> skills) => [
  AvailableCommand(
    name: compactCommandName,
    description: 'Compact earlier conversation context.',
  ),
  for (final summary in skills)
    if (summary.name != compactCommandName &&
        validSlashCommandName(summary.name))
      AvailableCommand(
        name: summary.name,
        description: summary.description,
        input: UnstructuredCommandInput(hint: 'task'),
      ),
];

/// Whether [name] can be safely exposed as a slash command name.
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
ToolKind toolCallKind(String name) => switch (name) {
  'read' => ToolKind.read,
  'write' || 'edit' => ToolKind.edit,
  'shell' => ToolKind.execute,
  'plan' => ToolKind.think,
  _ => ToolKind.other,
};

/// The maximum length of a shell command shown in a `tool_call` title.
const shellTitleLimit = 1000;

/// The short human-readable `tool_call` title for [name], shown by clients as
/// the headline of the tool call.
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
        final List<Map<String, Object?>> plan =>
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
ToolCallDiff diffToolCallContent({
  required String path,
  required String? oldText,
  required String newText,
}) => ToolCallDiff(path: path, oldText: oldText, newText: newText);

/// The absolute file locations affected by a tool call, extracted from its
/// arguments for ACP `locations` follow-along support.
List<ToolCallLocation> toolCallLocations(
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
    return [ToolCallLocation(path: absolute, line: offset.toInt())];
  }
  return [ToolCallLocation(path: absolute)];
}

/// Resolves [path] against [workingDirectory] when it is relative and a
/// working directory is known; absolute paths pass through unchanged.
String _absolutePath(String path, String? workingDirectory) {
  if (workingDirectory == null || workingDirectory.isEmpty) {
    return path;
  }
  if (path.startsWith('/') || RegExp(r'^[A-Za-z]:[\\/]').hasMatch(path)) {
    return path;
  }
  final separator = Platform.pathSeparator;
  return '$workingDirectory$separator$path';
}

/// Converts the `plan` tool argument list into ACP plan entries, or returns
/// `null` when [rawPlan] is not a well-formed plan payload.
List<Map<String, Object?>>? planEntries(Object? rawPlan) {
  if (rawPlan is! List || rawPlan.isEmpty) {
    return null;
  }
  final entries = <Map<String, Object?>>[];
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
