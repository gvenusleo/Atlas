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
  final _toolOutputs = <String, _ToolOutput>{};

  /// Tool calls already announced with a [rt.ToolStarted] event.
  final _startedCalls = <String>{};

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
        _rememberToolOutput(
          _toolOutputs,
          ToolCallUpdate(
            toolCallId: toolCall.toolCallId,
            content: toolCall.content,
            rawOutput: toolCall.rawOutput,
          ),
        );
        _startedCalls.add(toolCall.toolCallId);
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
        final output = _rememberToolOutput(_toolOutputs, update);
        // Status updates arrive repeatedly: an in-progress report (sent right
        // after the call is announced) must not finish the card, and only a
        // completed/failed status carries the result.
        if (update.status == ToolCallStatus.completed ||
            update.status == ToolCallStatus.failed) {
          _startedCalls.remove(update.toolCallId);
          _toolOutputs.remove(update.toolCallId);
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
                content: output.snapshot.content,
                isError: update.status == ToolCallStatus.failed,
                metadata: output.metadata,
              ),
            ),
          ];
        }
        // Some servers only send progress updates and never announce the
        // call; surface the card on first sight so it is not invisible.
        final events = <rt.AgentEvent>[];
        if (_startedCalls.add(update.toolCallId)) {
          events.addAll([
            rt.ToolStarted(
              sessionId: sessionId,
              turnId: turnId,
              sequence: nextSequence(),
              occurredAt: _now(),
              call: rt.ToolCallItem(
                id: rt.TimelineItemId(update.toolCallId),
                sessionId: sessionId,
                turnId: turnId,
                sequence: nextSequence(),
                occurredAt: _now(),
                call: rt.ToolCall(
                  id: rt.ToolCallId(update.toolCallId),
                  name: _nameFromKind(update.kind),
                  arguments: update.rawInput is Map
                      ? Map<String, Object?>.from(update.rawInput as Map)
                      : const <String, Object?>{},
                ),
              ),
            ),
          ]);
        }
        if (update.rawOutput != null || update.content != null) {
          events.add(
            rt.ToolOutputUpdated(
              sessionId: sessionId,
              turnId: turnId,
              sequence: nextSequence(),
              occurredAt: _now(),
              callId: rt.ToolCallId(update.toolCallId),
              output: output.snapshot,
            ),
          );
        }
        return events;
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
      case UsageSessionUpdate(:final used):
        // The server reports context occupancy after every model step (not
        // only at the turn boundary), so the client can refresh live usage.
        if (used == null || used <= 0) {
          return const [];
        }
        return [
          rt.UsageUpdated(
            sessionId: sessionId,
            turnId: turnId,
            sequence: nextSequence(),
            occurredAt: _now(),
            usage: rt.TokenUsage(inputTokens: used, totalTokens: used),
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
  final _toolOutputs = <String, _ToolOutput>{};

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
        _rememberToolOutput(
          _toolOutputs,
          ToolCallUpdate(
            toolCallId: toolCall.toolCallId,
            content: toolCall.content,
            rawOutput: toolCall.rawOutput,
          ),
        );
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
        final output = _rememberToolOutput(_toolOutputs, update);
        if (update.status != ToolCallStatus.completed &&
            update.status != ToolCallStatus.failed) {
          return const [];
        }
        _toolOutputs.remove(update.toolCallId);
        return [
          rt.ToolResultItem(
            id: rt.TimelineItemId('result-${update.toolCallId}'),
            sessionId: sessionId,
            turnId: _turnId,
            sequence: _sequence++,
            occurredAt: _now(),
            callId: rt.ToolCallId(update.toolCallId),
            content: output.snapshot.content,
            isError: update.status == ToolCallStatus.failed,
            metadata: output.metadata,
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

/// Maps ACP replay updates to presentation-only conversation items.
final class ClientConversationMapper {
  /// Creates a mapper for [sessionId].
  ClientConversationMapper(this.sessionId);

  /// Session identifier used to filter updates.
  final rt.SessionId sessionId;
  final _toolOutputs = <String, _ToolOutput>{};

  /// Converts one update without fabricating durable runtime identities.
  List<rt.ConversationItem> map(SessionUpdate update) {
    if (update is ToolCallStatusUpdate) {
      _rememberToolOutput(_toolOutputs, update.update);
    } else if (update is ToolCallUpdateSession) {
      _rememberToolOutput(
        _toolOutputs,
        ToolCallUpdate(
          toolCallId: update.toolCall.toolCallId,
          content: update.toolCall.content,
          rawOutput: update.toolCall.rawOutput,
        ),
      );
    }
    return switch (update) {
      UserMessageChunk(:final chunk) => [
        rt.ConversationUserMessage([rt.TextContent(_chunkText(chunk))]),
      ],
      AgentMessageChunk(:final chunk) => [
        rt.ConversationAssistantMessage([rt.TextContent(_chunkText(chunk))]),
      ],
      AgentThoughtChunk(:final chunk) => [
        rt.ConversationAssistantMessage(const [], reasoning: _chunkText(chunk)),
      ],
      ToolCallUpdateSession(:final toolCall) => [
        rt.ConversationToolCall(
          callId: toolCall.toolCallId,
          name: _nameFromKind(toolCall.kind),
          arguments: toolCall.rawInput is Map
              ? Map<String, Object?>.from(toolCall.rawInput as Map)
              : const {},
        ),
      ],
      ToolCallStatusUpdate(:final update)
          when update.status == ToolCallStatus.completed ||
              update.status == ToolCallStatus.failed =>
        [
          rt.ConversationToolResult(
            callId: update.toolCallId,
            content: _toolOutputs.remove(update.toolCallId)!.snapshot.content,
            isError: update.status == ToolCallStatus.failed,
          ),
        ],
      _ => const [],
    };
  }

  static String _chunkText(ContentChunk chunk) =>
      chunk.content is TextContentBlock
      ? (chunk.content as TextContentBlock).text
      : '';

  static String _nameFromKind(ToolKind? kind) => switch (kind) {
    ToolKind.read => 'read',
    ToolKind.edit => 'edit',
    ToolKind.execute => 'shell',
    ToolKind.think => 'plan',
    _ => 'tool',
  };
}

typedef _ToolOutput = ({
  rt.ToolOutputSnapshot snapshot,
  rt.JsonObject metadata,
});

// ACP updates replace only supplied fields. Omitted content must survive both
// live status notifications and session replay; an explicit [] clears it.
_ToolOutput _rememberToolOutput(
  Map<String, _ToolOutput> outputs,
  ToolCallUpdate update,
) {
  final previous = outputs[update.toolCallId];
  final raw = update.rawOutput;
  return outputs[update.toolCallId] = (
    snapshot: rt.ToolOutputSnapshot(
      content: _updateText(update) ?? previous?.snapshot.content ?? '',
      totalBytes: raw is Map && raw['total_bytes'] is int
          ? raw['total_bytes'] as int
          : previous?.snapshot.totalBytes ?? 0,
      truncated: raw is Map && raw['truncated'] is bool
          ? raw['truncated'] as bool
          : previous?.snapshot.truncated ?? false,
    ),
    metadata: raw == null
        ? previous?.metadata ?? const {}
        : _updateMetadata(update),
  );
}

String? _updateText(ToolCallUpdate update) {
  final raw = update.rawOutput;
  if (raw is Map && raw['output'] is String) return raw['output'] as String;
  if (update.content == null) return null;
  return update.content!
      .whereType<ToolCallContentBlock>()
      .map((block) => block.content)
      .whereType<TextContentBlock>()
      .map((block) => block.text)
      .join('\n');
}

rt.JsonObject _updateMetadata(ToolCallUpdate update) {
  final raw = update.rawOutput;
  if (raw is Map && raw['metadata'] is Map) {
    return Map<String, Object?>.from(raw['metadata'] as Map);
  }
  return const {};
}
