import 'dart:convert';
import 'dart:math' as math;

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
    'You are a context summarization assistant. Do not continue the '
    'conversation; output only a concise structured checkpoint.\n\n'
    'Use exactly these sections:\n'
    '## Goal\n## Constraints & Preferences\n## Progress\n'
    '### Done\n### In Progress\n### Blocked\n## Key Decisions\n'
    '## Next Steps\n## Critical Context\n\n'
    'Preserve exact file paths, symbols, commands, and error messages. '
    'Summarize only the supplied history; recent messages are retained verbatim.';

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

/// Plans and executes context compaction before or after model turns.
///
/// Compaction summarizes the timeline prefix before a message boundary, keeps
/// the newest token window verbatim, and persists the resulting checkpoint
/// through the session store.
final class ContextCompactor {
  /// Creates a compactor with injected provider and storage ports.
  ContextCompactor({
    required this.provider,
    required this.store,
    required this.threshold,
    this.keepRecentTokens,
    this.keptRecentTurns = 5,
    this.reserveTokens = 16384,
    DateTime Function()? now,
  }) : _now = now ?? DateTime.now;

  /// The model provider adapter used for summary generation.
  final ModelProvider provider;

  /// The session persistence adapter receiving checkpoints.
  final SessionStore store;

  /// Context window fraction that triggers compaction after a turn.
  final double threshold;

  /// The approximate number of newest tokens kept verbatim.
  final int? keepRecentTokens;

  /// The legacy turn count used only as a fallback for small sessions.
  final int keptRecentTurns;

  /// Tokens reserved for the next model response.
  final int reserveTokens;

  final DateTime Function() _now;

  /// Whether this compactor uses token-based message boundaries.
  bool get usesTokenWindow => keepRecentTokens != null;

  /// Compacts [job] when required, emitting the compaction event triple.
  Stream<AgentEvent> compact(CompactionJob job) async* {
    // Cancellation stops all trailing background work, including compaction.
    if (job.cancellation?.isCancelled == true) {
      return;
    }
    if (job.timeline.length < 2 ||
        (!job.enforceThreshold &&
            keepRecentTokens == null &&
            job.timeline.map((item) => item.turnId).toSet().length <= 1)) {
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
    final thresholdTokens =
        reserveTokens > 0 && reserveTokens < descriptor.contextWindow
        ? descriptor.contextWindow - reserveTokens
        : (descriptor.contextWindow * threshold).floor();
    if (job.enforceThreshold && tokens < thresholdTokens) {
      return;
    }
    final timeline = job.timeline;
    final int firstKeptIndex;
    final List<TimelineItem> kept;
    if (keepRecentTokens == null) {
      final turnCount = timeline.map((item) => item.turnId).toSet().length;
      kept = turnCount <= keptRecentTurns + 1
          ? _keptWindow(timeline, keptRecentTurns)
          : _keptWindowWithinBudget(
              timeline,
              math.max(1, descriptor.contextWindow ~/ 3),
            );
      firstKeptIndex = math.max(1, timeline.length - kept.length);
    } else {
      final keepBudget =
          keepRecentTokens!.clamp(1, math.max(1, descriptor.contextWindow ~/ 3))
              as int;
      firstKeptIndex = _firstKeptIndex(timeline, keepBudget);
      kept = timeline.sublist(firstKeptIndex);
    }
    if (firstKeptIndex <= 0 || firstKeptIndex >= timeline.length) return;
    final boundary = timeline[firstKeptIndex - 1];
    yield CompactionStarted(
      sessionId: job.session.id,
      turnId: job.turnId,
      sequence: job.nextSequence(),
      occurredAt: _now().toUtc(),
    );
    try {
      final compacted = timeline.sublist(0, firstKeptIndex);
      final turnPrefixStart = _turnStartIndex(timeline, firstKeptIndex);
      final history = compacted.sublist(0, turnPrefixStart);
      final turnPrefix = compacted.sublist(turnPrefixStart);
      final summaries = <String>[];
      if (history.isNotEmpty) {
        summaries.add(
          await _generateSummary(
            session: job.session,
            compacted: history,
            model: job.model,
            turnId: job.turnId,
            instruction: job.instruction,
            cancellation: job.cancellation,
            outputTokenLimit: descriptor.maxOutputTokens,
            contextWindow: descriptor.contextWindow,
          ),
        );
      } else if (job.session.compaction?.summary.trim().isNotEmpty == true) {
        summaries.add(job.session.compaction!.summary.trim());
      }
      if (turnPrefix.isNotEmpty) {
        summaries.add(
          await _generateTurnPrefixSummary(
            turnPrefix: turnPrefix,
            model: job.model,
            session: job.session,
            turnId: job.turnId,
            cancellation: job.cancellation,
            outputTokenLimit: descriptor.maxOutputTokens,
            instruction: job.instruction,
          ),
        );
      }
      final summary = summaries.join('\n\n---\n\n');
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

  /// Summarizes the prefix of a turn whose recent suffix remains verbatim.
  Future<String> _generateTurnPrefixSummary({
    required List<TimelineItem> turnPrefix,
    required ModelRef model,
    required Session session,
    required TurnId turnId,
    required CancellationToken? cancellation,
    required int outputTokenLimit,
    String? instruction,
  }) => _requestSummary(
    prompt:
        'The following is the early prefix of a turn. The newer suffix is '
        'retained verbatim. Summarize only the information needed to '
        'understand that suffix. Use concise bullets under these headings:\n'
        '## Original Request\n## Early Progress\n## Context for Suffix\n\n'
        '${instruction?.trim().isEmpty ?? true ? '' : 'Additional user instruction:\n${instruction!.trim()}\n\n'}'
        '<transcript>\n${_renderTimeline(turnPrefix)}\n</transcript>',
    session: session,
    model: model,
    turnId: turnId,
    cancellation: cancellation,
    outputTokenLimit: outputTokenLimit,
  );

  /// Finds the first item of the turn containing [firstKeptIndex].
  static int _turnStartIndex(List<TimelineItem> timeline, int firstKeptIndex) {
    if (firstKeptIndex <= 0) return 0;
    final turn = timeline[firstKeptIndex].turnId;
    var index = firstKeptIndex - 1;
    while (index >= 0 && timeline[index].turnId == turn) {
      index--;
    }
    return index + 1;
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

  /// Finds a message boundary that keeps approximately [budget] newest tokens.
  ///
  /// A boundary may precede a user or assistant item, but never a tool call
  /// result. The newest user message is always retained.
  static int _firstKeptIndex(List<TimelineItem> timeline, int budget) {
    final valid = <int>[];
    for (var i = 0; i < timeline.length; i++) {
      if (timeline[i] is UserMessageItem ||
          timeline[i] is AssistantMessageItem) {
        valid.add(i);
      }
    }
    if (valid.isEmpty) return timeline.length;
    var tokens = 0;
    var first = valid.first;
    for (var i = timeline.length - 1; i >= 0; i--) {
      tokens += estimateTokenCount(_renderTimeline([timeline[i]]));
      if (tokens >= budget) {
        first = valid.firstWhere(
          (index) => index >= i,
          orElse: () => valid.last,
        );
        break;
      }
    }
    if (first == valid.first && valid.length > 1) {
      final newestTurn = timeline.last.turnId;
      first = valid.firstWhere(
        (index) => timeline[index].turnId == newestTurn,
        orElse: () => valid.last,
      );
    }
    return first;
  }

  /// Returns the newest [keptRecentTurns] whole turns from [timeline].
  static List<TimelineItem> _keptWindow(
    List<TimelineItem> timeline,
    int keptRecentTurns,
  ) {
    if (timeline.isEmpty || keptRecentTurns <= 0) return const [];
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
