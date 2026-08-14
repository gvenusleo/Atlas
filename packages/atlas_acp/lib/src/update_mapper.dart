import 'package:atlas_runtime/atlas_runtime.dart';

import 'acp_types.dart';

/// Maps live runtime events for one prompt turn into ACP updates.
///
/// Keeps per-turn state: the message id shared by chunks of the current
/// assistant message, and the tool call ids that were reported as plan
/// updates instead of ordinary tool calls.
final class TurnUpdateMapper {
  /// Creates a mapper for one turn of [sessionId].
  TurnUpdateMapper(this.sessionId);

  /// The session being mapped.
  final SessionId sessionId;

  String? _messageId;
  int _messageCounter = 0;
  final _planCallIds = <String>{};
  final _shellCallIds = <String>{};

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
        return [
          toolCallUpdate(
            sessionId,
            toolCallId: result.callId.value,
            status: result.isError ? 'failed' : 'completed',
            content: isShell
                ? shellTerminalContent(result.callId.value)
                : (result.content.isEmpty
                      ? null
                      : textToolCallContent(result.content)),
            rawOutput: result.isError ? null : _toolRawOutput(result.content),
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

  /// Builds the pending `tool_call` update for [item], recording shell calls
  /// so their results can keep the display-only terminal reference.
  SessionUpdate _toolCallUpdate(ToolCallItem item) {
    final isShell = _isShell(item.call.name);
    if (isShell) {
      _shellCallIds.add(item.call.id.value);
    }
    return toolCall(
      sessionId,
      toolCallId: item.call.id.value,
      title: toolCallTitle(item.call.name, item.call.arguments),
      kind: toolCallKind(item.call.name),
      rawInput: item.call.arguments,
      locations: toolCallLocations(item.call.name, item.call.arguments),
      content: isShell ? shellTerminalContent(item.call.id.value) : null,
      meta: isShell
          ? shellTerminalInfo(item.call.id.value, item.call.arguments)
          : null,
    );
  }

  static bool _isShell(String name) => name == 'shell';

  String _messageIdFor(TurnId turnId) =>
      _messageId ??= 'msg-${turnId.value}-${_messageCounter++}';
}

/// Converts a persisted timeline into the `session/update` replay stream
/// required by `session/load`.
///
/// Message ids come from the durable timeline item ids; plan tool calls are
/// replayed as plan updates and their results are skipped.
List<SessionUpdate> replayTimeline(List<TimelineItem> timeline) {
  final updates = <SessionUpdate>[];
  // Call ids of plan tool calls whose result is skipped. Results always
  // follow their owning call in the timeline, so a FIFO queue matches them
  // even when the model reuses a call id across turns.
  final pendingPlanResults = <String>[];
  // Call ids of shell tool calls, whose results keep the display-only
  // terminal reference instead of a text content block.
  final pendingShellResults = <String>[];
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
          if (isShell) {
            pendingShellResults.add(call.id.value);
          }
          updates.add(
            toolCall(
              item.sessionId,
              toolCallId: call.id.value,
              title: toolCallTitle(call.name, call.arguments),
              kind: toolCallKind(call.name),
              rawInput: call.arguments,
              locations: toolCallLocations(call.name, call.arguments),
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
        final exitCode = (item.metadata['exit_code'] as num?)?.toInt();
        updates.add(
          toolCallUpdate(
            item.sessionId,
            toolCallId: callId.value,
            status: isError ? 'failed' : 'completed',
            content: isShell
                ? shellTerminalContent(callId.value)
                : (content.isEmpty ? null : textToolCallContent(content)),
            rawOutput: isError ? null : _toolRawOutput(content),
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
