import 'package:atlas_runtime/atlas_runtime.dart';

import 'acp_types.dart';

/// Maps live runtime events for one prompt turn into ACP updates.
///
/// Keeps per-turn state: the message id shared by chunks of the current
/// assistant message, and the tool call ids that were reported as plan
/// updates instead of ordinary tool calls.
final class TurnUpdateMapper {
  /// Creates a mapper for one turn of [sessionId]. [workingDirectory]
  /// resolves relative tool paths into absolute ACP locations; when null,
  /// paths are reported as given.
  TurnUpdateMapper(this.sessionId, {this.workingDirectory});

  /// The session being mapped.
  final SessionId sessionId;

  /// The session working directory used to absolutize file locations.
  final String? workingDirectory;

  String? _messageId;
  int _messageCounter = 0;
  final _planCallIds = <String>{};
  final _shellCallIds = <String>{};
  final _fileCallIds = <String>{};

  /// Converts [event] into zero or more `session/update` notifications.
  List<SessionUpdate> map(AgentEvent event) {
    switch (event) {
      case ModelTextDelta(:final delta):
        return [
          agentMessageChunk(
            sessionId,
            messageId: _messageIdFor(event.turnId),
            text: delta,
          ),
        ];
      case ModelReasoningDelta(:final delta):
        return [
          agentThoughtChunk(
            sessionId,
            messageId: _messageIdFor(event.turnId),
            text: delta,
          ),
        ];
      case ModelResponseReceived(:final toolCalls) when toolCalls.isNotEmpty:
        // A new assistant message starts after the previous response.
        _messageId = null;
        return [for (final item in toolCalls) _toolCallUpdate(item)];
      case ToolStarted(:final call):
        final plan = planEntries(call.call.arguments['plan']);
        if (call.call.name == 'plan' && plan != null) {
          _planCallIds.add(call.call.id.value);
          return [planUpdate(sessionId, plan)];
        }
        return [
          toolCallUpdate(
            sessionId,
            toolCallId: call.call.id.value,
            status: 'in_progress',
            // Keep the terminal reference for shell calls so Zed does not
            // replace the live terminal with a collapsed card.
            content: _isShell(call.call.name)
                ? shellTerminalContent(call.call.id.value)
                : null,
          ),
        ];
      case ToolFinished(:final result):
        if (_planCallIds.contains(result.callId.value)) {
          return const [];
        }
        final isShell = _shellCallIds.contains(result.callId.value);
        final exitCode = (result.metadata['exit_code'] as num?)?.toInt();
        final diff = _fileCallIds.contains(result.callId.value)
            ? _diffContent(result)
            : null;
        return [
          toolCallUpdate(
            sessionId,
            toolCallId: result.callId.value,
            status: result.isError ? 'failed' : 'completed',
            content: isShell
                ? shellTerminalContent(result.callId.value)
                : (diff ??
                      (result.content.isEmpty
                          ? null
                          : textToolCallContent(result.content))),
            rawOutput: result.isError ? null : _toolRawOutput(result.content),
            locations: diff != null ? _diffLocation(result) : null,
            meta: isShell
                ? shellTerminalUpdateMeta(
                    result.callId.value,
                    result.content,
                    exitCode,
                  )
                : null,
          ),
        ];
      case TurnStarted() ||
          ModelResponseReceived() ||
          CompactionStarted() ||
          CompactionFinished() ||
          CompactionFailed() ||
          TurnFinished():
        return const [];
    }
  }

  /// Builds the pending `tool_call` update for [item], recording shell and
  /// file tool calls so their results keep the terminal reference or render
  /// as diffs.
  SessionUpdate _toolCallUpdate(ToolCallItem item) {
    final isShell = _isShell(item.call.name);
    if (isShell) {
      _shellCallIds.add(item.call.id.value);
    }
    if (_isFileTool(item.call.name)) {
      _fileCallIds.add(item.call.id.value);
    }
    return toolCall(
      sessionId,
      toolCallId: item.call.id.value,
      title: toolCallTitle(item.call.name, item.call.arguments),
      kind: toolCallKind(item.call.name),
      rawInput: item.call.arguments,
      locations: toolCallLocations(
        item.call.name,
        item.call.arguments,
        workingDirectory: workingDirectory,
      ),
      content: isShell ? shellTerminalContent(item.call.id.value) : null,
      meta: isShell
          ? shellTerminalInfo(item.call.id.value, item.call.arguments)
          : null,
    );
  }

  static bool _isShell(String name) => name == 'shell';

  static bool _isFileTool(String name) => name == 'write' || name == 'edit';

  String _messageIdFor(TurnId turnId) =>
      _messageId ??= 'msg-${turnId.value}-${_messageCounter++}';
}

/// Converts a persisted timeline into the `session/update` replay stream
/// required by `session/load`.
///
/// Message ids come from the durable timeline item ids; plan tool calls are
/// replayed as plan updates and their results are skipped. [workingDirectory]
/// resolves relative tool paths into absolute ACP locations.
List<SessionUpdate> replayTimeline(
  List<TimelineItem> timeline, {
  String? workingDirectory,
}) {
  final updates = <SessionUpdate>[];
  // Call ids of plan tool calls whose result is skipped. Results always
  // follow their owning call in the timeline, so a FIFO queue matches them
  // even when the model reuses a call id across turns.
  final pendingPlanResults = <String>[];
  // Call ids of shell tool calls, whose results keep the display-only
  // terminal reference instead of a text content block.
  final pendingShellResults = <String>[];
  // Call ids of write/edit tool calls, whose results render as diffs.
  final pendingFileResults = <String>[];
  for (final item in timeline) {
    switch (item) {
      case UserMessageItem(:final content):
        updates.add(
          userMessageChunk(
            item.sessionId,
            messageId: item.id.value,
            text: textFromContent(content),
          ),
        );
      case AssistantMessageItem(:final content):
        updates.add(
          agentMessageChunk(
            item.sessionId,
            messageId: item.id.value,
            text: textFromContent(content),
          ),
        );
      case ToolCallItem(:final call):
        final plan = planEntries(call.arguments['plan']);
        if (call.name == 'plan' && plan != null) {
          pendingPlanResults.add(call.id.value);
          updates.add(planUpdate(item.sessionId, plan));
        } else {
          final isShell = call.name == 'shell';
          final isFile = call.name == 'write' || call.name == 'edit';
          if (isShell) {
            pendingShellResults.add(call.id.value);
          }
          if (isFile) {
            pendingFileResults.add(call.id.value);
          }
          updates.add(
            toolCall(
              item.sessionId,
              toolCallId: call.id.value,
              title: toolCallTitle(call.name, call.arguments),
              kind: toolCallKind(call.name),
              rawInput: call.arguments,
              locations: toolCallLocations(
                call.name,
                call.arguments,
                workingDirectory: workingDirectory,
              ),
              content: isShell ? shellTerminalContent(call.id.value) : null,
              meta: isShell
                  ? shellTerminalInfo(call.id.value, call.arguments)
                  : null,
            ),
          );
        }
      case ToolResultItem(:final callId, :final content, :final isError):
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
        final diff = isFile ? _diffContent(item) : null;
        final exitCode = (item.metadata['exit_code'] as num?)?.toInt();
        updates.add(
          toolCallUpdate(
            item.sessionId,
            toolCallId: callId.value,
            status: isError ? 'failed' : 'completed',
            content: isShell
                ? shellTerminalContent(callId.value)
                : (diff ??
                      (content.isEmpty ? null : textToolCallContent(content))),
            rawOutput: isError ? null : _toolRawOutput(content),
            locations: diff != null ? _diffLocation(item) : null,
            meta: isShell
                ? shellTerminalUpdateMeta(callId.value, content, exitCode)
                : null,
          ),
        );
    }
  }
  return updates;
}

/// Wraps a text tool result as the object `rawOutput` ACP expects; returns
/// null for empty results.
Map<String, Object?>? _toolRawOutput(String content) =>
    content.trim().isEmpty ? null : {'output': content};

/// Builds a `diff` content block from [result] metadata when a file tool
/// reported old and new contents; returns null for failures or non-file
/// tools.
List<JsonObject>? _diffContent(ToolResultItem result) {
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
List<JsonObject>? _diffLocation(ToolResultItem result) {
  final path = result.metadata['path'];
  final line = result.metadata['line'];
  if (path is! String) {
    return null;
  }
  return [
    if (line is num && line >= 1)
      {'path': path, 'line': line.toInt()}
    else
      {'path': path},
  ];
}
