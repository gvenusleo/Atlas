import 'dart:async';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:nocterm/nocterm.dart';

import 'chat_message.dart';

/// Maximum characters of a tool result rendered in the transcript.
const maxToolResultChars = 200;

/// Maximum characters kept for a reasoning message; only the tail is shown
/// on a single line, so anything older than this window is dropped.
const maxReasoningChars = 1024;

/// Bridges runtime turn events into rendered chat messages.
///
/// Owns the conversation state for one TUI session: it submits user text as
/// turns through the injected [AgentRuntime], accumulates model deltas and
/// tool activity into [ChatMessage]s, and notifies listeners on every change.
/// It never calls providers, tools, or storage directly.
final class ChatController implements Listenable {
  /// Creates a controller bound to [runtime].
  ChatController({required this.runtime, String? workingDirectory})
    : _workingDirectory = workingDirectory ?? Directory.current.path;

  /// The runtime that executes turns.
  final AgentRuntime runtime;

  final String _workingDirectory;
  final List<ChatMessage> _messages = [];
  final List<void Function()> _listeners = [];
  SessionId? _sessionId;
  ModelRef? _model;
  String? _reasoningEffort;
  bool _busy = false;

  /// Whether the last appended delta is still open for accumulation.
  bool _sealed = true;

  /// The rendered transcript in occurrence order.
  List<ChatMessage> get messages => List.unmodifiable(_messages);

  /// Whether a turn is currently running.
  bool get busy => _busy;

  /// The model override for subsequent turns, or `null` for the default.
  ModelRef? get model => _model;

  /// The reasoning effort for subsequent turns, or `null` for the default.
  String? get reasoningEffort => _reasoningEffort;

  @override
  void addListener(void Function() listener) => _listeners.add(listener);

  @override
  void removeListener(void Function() listener) => _listeners.remove(listener);

  /// Submits [text] as a new user turn.
  ///
  /// The turn is ignored while one is already running and when [text] is
  /// blank. The session id is reused across turns so the conversation stays
  /// in one persisted session.
  Future<void> send(String text) async {
    final trimmed = text.trim();
    if (trimmed.isEmpty || _busy) {
      return;
    }
    _busy = true;
    _messages.add(ChatMessage(kind: ChatMessageKind.user, text: trimmed));
    _notify();
    try {
      await for (final event in runtime.run(
        TurnRequest(
          content: [TextContent(trimmed)],
          sessionId: _sessionId,
          workingDirectory: _workingDirectory,
          model: _model,
          reasoningEffort: _reasoningEffort,
        ),
      )) {
        _handle(event);
      }
    } catch (error) {
      _messages.add(ChatMessage(kind: ChatMessageKind.error, text: '$error'));
    } finally {
      _busy = false;
      _notify();
    }
  }

  /// Starts a new session: clears the transcript and forgets the session id.
  void reset() {
    _messages.clear();
    _sessionId = null;
    _sealed = true;
    _notify();
  }

  /// Switches the model used by subsequent turns and confirms with a notice.
  ///
  /// [effortName] only labels the notice; the provider-local [effort] value is
  /// what gets sent with each turn.
  void setModel(
    ModelRef model, {
    String? displayName,
    String? effort,
    String? effortName,
  }) {
    _model = model;
    _reasoningEffort = effort;
    final label = displayName ?? model.modelId.value;
    final suffix = effortName == null ? '' : ', effort $effortName';
    addNotice('Switched to $label (${model.providerId.value})$suffix');
  }

  /// Appends a local notice, such as slash command help, to the transcript.
  void addNotice(String text) {
    _messages.add(ChatMessage(kind: ChatMessageKind.system, text: text));
    _notify();
  }

  void _handle(AgentEvent event) {
    switch (event) {
      case TurnStarted(:final sessionId):
        _sessionId = sessionId;
      case ModelTextDelta(:final delta):
        _appendDelta(ChatMessageKind.assistant, delta);
      case ModelReasoningDelta(:final delta):
        _appendDelta(ChatMessageKind.reasoning, delta);
      case ToolStarted(:final call):
        _sealed = true;
        _messages.add(
          ChatMessage(
            kind: ChatMessageKind.tool,
            toolName: call.call.name,
            text: 'running…',
          ),
        );
      case ToolFinished(:final result):
        _updateLastTool(result);
      case TurnFinished():
        _sealed = true;
      default:
        break;
    }
    _notify();
  }

  void _appendDelta(ChatMessageKind kind, String delta) {
    final last = _messages.isEmpty ? null : _messages.last;
    if (!_sealed && last != null && last.kind == kind) {
      final text = kind == ChatMessageKind.reasoning
          ? _tailBudget('${last.text}$delta')
          : '${last.text}$delta';
      _messages[_messages.length - 1] = ChatMessage(
        kind: kind,
        text: text,
        toolName: last.toolName,
        isError: last.isError,
      );
    } else {
      final text = kind == ChatMessageKind.reasoning
          ? _tailBudget(delta)
          : delta;
      _messages.add(ChatMessage(kind: kind, text: text));
    }
    _sealed = false;
  }

  /// Keeps only the tail of [text], bounded by [maxReasoningChars].
  static String _tailBudget(String text) => text.length <= maxReasoningChars
      ? text
      : text.substring(text.length - maxReasoningChars);

  void _updateLastTool(ToolResultItem result) {
    final index = _messages.lastIndexWhere(
      (message) => message.kind == ChatMessageKind.tool,
    );
    if (index < 0) {
      return;
    }
    final summary = result.content.length > maxToolResultChars
        ? '${result.content.substring(0, maxToolResultChars)}…'
        : result.content;
    _messages[index] = ChatMessage(
      kind: ChatMessageKind.tool,
      toolName: _messages[index].toolName,
      text: result.isError ? 'failed: $summary' : summary,
      isError: result.isError,
    );
  }

  void _notify() {
    for (final listener in List.of(_listeners)) {
      listener();
    }
  }
}
