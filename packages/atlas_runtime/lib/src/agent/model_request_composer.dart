import '../domain/content.dart';
import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/timeline.dart';
import '../skills/skill.dart';
import '../skills/skill_catalog.dart';

import 'xml_escape.dart';

/// The maximum total size of skill instructions injected into one turn.
const maxSelectedSkillBytes = 64 * 1024;

/// Assembles model input messages from durable session state.
///
/// Pure mapping only: timeline projection, skill instruction injection, and
/// input-capability filtering; no provider, tool, or storage access.
abstract final class ModelRequestComposer {
  /// Renders explicitly selected skills as non-persistent user context
  /// messages for the current turn, in first-selection order.
  static List<ModelMessage> skillMessages(
    List<String> names,
    SkillCatalog skills,
  ) {
    if (names.isEmpty) {
      return const [];
    }
    final result = <ModelMessage>[];
    final seen = <String>{};
    var total = 0;
    for (final name in names) {
      if (!seen.add(name)) {
        continue;
      }
      final skill = skills.lookup(name);
      if (skill == null) {
        continue;
      }
      total += skill.content.length;
      if (total > maxSelectedSkillBytes) {
        throw StateError('selected skill instructions exceed 64 KiB');
      }
      result.add(
        ModelMessage(
          role: ModelMessageRole.user,
          content: [TextContent(_skillContext(skill))],
        ),
      );
    }
    return result;
  }

  /// Projects durable timeline items onto model messages, merging tool calls
  /// into their assistant messages and dropping calls that never received a
  /// result. Items at or before the compaction boundary are excluded.
  static List<ModelMessage> projectTimeline(
    List<TimelineItem> items,
    List<ModelCheckpoint> checkpoints, {
    CompactionCheckpoint? compaction,
  }) {
    final continuations = {
      for (final checkpoint in checkpoints)
        checkpoint.timelineItemId: checkpoint.continuation,
    };
    final result = <ModelMessage>[];
    final visibleItems = compaction == null || compaction.summary.trim().isEmpty
        ? items
        : items.where(
            (item) => item.sequence > compaction.compactedThroughSequence,
          );
    for (final item in visibleItems) {
      switch (item) {
        case UserMessageItem(:final content):
          result.add(
            ModelMessage(role: ModelMessageRole.user, content: content),
          );
        case AssistantMessageItem(:final id, :final content):
          result.add(
            ModelMessage(
              role: ModelMessageRole.assistant,
              content: content,
              continuation: continuations[id],
            ),
          );
        case ToolCallItem(:final call):
          if (result.isNotEmpty &&
              result.last.role == ModelMessageRole.assistant) {
            final previous = result.removeLast();
            result.add(
              ModelMessage(
                role: ModelMessageRole.assistant,
                content: previous.content,
                toolCalls: [...previous.toolCalls, call],
                continuation: previous.continuation,
              ),
            );
          } else {
            result.add(
              ModelMessage(role: ModelMessageRole.assistant, toolCalls: [call]),
            );
          }
        case ToolResultItem(:final callId, :final content):
          result.add(
            ModelMessage(
              role: ModelMessageRole.tool,
              toolCallId: callId,
              toolOutput: content,
            ),
          );
      }
    }
    return List<ModelMessage>.unmodifiable(_dropOrphanToolCalls(result));
  }

  /// Replaces image content with a placeholder when the active model cannot
  /// accept images, keeping the conversation usable after a model switch.
  ///
  /// Returns the original list when no filtering is needed.
  static List<ModelMessage> applyInputCapabilities(
    List<ModelMessage> messages,
    ModelDescriptor? descriptor,
  ) {
    if (descriptor == null ||
        descriptor.inputCapabilities.contains(ModelInputCapability.image)) {
      return messages;
    }
    final filtered = <ModelMessage>[];
    var changed = false;
    for (final message in messages) {
      if (!message.content.any((part) => part is ImageContent)) {
        filtered.add(message);
        continue;
      }
      changed = true;
      filtered.add(
        ModelMessage(
          role: message.role,
          content: [
            for (final part in message.content)
              if (part is ImageContent)
                const TextContent(_omittedImagePlaceholder)
              else
                part,
          ],
          toolCalls: message.toolCalls,
          toolCallId: message.toolCallId,
          toolOutput: message.toolOutput,
          continuation: message.continuation,
        ),
      );
    }
    return changed ? filtered : messages;
  }

  /// Wraps the full SKILL.md content in XML instruction tags.
  ///
  /// Only the metadata is escaped; the body is injected verbatim so the model
  /// reads the raw markdown, matching the instruction-file injection pattern.
  static String _skillContext(Skill skill) =>
      '<skill>\n<name>${escapeXml(skill.name)}</name>\n'
      '<path>${escapeXml(skill.path)}</path>\n'
      '${skill.content.trimRight()}\n</skill>';

  /// Removes tool calls that never received a tool result.
  ///
  /// A cancelled or interrupted turn can persist a tool call without its
  /// result; strict chat-completions providers reject such a message because
  /// every `tool_calls` entry must be answered by a tool message.
  static List<ModelMessage> _dropOrphanToolCalls(List<ModelMessage> messages) {
    final toolResultIds = <ToolCallId>{
      for (final message in messages)
        if (message.role == ModelMessageRole.tool && message.toolCallId != null)
          message.toolCallId!,
    };
    final cleaned = <ModelMessage>[];
    for (final message in messages) {
      if (message.role != ModelMessageRole.assistant ||
          message.toolCalls.isEmpty) {
        cleaned.add(message);
        continue;
      }
      final paired = [
        for (final call in message.toolCalls)
          if (toolResultIds.contains(call.id)) call,
      ];
      if (paired.length == message.toolCalls.length) {
        cleaned.add(message);
        continue;
      }
      if (paired.isNotEmpty || message.content.isNotEmpty) {
        cleaned.add(
          ModelMessage(
            role: message.role,
            content: message.content,
            toolCalls: paired,
            continuation: message.continuation,
          ),
        );
      }
    }
    return cleaned;
  }

  /// Placeholder text replacing image content when the active model cannot
  /// accept images.
  static const _omittedImagePlaceholder =
      '[image omitted: current model does not support image input]';
}
