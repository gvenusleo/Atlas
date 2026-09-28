import 'package:atlas_runtime/atlas_runtime.dart';

import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';

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
  final messages = <WorkspaceMessage>[];
  final calls = <ToolCallId, int>{};
  for (final item in timeline) {
    switch (item) {
      case UserMessageItem(:final content):
        final text = textFromContent(content);
        final imageSources = [
          for (final part in content)
            if (part is ImageContent) part.source,
        ];
        if (text.isNotEmpty || imageSources.isNotEmpty) {
          messages.add(
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
          messages.add(
            WorkspaceMessage(
              id: nextId(),
              kind: WorkspaceMessageKind.reasoning,
              text: reasoning,
            ),
          );
        }
        final text = textFromContent(content);
        if (text.isNotEmpty) {
          messages.add(
            WorkspaceMessage(
              id: item.id.value,
              kind: WorkspaceMessageKind.assistant,
              text: text,
            ),
          );
        }
      case ToolCallItem(:final call):
        calls[call.id] = messages.length;
        messages.add(
          WorkspaceMessage(
            id: item.id.value,
            kind: WorkspaceMessageKind.tool,
            text: '',
            toolName: call.name,
            arguments: call.arguments,
            isRunning: true,
          ),
        );
      case ToolResultItem(:final callId, :final content, :final isError):
        final index = calls[callId];
        if (index != null) {
          messages[index] = messages[index].copyWith(
            text: content,
            isError: isError,
            isRunning: false,
          );
        }
    }
  }
  return messages;
}
