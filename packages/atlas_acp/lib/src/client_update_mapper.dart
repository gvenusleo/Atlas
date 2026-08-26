import 'package:acpd/acpd.dart';
import 'package:atlas_runtime/atlas_runtime.dart' as rt;

/// Maps ACP `session/update` notifications back into runtime events.
///
/// ACP is a display-oriented protocol, so the reconstruction is lossy:
/// sequence numbers and timestamps are regenerated locally, and tool names
/// are derived from the ACP tool kind. The events cover what presentation
/// code consumes: text and reasoning deltas, tool start/finish, plans, and
/// compaction outcomes.
final class ClientUpdateMapper {
  /// Creates a mapper for one turn of [sessionId] with [turnId].
  ClientUpdateMapper(this.sessionId, this.turnId);

  /// The session being mapped.
  final rt.SessionId sessionId;

  /// The turn being mapped.
  final rt.TurnId turnId;

  int _sequence = 0;

  /// The next event sequence number.
  int nextSequence() => _sequence++;

  /// Converts [update] into zero or more runtime events.
  List<rt.AgentEvent> map(SessionUpdate update) {
    switch (update) {
      case AgentMessageChunk(:final chunk):
        return [
          rt.ModelTextDelta(
            sessionId: sessionId,
            turnId: turnId,
            sequence: nextSequence(),
            occurredAt: _now(),
            delta: _chunkText(chunk),
          ),
        ];
      case AgentThoughtChunk(:final chunk):
        return [
          rt.ModelReasoningDelta(
            sessionId: sessionId,
            turnId: turnId,
            sequence: nextSequence(),
            occurredAt: _now(),
            delta: _chunkText(chunk),
          ),
        ];
      case UserMessageChunk():
        return const [];
      case ToolCallUpdateSession(:final toolCall):
        return [
          rt.ToolStarted(
            sessionId: sessionId,
            turnId: turnId,
            sequence: nextSequence(),
            occurredAt: _now(),
            call: rt.ToolCallItem(
              id: rt.TimelineItemId(toolCall.toolCallId),
              sessionId: sessionId,
              turnId: turnId,
              sequence: nextSequence(),
              occurredAt: _now(),
              call: rt.ToolCall(
                id: rt.ToolCallId(toolCall.toolCallId),
                name: _nameFromKind(toolCall.kind),
                arguments: toolCall.rawInput is Map
                    ? Map<String, Object?>.from(toolCall.rawInput as Map)
                    : const <String, Object?>{},
              ),
            ),
          ),
        ];
      case ToolCallStatusUpdate(:final update):
        return [
          rt.ToolFinished(
            sessionId: sessionId,
            turnId: turnId,
            sequence: nextSequence(),
            occurredAt: _now(),
            result: rt.ToolResultItem(
              id: rt.TimelineItemId('result-${update.toolCallId}'),
              sessionId: sessionId,
              turnId: turnId,
              sequence: nextSequence(),
              occurredAt: _now(),
              callId: rt.ToolCallId(update.toolCallId),
              content: _outputText(update.rawOutput),
              isError: update.status == ToolCallStatus.failed,
            ),
          ),
        ];
      case PlanUpdate(:final plan):
        return [
          rt.PlanUpdated(
            sessionId: sessionId,
            turnId: turnId,
            sequence: nextSequence(),
            occurredAt: _now(),
            entries: List.unmodifiable([
              for (final entry in plan.entries)
                rt.PlanEntry(
                  content: entry.content,
                  priority: entry.priority.toJson(),
                  status: entry.status.toJson(),
                ),
            ]),
          ),
        ];
      default:
        return const [];
    }
  }

  static String _chunkText(ContentChunk chunk) {
    final content = chunk.content;
    if (content is TextContentBlock) {
      return content.text;
    }
    return '';
  }

  static String _outputText(Object? rawOutput) {
    if (rawOutput is Map && rawOutput['output'] is String) {
      return rawOutput['output'] as String;
    }
    return '';
  }

  static String _nameFromKind(ToolKind? kind) => switch (kind) {
    ToolKind.read => 'read',
    ToolKind.edit => 'edit',
    ToolKind.execute => 'shell',
    ToolKind.think => 'plan',
    _ => 'tool',
  };

  static DateTime _now() => DateTime.now().toUtc();
}

/// Reconstructs timeline items from the `session/load` replay stream.
///
/// Message ids come from the ACP message and tool call ids; turns are
/// collapsed into one synthetic turn per session because ACP does not expose
/// turn boundaries.
final class ClientTimelineMapper {
  /// Creates a timeline mapper for [sessionId].
  ClientTimelineMapper(this.sessionId);

  /// The session being mapped.
  final rt.SessionId sessionId;

  final _turnId = rt.TurnId('acp-session');
  int _sequence = 0;

  /// Converts one replayed [update] into timeline items.
  List<rt.TimelineItem> map(SessionUpdate update) {
    switch (update) {
      case UserMessageChunk(:final chunk):
        return [
          rt.UserMessageItem(
            id: rt.TimelineItemId(_messageId(chunk)),
            sessionId: sessionId,
            turnId: _turnId,
            sequence: _sequence++,
            occurredAt: _now(),
            content: [rt.TextContent(_chunkText(chunk))],
          ),
        ];
      case AgentMessageChunk(:final chunk):
        return [
          rt.AssistantMessageItem(
            id: rt.TimelineItemId(_messageId(chunk)),
            sessionId: sessionId,
            turnId: _turnId,
            sequence: _sequence++,
            occurredAt: _now(),
            content: [rt.TextContent(_chunkText(chunk))],
            model: _defaultModel,
            stopReason: rt.StopReason.endTurn,
          ),
        ];
      case AgentThoughtChunk(:final chunk):
        return [
          rt.AssistantMessageItem(
            id: rt.TimelineItemId(_messageId(chunk)),
            sessionId: sessionId,
            turnId: _turnId,
            sequence: _sequence++,
            occurredAt: _now(),
            content: const [],
            reasoning: _chunkText(chunk),
            model: _defaultModel,
            stopReason: rt.StopReason.endTurn,
          ),
        ];
      case ToolCallUpdateSession(:final toolCall):
        return [
          rt.ToolCallItem(
            id: rt.TimelineItemId(toolCall.toolCallId),
            sessionId: sessionId,
            turnId: _turnId,
            sequence: _sequence++,
            occurredAt: _now(),
            call: rt.ToolCall(
              id: rt.ToolCallId(toolCall.toolCallId),
              name: _nameFromKind(toolCall.kind),
              arguments: toolCall.rawInput is Map
                  ? Map<String, Object?>.from(toolCall.rawInput as Map)
                  : const <String, Object?>{},
            ),
          ),
        ];
      case ToolCallStatusUpdate(:final update):
        return [
          rt.ToolResultItem(
            id: rt.TimelineItemId('result-${update.toolCallId}'),
            sessionId: sessionId,
            turnId: _turnId,
            sequence: _sequence++,
            occurredAt: _now(),
            callId: rt.ToolCallId(update.toolCallId),
            content: _outputText(update.rawOutput),
            isError: update.status == ToolCallStatus.failed,
          ),
        ];
      default:
        return const [];
    }
  }

  static String _messageId(ContentChunk chunk) =>
      chunk.messageId ?? 'msg-${chunk.hashCode}';

  static String _chunkText(ContentChunk chunk) {
    final content = chunk.content;
    if (content is TextContentBlock) {
      return content.text;
    }
    return '';
  }

  static String _outputText(Object? rawOutput) {
    if (rawOutput is Map && rawOutput['output'] is String) {
      return rawOutput['output'] as String;
    }
    return '';
  }

  static String _nameFromKind(ToolKind? kind) => switch (kind) {
    ToolKind.read => 'read',
    ToolKind.edit => 'edit',
    ToolKind.execute => 'shell',
    ToolKind.think => 'plan',
    _ => 'tool',
  };

  static DateTime _now() => DateTime.now().toUtc();

  static final _defaultModel = rt.ModelRef(
    providerId: rt.ProviderId('acp'),
    modelId: rt.ModelId('default'),
  );
}
