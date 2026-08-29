import 'dart:convert';

import '../domain/content.dart';
import '../domain/events.dart';
import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/timeline.dart';
import '../domain/token_estimate.dart';
import '../domain/usage.dart';
import '../ports/cancellation.dart';
import '../ports/model_provider.dart';
import '../ports/session_store.dart';

import 'error_summary.dart';
import 'xml_escape.dart';

/// The largest output budget for a generated compaction summary.
const maxSummaryTokens = 4096;

/// The instruction prefix used for compaction summary generation.
const _compactionInstruction =
    'You are summarizing the early portion of a session transcript so the '
    'conversation can continue within a compact context.\n\n'
    'Preserve, in dense factual plain text:\n'
    '- The user\'s goals and constraints.\n'
    '- Key decisions and their rationale.\n'
    '- The current task state and what remains undone.\n'
    '- Files, commands, and code touched, with their purposes.\n'
    '- Any unresolved issues or open questions.\n\n'
    'Do not summarize the recent messages that are kept in context verbatim.';

/// The inputs for one compaction attempt over a session timeline.
final class CompactionJob {
  /// Creates a compaction job.
  const CompactionJob({
    required this.session,
    required this.timeline,
    required this.systemPrompt,
    required this.model,
    required this.turnId,
    required this.latestUsage,
    required this.nextSequence,
    this.enforceThreshold = true,
    this.instruction,
    this.cancellation,
  });

  /// The session being compacted; its checkpoint chains prior summaries.
  final Session session;

  /// The full known timeline, including items before prior boundaries.
  final List<TimelineItem> timeline;

  /// The system prompt as sent to the model; used for token estimation.
  final String systemPrompt;

  /// The model that generates the summary.
  final ModelRef model;

  /// The turn the emitted events are attached to.
  final TurnId turnId;

  /// The most recent token usage; drives the threshold comparison.
  final TokenUsage latestUsage;

  /// Allocates event sequences continuing the caller's ordering.
  final int Function() nextSequence;

  /// When true, compaction runs only above the configured token threshold.
  final bool enforceThreshold;

  /// Optional user-provided direction included in the summary request.
  final String? instruction;

  /// Cooperative cancellation for the summary model requests.
  final CancellationToken? cancellation;
}

/// Plans and executes context compaction after terminal turns.
///
/// Compaction summarizes the timeline prefix before a boundary item, keeps
/// the newest turns verbatim, and persists the resulting checkpoint through
/// the session store; the store validates that the boundary is the final
/// item of a terminal turn.
final class ContextCompactor {
  /// Creates a compactor with injected provider and storage ports.
  ContextCompactor({
    required this.provider,
    required this.store,
    required this.threshold,
    required this.keptRecentTurns,
    DateTime Function()? now,
  }) : _now = now ?? DateTime.now;

  /// The model provider adapter used for summary generation.
  final ModelProvider provider;

  /// The session persistence adapter receiving checkpoints.
  final SessionStore store;

  /// Context window fraction that triggers compaction after a turn.
  final double threshold;

  /// The number of most recent turns kept verbatim during compaction.
  final int keptRecentTurns;

  final DateTime Function() _now;

  /// Compacts [job] when required, emitting the compaction event triple.
  Stream<AgentEvent> compact(CompactionJob job) async* {
    // Cancellation stops all trailing background work, including compaction.
    if (job.cancellation?.isCancelled == true) {
      return;
    }
    if (!job.enforceThreshold &&
        job.timeline.map((item) => item.turnId).toSet().length <= 1) {
      return;
    }
    final ModelDescriptor descriptor;
    try {
      descriptor = await provider.describe(job.model);
    } catch (_) {
      return;
    }
    if (descriptor.contextWindow <= 0) {
      return;
    }
    final tokens = job.latestUsage.inputTokens > 0
        ? job.latestUsage.inputTokens
        : estimateTokenCount(
            '${job.systemPrompt}\n${_renderTimeline(job.timeline)}',
          );
    if (job.enforceThreshold && tokens < descriptor.contextWindow * threshold) {
      return;
    }
    final timeline = job.timeline;
    final turnCount = timeline.map((item) => item.turnId).toSet().length;
    final kept = turnCount <= keptRecentTurns + 1
        ? _keptWindow(timeline, keptRecentTurns)
        : _keptWindowWithinBudget(timeline, descriptor.contextWindow ~/ 3);
    // A long single turn can fill the context before a second turn exists.
    // Keep the newest item as the minimum live context and compact the prefix.
    final boundaryIndex = timeline.length - kept.length - 1 < 0
        ? (timeline.length > 1 ? 0 : -1)
        : timeline.length - kept.length - 1;
    if (boundaryIndex < 0) return;
    final boundary = timeline[boundaryIndex];
    yield CompactionStarted(
      sessionId: job.session.id,
      turnId: job.turnId,
      sequence: job.nextSequence(),
      occurredAt: _now().toUtc(),
    );
    try {
      final compacted = timeline.sublist(0, boundaryIndex + 1);
      final summary = await _generateSummary(
        session: job.session,
        compacted: compacted,
        model: job.model,
        turnId: job.turnId,
        instruction: job.instruction,
        cancellation: job.cancellation,
        outputTokenLimit: descriptor.maxOutputTokens,
        contextWindow: descriptor.contextWindow,
      );
      final checkpoint = CompactionCheckpoint(
        sessionId: job.session.id,
        compactedThroughSequence: boundary.sequence,
        summary: summary,
        keptRecentMessages: kept.length,
        inputTokensBefore: tokens,
        inputTokensAfter:
            estimateTokenCount(summary) +
            estimateTokenCount(_renderTimeline(kept)),
        createdAt: _now().toUtc(),
      );
      await store.saveCompaction(job.session.id, checkpoint);
      yield CompactionFinished(
        sessionId: job.session.id,
        turnId: job.turnId,
        sequence: job.nextSequence(),
        occurredAt: _now().toUtc(),
        checkpoint: checkpoint,
      );
    } on TurnCancelledException {
      rethrow;
    } catch (error) {
      yield CompactionFailed(
        sessionId: job.session.id,
        turnId: job.turnId,
        sequence: job.nextSequence(),
        occurredAt: _now().toUtc(),
        message: safeErrorMessage(
          'Context compaction failed; shorten the compaction instruction or '
          'select a model with a larger context window',
          error,
        ),
      );
    }
  }

  /// Generates a compaction summary over the compacted items with one model
  /// call, chaining the previous checkpoint summary when present.
  Future<String> _generateSummary({
    required Session session,
    required List<TimelineItem> compacted,
    required ModelRef model,
    required TurnId turnId,
    String? instruction,
    CancellationToken? cancellation,
    int outputTokenLimit = 0,
    required int contextWindow,
  }) async {
    final prefix = StringBuffer(_compactionInstruction);
    final instructionText = instruction?.trim();
    if (instructionText != null && instructionText.isNotEmpty) {
      prefix.write('\n\nAdditional user instruction:\n$instructionText');
    }
    final previous = session.compaction?.summary.trim();
    if (previous != null && previous.isNotEmpty) {
      prefix.write('\n\n<previous_summary>\n$previous\n</previous_summary>');
    }
    final inputBudget = (contextWindow * 0.6).floor().clamp(1, contextWindow);
    final transcript = _renderTimeline(compacted);
    if (estimateTokenCount('$prefix\n$transcript') <= inputBudget) {
      return _requestSummary(
        prompt: '$prefix\n\n<transcript>\n$transcript\n</transcript>',
        session: session,
        model: model,
        turnId: turnId,
        cancellation: cancellation,
        outputTokenLimit: outputTokenLimit,
      );
    }
    final chunks = _summaryChunks(compacted, inputBudget ~/ 2);
    final summaries = <String>[];
    final chunkCount = chunks.length;
    for (var index = 0; index < chunks.length; index++) {
      cancellation?.throwIfCancelled();
      summaries.add(
        await _requestSummary(
          prompt:
              '$_compactionInstruction\n\nSummarize history chunk '
              '${index + 1} of $chunkCount.\n\n<transcript>\n'
              '${_renderTimeline(chunks[index])}\n</transcript>',
          session: session,
          model: model,
          turnId: turnId,
          cancellation: cancellation,
          outputTokenLimit: outputTokenLimit,
        ),
      );
    }
    final boundedSummaries = summaries
        .map(
          (summary) => summary.length > 12000
              ? '${summary.substring(0, 12000)}\n...[summary truncated]'
              : summary,
        )
        .join('\n\n');
    return _requestSummary(
      prompt:
          '$prefix\n\n<chunk_summaries>\n$boundedSummaries'
          '\n</chunk_summaries>',
      session: session,
      model: model,
      turnId: turnId,
      cancellation: cancellation,
      outputTokenLimit: outputTokenLimit,
    );
  }

  /// Groups complete timeline items into bounded summary requests.
  static List<List<TimelineItem>> _summaryChunks(
    List<TimelineItem> items,
    int tokenBudget,
  ) {
    final chunks = <List<TimelineItem>>[];
    var current = <TimelineItem>[];
    var tokens = 0;
    for (final item in items) {
      final itemTokens = estimateTokenCount(_renderTimeline([item]));
      if (current.isNotEmpty && tokens + itemTokens > tokenBudget) {
        chunks.add(current);
        current = <TimelineItem>[];
        tokens = 0;
      }
      current.add(item);
      tokens += itemTokens;
    }
    if (current.isNotEmpty) chunks.add(current);
    return chunks;
  }

  /// Executes one bounded summary request and returns its non-empty text.
  Future<String> _requestSummary({
    required String prompt,
    required Session session,
    required ModelRef model,
    required TurnId turnId,
    required CancellationToken? cancellation,
    required int outputTokenLimit,
  }) async {
    final request = ModelRequest(
      sessionId: session.id,
      turnId: turnId,
      model: model,
      messages: [
        ModelMessage(
          role: ModelMessageRole.user,
          content: [TextContent(prompt)],
        ),
      ],
      maxOutputTokens: outputTokenLimit > 0
          ? outputTokenLimit.clamp(1, 8192)
          : maxSummaryTokens,
      cancellation: cancellation,
    );
    ModelResponse? completed;
    await for (final event in provider.stream(request)) {
      cancellation?.throwIfCancelled();
      switch (event) {
        case TextDeltaEvent() || ReasoningDeltaEvent():
          break;
        case ModelCompletedEvent(:final response):
          if (completed != null) {
            throw StateError('model stream completed more than once');
          }
          completed = response;
        case ModelFailedEvent(:final error, :final stackTrace):
          Error.throwWithStackTrace(error, stackTrace);
      }
    }
    if (completed == null) {
      throw StateError('model stream ended without response');
    }
    final summary = textFromContent(completed.content).trim();
    if (summary.isEmpty) {
      throw StateError('compaction summary is empty');
    }
    return summary;
  }

  /// Renders timeline items as a transcript for summary generation.
  static String _renderTimeline(List<TimelineItem> items) {
    final buffer = StringBuffer();
    for (final item in items) {
      switch (item) {
        case UserMessageItem(:final content):
          buffer.writeln(
            '<user>\n${escapeXml(textFromContent(content))}\n</user>',
          );
        case AssistantMessageItem(:final content):
          buffer.writeln(
            '<assistant>\n${escapeXml(textFromContent(content))}\n</assistant>',
          );
        case ToolCallItem(:final call):
          buffer
            ..write('<tool_call name="${escapeXml(call.name)}" arguments="')
            ..write(escapeXml(jsonEncode(call.arguments)))
            ..writeln('"/>');
        case ToolResultItem(:final content, :final isError):
          final rendered = _truncateToolResult(content);
          buffer
            ..writeln('<tool_result error="$isError">')
            ..writeln(escapeXml(rendered))
            ..writeln('</tool_result>');
      }
    }
    return buffer.toString().trimRight();
  }

  /// Bounds tool output included in model-facing transcripts to avoid one
  /// command consuming the complete compaction request budget.
  static String _truncateToolResult(String content) {
    const limit = 4096;
    if (content.length <= limit) return content;
    final head = limit ~/ 2;
    final tail = limit - head;
    return '${content.substring(0, head)}\n...[tool result truncated; original length ${content.length}]...\n${content.substring(content.length - tail)}';
  }

  /// Returns the newest [keptRecentTurns] whole turns from [timeline].
  static List<TimelineItem> _keptWindow(
    List<TimelineItem> timeline,
    int keptRecentTurns,
  ) {
    if (timeline.isEmpty || keptRecentTurns <= 0) {
      return const [];
    }
    final keptTurns = <TurnId>{};
    for (
      var i = timeline.length - 1;
      i >= 0 && keptTurns.length < keptRecentTurns;
      i--
    ) {
      keptTurns.add(timeline[i].turnId);
    }
    return timeline.where((item) => keptTurns.contains(item.turnId)).toList();
  }

  /// Keeps complete newest turns within a token budget, always retaining one.
  static List<TimelineItem> _keptWindowWithinBudget(
    List<TimelineItem> timeline,
    int budget,
  ) {
    final turns = <TurnId>[];
    for (var i = timeline.length - 1; i >= 0; i--) {
      if (!turns.contains(timeline[i].turnId)) turns.add(timeline[i].turnId);
    }
    final kept = <TimelineItem>[];
    for (final turn in turns) {
      final candidate = timeline.where((item) => item.turnId == turn).toList();
      if (kept.isNotEmpty &&
          budget > 0 &&
          estimateTokenCount(_renderTimeline([...candidate, ...kept])) >
              budget) {
        break;
      }
      kept.insertAll(0, candidate);
    }
    if (kept.length == timeline.length && timeline.length > 1) {
      return [timeline.last];
    }
    return kept;
  }
}
