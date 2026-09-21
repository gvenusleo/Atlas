import 'package:acpd/acpd.dart';
import 'package:atlas_runtime/atlas_runtime.dart' as rt;

import 'acp_types.dart';

/// Maps live runtime events for one prompt turn into ACP updates.
///
/// Keeps per-turn state: the message id shared by chunks of the current
/// assistant message, and the tool call ids that were reported as plan
/// updates instead of ordinary tool calls.
final class TurnUpdateMapper(
  /// The session being mapped.
  final rt.SessionId sessionId, {

  /// The session working directory used to absolutize file locations.
  final String? workingDirectory,
}) {
  /// Creates a mapper for one turn of [sessionId]. [workingDirectory]
  /// resolves relative tool paths into absolute ACP locations; when null,
  /// paths are reported as given.
  this;

  String? _messageId;
  int _messageCounter = 0;
  final _planCallIds = <String>{};
  final _shellCallIds = <String>{};
  final _fileCallIds = <String>{};

  /// Converts [event] into zero or more `session/update` notifications.
  List<SessionUpdate> map(rt.AgentEvent event) {
    switch (event) {
      case rt.ModelTextDelta(:final delta):
        return [
          AgentMessageChunk(
            chunk: ContentChunk(
              messageId: _messageIdFor(event.turnId),
              content: TextContentBlock(text: delta),
            ),
          ),
        ];
      case rt.ModelReasoningDelta(:final delta):
        return [
          AgentThoughtChunk(
            chunk: ContentChunk(
              messageId: _messageIdFor(event.turnId),
              content: TextContentBlock(text: delta),
            ),
          ),
        ];
      case rt.ModelResponseReceived(:final toolCalls) when toolCalls.isNotEmpty:
        // A new assistant message starts after the previous response.
        _messageId = null;
        return [for (final item in toolCalls) _toolCallUpdate(item)];
      case rt.ToolStarted(:final call):
        final plan = planEntries(call.call.arguments['plan']);
        if (call.call.name == 'plan' && plan != null) {
          _planCallIds.add(call.call.id.value);
          return [planUpdate(plan)];
        }
        // The pending tool call already carries the display terminal for shell
        // calls, so a status-only update leaves that terminal in place.
        if (_isShell(call.call.name)) {
          _shellCallIds.add(call.call.id.value);
        }
        return [
          ToolCallStatusUpdate(
            update: ToolCallUpdate(
              toolCallId: call.call.id.value,
              status: ToolCallStatus.inProgress,
            ),
          ),
        ];
      case rt.ToolOutputUpdated(:final callId, :final output):
        return [
          ToolCallStatusUpdate(
            update: ToolCallUpdate(
              toolCallId: callId.value,
              status: ToolCallStatus.inProgress,
              // Shell output renders through the client terminal, so its
              // snapshots travel in rawOutput only.
              content: _shellCallIds.contains(callId.value)
                  ? null
                  : _textOutput(output.content),
              rawOutput: {
                'output': output.content,
                'truncated': output.truncated,
                'total_bytes': output.totalBytes,
              },
            ),
          ),
        ];
      case rt.ToolFinished(:final result):
        if (_planCallIds.contains(result.callId.value)) {
          return const [];
        }
        final isShell = _shellCallIds.contains(result.callId.value);
        final exitCode = (result.metadata['exit_code'] as num?)?.toInt();
        final diff = !isShell && _fileCallIds.contains(result.callId.value)
            ? _diffContent(result)
            : null;
        return [
          ToolCallStatusUpdate(
            update: ToolCallUpdate(
              toolCallId: result.callId.value,
              status: result.isError
                  ? ToolCallStatus.failed
                  : ToolCallStatus.completed,
              content: isShell
                  ? [terminalToolCallContent(result.callId.value)]
                  : (diff != null ? [diff] : _textOutput(result.content)),
              rawOutput: _toolRawOutput(result.content, result.metadata),
              locations: diff != null ? _diffLocation(result) : null,
              meta: isShell
                  ? shellTerminalUpdateMeta(
                      result.callId.value,
                      result.content,
                      exitCode,
                    )
                  : null,
            ),
          ),
        ];
      case rt.TurnStarted() ||
          rt.ModelResponseReceived() ||
          rt.UsageUpdated() ||
          rt.PlanUpdated() ||
          rt.CompactionStarted() ||
          rt.CompactionFinished() ||
          rt.CompactionFailed() ||
          rt.TurnFinished():
        return const [];
    }
  }

  /// Builds a pending update and remembers shell and file calls, whose
  /// results keep the client terminal or render as diffs.
  SessionUpdate _toolCallUpdate(rt.ToolCallItem item) {
    final isShell = _isShell(item.call.name);
    if (isShell) {
      _shellCallIds.add(item.call.id.value);
    }
    if (_isFileTool(item.call.name)) {
      _fileCallIds.add(item.call.id.value);
    }
    return ToolCallUpdateSession(
      toolCall: ToolCall(
        toolCallId: item.call.id.value,
        title: toolCallTitle(item.call.name, item.call.arguments),
        kind: toolCallKind(item.call.name),
        status: ToolCallStatus.pending,
        rawInput: item.call.arguments,
        locations: toolCallLocations(
          item.call.name,
          item.call.arguments,
          workingDirectory: workingDirectory,
        ),
        content: isShell
            ? [terminalToolCallContent(item.call.id.value)]
            : const [],
        meta: isShell
            ? shellTerminalInfo(item.call.id.value, item.call.arguments)
            : null,
      ),
    );
  }

  static bool _isShell(String name) => name == 'shell';

  static bool _isFileTool(String name) => name == 'write' || name == 'edit';

  String _messageIdFor(rt.TurnId turnId) =>
      _messageId ??= 'msg-${turnId.value}-${_messageCounter++}';
}

/// Converts a persisted timeline into the `session/update` replay stream
/// required by `session/load`.
///
/// Message ids come from the durable timeline item ids; plan tool calls are
/// replayed as plan updates and their results are skipped. [workingDirectory]
/// resolves relative tool paths into absolute ACP locations.
List<SessionUpdate> replayTimeline(
  List<rt.TimelineItem> timeline, {
  String? workingDirectory,
}) {
  final updates = <SessionUpdate>[];
  // Call ids of plan tool calls whose result is skipped. Results always
  // follow their owning call in the timeline, so a FIFO queue matches them
  // even when the model reuses a call id across turns.
  final pendingPlanResults = <String>[];
  // Call ids of shell tool calls, whose results stream into the client
  // terminal.
  final pendingShellResults = <String>[];
  // Call ids of write/edit tool calls, whose results render as diffs.
  final pendingFileResults = <String>[];
  final resultCallIds = <(rt.TurnId, String)>{
    for (final item in timeline)
      if (item is rt.ToolResultItem) (item.turnId, item.callId.value),
  };
  for (final item in timeline) {
    switch (item) {
      case rt.UserMessageItem(:final content):
        updates.add(
          UserMessageChunk(
            chunk: ContentChunk(
              messageId: item.id.value,
              content: TextContentBlock(text: rt.textFromContent(content)),
            ),
          ),
        );
      case rt.AssistantMessageItem(:final content, :final reasoning):
        if (reasoning.isNotEmpty) {
          updates.add(
            AgentThoughtChunk(
              chunk: ContentChunk(
                messageId: item.id.value,
                content: TextContentBlock(text: reasoning),
              ),
            ),
          );
        }
        updates.add(
          AgentMessageChunk(
            chunk: ContentChunk(
              messageId: item.id.value,
              content: TextContentBlock(text: rt.textFromContent(content)),
            ),
          ),
        );
      case rt.ToolCallItem(:final call, :final turnId):
        final plan = planEntries(call.arguments['plan']);
        if (call.name == 'plan' && plan != null) {
          pendingPlanResults.add(call.id.value);
          updates.add(planUpdate(plan));
        } else {
          final isShell = call.name == 'shell';
          final isFile = call.name == 'write' || call.name == 'edit';
          final hasResult = resultCallIds.contains((turnId, call.id.value));
          if (isShell && hasResult) {
            pendingShellResults.add(call.id.value);
          }
          if (isFile && hasResult) {
            pendingFileResults.add(call.id.value);
          }
          updates.add(
            ToolCallUpdateSession(
              toolCall: ToolCall(
                toolCallId: call.id.value,
                title: toolCallTitle(call.name, call.arguments),
                kind: toolCallKind(call.name),
                status: hasResult
                    ? ToolCallStatus.pending
                    : ToolCallStatus.failed,
                rawInput: call.arguments,
                locations: toolCallLocations(
                  call.name,
                  call.arguments,
                  workingDirectory: workingDirectory,
                ),
                content: isShell && hasResult
                    ? [terminalToolCallContent(call.id.value)]
                    : (!hasResult
                          ? [
                              ToolCallContentBlock(
                                content: TextContentBlock(text: 'interrupted'),
                              ),
                            ]
                          : const []),
                meta: isShell && hasResult
                    ? shellTerminalInfo(call.id.value, call.arguments)
                    : null,
              ),
            ),
          );
        }
      case rt.ToolResultItem(:final callId, :final content, :final isError):
        if (pendingPlanResults.isNotEmpty &&
            pendingPlanResults.first == callId.value) {
          pendingPlanResults.removeAt(0);
          break;
        }
        final isShell =
            pendingShellResults.isNotEmpty &&
            pendingShellResults.first == callId.value;
        if (isShell) {
          pendingShellResults.removeAt(0);
        }
        final isFile =
            pendingFileResults.isNotEmpty &&
            pendingFileResults.first == callId.value;
        if (isFile) {
          pendingFileResults.removeAt(0);
        }
        final exitCode = (item.metadata['exit_code'] as num?)?.toInt();
        final diff = isFile ? _diffContent(item) : null;
        updates.add(
          ToolCallStatusUpdate(
            update: ToolCallUpdate(
              toolCallId: callId.value,
              status: isError
                  ? ToolCallStatus.failed
                  : ToolCallStatus.completed,
              content: isShell
                  ? [terminalToolCallContent(callId.value)]
                  : (diff != null ? [diff] : _textOutput(content)),
              rawOutput: _toolRawOutput(content, item.metadata),
              locations: diff != null ? _diffLocation(item) : null,
              meta: isShell
                  ? shellTerminalUpdateMeta(callId.value, content, exitCode)
                  : null,
            ),
          ),
        );
    }
  }
  return updates;
}

/// Builds a `plan` update from raw plan entries.
SessionUpdate planUpdate(List<Map<String, Object?>> entries) => PlanUpdate(
  plan: Plan(
    entries: [
      for (final entry in entries)
        PlanEntry(
          content: entry['content'] as String? ?? '',
          priority: _planPriority(entry['priority'] as String?),
          status: _planStatus(entry['status'] as String?),
        ),
    ],
  ),
);

PlanEntryPriority _planPriority(String? value) => switch (value) {
  'high' => PlanEntryPriority.high,
  'low' => PlanEntryPriority.low,
  _ => PlanEntryPriority.medium,
};

PlanEntryStatus _planStatus(String? value) => switch (value) {
  'in_progress' => PlanEntryStatus.inProgress,
  'completed' => PlanEntryStatus.completed,
  _ => PlanEntryStatus.pending,
};

/// Preserves final text and metadata in ACP, including errors and empty output.
Map<String, Object?> _toolRawOutput(String content, rt.JsonObject metadata) => {
  'output': content,
  if (metadata.isNotEmpty) 'metadata': metadata,
};

List<ToolCallContent> _textOutput(String content) => [
  ToolCallContentBlock(content: TextContentBlock(text: content)),
];

/// Builds a `diff` content block from [result] metadata when a file tool
/// reported old and new contents; returns null for failures or non-file
/// tools.
ToolCallDiff? _diffContent(rt.ToolResultItem result) {
  final path = result.metadata['path'];
  final newText = result.metadata['newText'];
  if (result.isError || path is! String || newText is! String) {
    return null;
  }
  final oldText = result.metadata['oldText'];
  return diffToolCallContent(
    path: path,
    oldText: oldText is String ? oldText : null,
    newText: newText,
  );
}

/// The follow-along location for a diff result, anchored at the first
/// replacement's line when the tool reported one.
List<ToolCallLocation>? _diffLocation(rt.ToolResultItem result) {
  final path = result.metadata['path'];
  final line = result.metadata['line'];
  if (path is! String) {
    return null;
  }
  return [
    if (line is num && line >= 1)
      ToolCallLocation(path: path, line: line.toInt())
    else
      ToolCallLocation(path: path),
  ];
}
