import 'package:atlas_runtime/atlas_runtime.dart';

import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';

/// Transcript projections shared by live turns and stored timelines.
///
/// A function returns the same list instance when it changes nothing, so
/// callers writing Riverpod state can reuse the previous value and skip a
/// rebuild.

/// Appends [message] to [messages].
List<WorkspaceMessage> appendMessage(
  List<WorkspaceMessage> messages,
  WorkspaceMessage message,
) => [...messages, message];

/// Appends a streaming [delta] of [kind].
///
/// Coalesces into the trailing message when [streaming] reports that the
/// previous delta opened a message of the same kind; otherwise it starts a new
/// one. Reasoning messages carry the elapsed-time start and run until the turn
/// moves on.
List<WorkspaceMessage> appendStreamDelta(
  List<WorkspaceMessage> messages, {
  required WorkspaceMessageKind kind,
  required String delta,
  required bool streaming,
  required String Function() nextId,
}) {
  final last = messages.lastOrNull;
  if (streaming && last?.kind == kind) {
    return [
      ...messages.sublist(0, messages.length - 1),
      last!.copyWith(text: last.text + delta),
    ];
  }
  return [
    ...messages,
    WorkspaceMessage(
      id: nextId(),
      kind: kind,
      text: delta,
      startedAt: kind == WorkspaceMessageKind.reasoning ? DateTime.now() : null,
      isRunning: kind == WorkspaceMessageKind.reasoning,
    ),
  ];
}

/// Marks the trailing streaming reasoning summary complete.
List<WorkspaceMessage> finishRunningReasoning(List<WorkspaceMessage> messages) {
  final index = messages.lastIndexWhere(
    (message) =>
        message.kind == WorkspaceMessageKind.reasoning && message.isRunning,
  );
  if (index < 0) {
    return messages;
  }
  final updated = [...messages];
  updated[index] = updated[index].copyWith(isRunning: false);
  return updated;
}

/// Appends [call] as a tool invocation that has not returned yet.
List<WorkspaceMessage> startToolCall(
  List<WorkspaceMessage> messages,
  ToolCall call, {
  required DateTime startedAt,
}) => [
  ...messages,
  WorkspaceMessage(
    id: call.id.value,
    kind: WorkspaceMessageKind.tool,
    text: '',
    toolName: call.name,
    arguments: call.arguments,
    startedAt: startedAt,
    isRunning: true,
  ),
];

/// Replaces the running tool message of [callId] with [output].
List<WorkspaceMessage> updateToolOutput(
  List<WorkspaceMessage> messages,
  ToolCallId callId,
  String output,
) {
  final index = _runningToolIndex(messages, callId, last: true);
  if (index < 0) {
    return messages;
  }
  final updated = [...messages];
  updated[index] = updated[index].copyWith(text: output);
  return updated;
}

/// Completes the running tool message of [callId] with its result.
List<WorkspaceMessage> finishToolCall(
  List<WorkspaceMessage> messages,
  ToolCallId callId, {
  required String content,
  required bool isError,
}) {
  final index = _runningToolIndex(messages, callId, last: false);
  if (index < 0) {
    return messages;
  }
  final updated = [...messages];
  updated[index] = updated[index].copyWith(
    text: content,
    isError: isError,
    isRunning: false,
  );
  return updated;
}

/// Replaces the trailing plan message with [text], or appends one.
List<WorkspaceMessage> upsertPlanMessage(
  List<WorkspaceMessage> messages, {
  required String id,
  required String text,
}) {
  final index = messages.lastIndexWhere(
    (message) => message.kind == WorkspaceMessageKind.plan,
  );
  if (index < 0) {
    return appendMessage(
      messages,
      WorkspaceMessage(id: id, kind: WorkspaceMessageKind.plan, text: text),
    );
  }
  final updated = [...messages];
  updated[index] = updated[index].copyWith(text: text);
  return updated;
}

/// Whether any timeline message carries image content.
bool timelineHasImages(List<TimelineItem> timeline) {
  for (final item in timeline) {
    final content = switch (item) {
      UserMessageItem(:final content) => content,
      AssistantMessageItem(:final content) => content,
      _ => null,
    };
    if (content != null && content.any((part) => part is ImageContent)) {
      return true;
    }
  }
  return false;
}

/// Converts a stored timeline into presentation messages in occurrence order.
List<WorkspaceMessage> messagesFromTimeline(
  List<TimelineItem> timeline, {
  required String Function() nextId,
}) {
  var messages = const <WorkspaceMessage>[];
  for (final item in timeline) {
    switch (item) {
      case UserMessageItem(:final content):
        final text = textFromContent(content);
        final imageSources = [
          for (final part in content)
            if (part is ImageContent) part.source,
        ];
        if (text.isNotEmpty || imageSources.isNotEmpty) {
          messages = appendMessage(
            messages,
            WorkspaceMessage(
              id: item.id.value,
              kind: WorkspaceMessageKind.user,
              text: text,
              imageSources: imageSources,
            ),
          );
        }
      case AssistantMessageItem(:final content, :final reasoning):
        if (reasoning.isNotEmpty) {
          messages = appendMessage(
            messages,
            WorkspaceMessage(
              id: nextId(),
              kind: WorkspaceMessageKind.reasoning,
              text: reasoning,
            ),
          );
        }
        final text = textFromContent(content);
        if (text.isNotEmpty) {
          messages = appendMessage(
            messages,
            WorkspaceMessage(
              id: item.id.value,
              kind: WorkspaceMessageKind.assistant,
              text: text,
            ),
          );
        }
      case ToolCallItem(:final call):
        messages = startToolCall(messages, call, startedAt: item.occurredAt);
      case ToolResultItem(:final callId, :final content, :final isError):
        messages = finishToolCall(
          messages,
          callId,
          content: content,
          isError: isError,
        );
    }
  }
  return messages;
}

/// Index of the running tool message of [callId], or -1.
///
/// Live output updates the most recent matching call; a result completes the
/// first one, matching the order the runtime reported.
int _runningToolIndex(
  List<WorkspaceMessage> messages,
  ToolCallId callId, {
  required bool last,
}) {
  bool matches(WorkspaceMessage message) =>
      message.kind == WorkspaceMessageKind.tool &&
      message.isRunning &&
      message.id == callId.value;
  return last ? messages.lastIndexWhere(matches) : messages.indexWhere(matches);
}
