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
        return [
          for (final item in toolCalls)
            toolCall(
              sessionId,
              toolCallId: item.call.id.value,
              title: toolCallTitle(item.call.name),
              kind: toolCallKind(item.call.name),
              rawInput: item.call.arguments,
              locations: toolCallLocations(item.call.name, item.call.arguments),
            ),
        ];
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
          ),
        ];
      case ToolFinished(:final result):
        if (_planCallIds.contains(result.callId.value)) {
          return const [];
        }
        return [
          toolCallUpdate(
            sessionId,
            toolCallId: result.callId.value,
            status: result.isError ? 'failed' : 'completed',
            content: result.content,
            rawOutput: result.isError ? null : _toolRawOutput(result.content),
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
          updates.add(
            toolCall(
              item.sessionId,
              toolCallId: call.id.value,
              title: toolCallTitle(call.name),
              kind: toolCallKind(call.name),
              rawInput: call.arguments,
              locations: toolCallLocations(call.name, call.arguments),
            ),
          );
        }
      case ToolResultItem(:final callId, :final content, :final isError):
        if (pendingPlanResults.isNotEmpty &&
            pendingPlanResults.first == callId.value) {
          pendingPlanResults.removeAt(0);
          break;
        }
        updates.add(
          toolCallUpdate(
            item.sessionId,
            toolCallId: callId.value,
            status: isError ? 'failed' : 'completed',
            content: content,
            rawOutput: isError ? null : _toolRawOutput(content),
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
