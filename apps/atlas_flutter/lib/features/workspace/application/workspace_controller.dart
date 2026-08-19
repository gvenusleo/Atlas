import 'dart:async';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/runtime_environment.dart';
import 'workspace_message.dart';
import 'workspace_state.dart';

/// Working directory for the workspace, overridable in tests.
final workspaceWorkingDirectoryProvider =
    NotifierProvider<WorkspaceWorkingDirectory, String>(
      WorkspaceWorkingDirectory.new,
    );

/// Holds the workspace working directory.
class WorkspaceWorkingDirectory extends Notifier<String> {
  @override
  String build() => Directory.current.path;

  /// Switches the working directory for subsequent sessions.
  void set(String directory) => state = directory;
}

/// Coordinates one Flutter workspace with the injected shared runtime.
final class WorkspaceController extends Notifier<WorkspaceState> {
  CancellationToken? _cancellation;
  int _localId = 0;
  bool _streamOpen = false;

  /// Runtime services supplied by the application composition root.
  RuntimeEnvironment get _environment =>
      ref.read(runtimeEnvironmentProvider) ??
      (throw StateError(
        'workspaceProvider requires runtimeEnvironmentProvider override',
      ));

  /// Models available in the current configuration.
  List<ModelDescriptor> get models => _environment.models;

  @override
  WorkspaceState build() {
    final environment = ref.watch(runtimeEnvironmentProvider);
    if (environment == null) {
      throw StateError('workspaceProvider requires a composed runtime');
    }
    final workingDirectory = ref.watch(workspaceWorkingDirectoryProvider);
    final activeModel = _descriptorFor(environment.runtime.defaultModel);
    ref.onDispose(() => _cancellation?.cancel());
    return WorkspaceState(
      messages: const [],
      sessions: const [],
      activeModel: activeModel,
      workingDirectory: workingDirectory,
      reasoningEffort: activeModel.reasoningEfforts.firstOrNull?.value,
    );
  }

  /// Loads every persisted session across all directories, newest first.
  Future<void> refreshSessions() async {
    state = state.copyWith(loadingSessions: true);
    try {
      final sessions = <SessionSummary>[];
      String? cursor;
      do {
        final page = await _environment.runtime.listSessions(
          cursor: cursor,
          limit: 100,
        );
        sessions.addAll(page.items);
        cursor = page.nextCursor;
      } while (cursor != null && sessions.length < 500);
      state = state.copyWith(sessions: sessions, loadingSessions: false);
    } catch (error) {
      _append(WorkspaceMessageKind.error, 'Cannot load sessions: $error');
      state = state.copyWith(loadingSessions: false);
    }
  }

  /// Clears the transcript so the next prompt creates a new session.
  void newSession() {
    if (state.busy) {
      return;
    }
    _streamOpen = false;
    state = state.copyWith(
      messages: const [],
      sessionId: null,
      contextTokens: 0,
      hasImages: false,
    );
  }

  /// Loads a persisted session and reconstructs its ordered timeline.
  Future<void> resume(SessionId id) async {
    if (state.busy || id == state.sessionId) {
      return;
    }
    try {
      final snapshot = await _environment.runtime.loadSession(id);
      _streamOpen = false;
      state = state.copyWith(
        messages: _messagesFromTimeline(snapshot.timeline),
        sessionId: snapshot.session.id,
        workingDirectory: snapshot.session.workingDirectory,
        contextTokens: snapshot.session.lastUsage.totalTokens,
        hasImages: _timelineHasImages(snapshot.timeline),
      );
    } catch (error) {
      _append(WorkspaceMessageKind.error, 'Cannot resume session: $error');
    }
  }

  /// Renames a persisted session's display title.
  Future<void> renameSession(SessionId id, String title) async {
    try {
      await _environment.runtime.renameSession(id, title);
      await refreshSessions();
    } catch (error) {
      _append(WorkspaceMessageKind.error, 'Cannot rename session: $error');
    }
  }

  /// Deletes a persisted session and resets the workspace if it was active.
  Future<void> deleteSession(SessionId id) async {
    try {
      await _environment.runtime.deleteSession(id);
      if (state.sessionId == id) {
        state = state.copyWith(
          messages: const [],
          sessionId: null,
          contextTokens: 0,
          hasImages: false,
        );
      }
      await refreshSessions();
    } catch (error) {
      _append(WorkspaceMessageKind.error, 'Cannot delete session: $error');
    }
  }

  /// Changes the model and resets reasoning effort to its first option.
  void selectModel(ModelDescriptor model) {
    state = state.copyWith(
      activeModel: model,
      reasoningEffort: model.reasoningEfforts.firstOrNull?.value,
    );
    if (state.hasImages &&
        !model.inputCapabilities.contains(ModelInputCapability.image)) {
      final label = model.name.isEmpty ? model.ref.modelId.value : model.name;
      _append(
        WorkspaceMessageKind.notice,
        '$label does not support images; images in this conversation will be omitted.',
      );
    }
  }

  /// Changes the provider-local reasoning effort for subsequent turns.
  void selectReasoningEffort(String? effort) {
    state = state.copyWith(reasoningEffort: effort);
  }

  /// Submits a prompt or executes a TUI-compatible slash command.
  Future<void> send(String rawText) async {
    final text = rawText.trim();
    if (text.isEmpty || state.busy) {
      return;
    }
    if (await _handleSlashCommand(text)) {
      return;
    }

    final cancellation = CancellationToken();
    _cancellation = cancellation;
    _streamOpen = false;
    state = state.copyWith(busy: true);
    _append(WorkspaceMessageKind.user, text);
    try {
      await for (final event in _environment.runtime.run(
        TurnRequest(
          content: [TextContent(text)],
          sessionId: state.sessionId,
          workingDirectory: state.workingDirectory,
          model: state.activeModel.ref,
          reasoningEffort: state.reasoningEffort,
          skills: _selectedSkills(text),
          cancellation: cancellation,
        ),
      )) {
        _handleEvent(event);
      }
    } on TurnCancelledException {
      _append(WorkspaceMessageKind.notice, 'Turn cancelled');
    } catch (error) {
      _append(WorkspaceMessageKind.error, 'Turn failed: $error');
    } finally {
      _cancellation = null;
      _streamOpen = false;
      state = state.copyWith(busy: false);
      await refreshSessions();
    }
  }

  /// Cancels the active runtime operation.
  void cancel() => _cancellation?.cancel();

  Future<bool> _handleSlashCommand(String text) async {
    final parts = text.split(RegExp(r'\s+'));
    switch (parts.first) {
      case '/new':
        if (parts.length != 1) {
          return false;
        }
        newSession();
        return true;
      case '/compact':
        await _compact(parts.skip(1).join(' '));
        return true;
      case '/resume':
        if (parts.length < 2) {
          _append(
            WorkspaceMessageKind.notice,
            'Select a session from the sidebar or provide its id.',
          );
          return true;
        }
        await resume(SessionId(parts[1]));
        return true;
      case '/model':
        _append(
          WorkspaceMessageKind.notice,
          'Choose a model from the input toolbar.',
        );
        return true;
      default:
        return false;
    }
  }

  Future<void> _compact(String instruction) async {
    final id = state.sessionId;
    if (id == null || state.busy) {
      _append(WorkspaceMessageKind.notice, 'No session to compact.');
      return;
    }
    final cancellation = CancellationToken();
    _cancellation = cancellation;
    state = state.copyWith(busy: true);
    try {
      await for (final event in _environment.runtime.compact(
        id,
        instruction: instruction.isEmpty ? null : instruction,
        cancellation: cancellation,
      )) {
        _handleEvent(event);
      }
    } catch (error) {
      _append(WorkspaceMessageKind.error, 'Compaction failed: $error');
    } finally {
      _cancellation = null;
      state = state.copyWith(busy: false);
    }
  }

  void _handleEvent(AgentEvent event) {
    switch (event) {
      case TurnStarted():
        state = state.copyWith(sessionId: event.sessionId);
      case ModelTextDelta(:final delta):
        _finishRunningReasoning();
        _appendDelta(WorkspaceMessageKind.assistant, delta);
      case ModelReasoningDelta(:final delta):
        _appendDelta(WorkspaceMessageKind.reasoning, delta);
      case ToolStarted(:final call):
        _finishRunningReasoning();
        _streamOpen = false;
        state = state.copyWith(
          messages: [
            ...state.messages,
            WorkspaceMessage(
              id: call.call.id.value,
              kind: WorkspaceMessageKind.tool,
              text: '',
              toolName: call.call.name,
              arguments: call.call.arguments,
              startedAt: DateTime.now(),
              isRunning: true,
            ),
          ],
        );
      case ToolFinished(:final result):
        final index = state.messages.lastIndexWhere(
          (message) =>
              message.kind == WorkspaceMessageKind.tool && message.isRunning,
        );
        if (index >= 0) {
          final messages = [...state.messages];
          messages[index] = messages[index].copyWith(
            text: result.content,
            isError: result.isError,
            isRunning: false,
          );
          state = state.copyWith(messages: messages);
        }
        _streamOpen = false;
      case TurnFinished(:final outcome):
        _finishRunningReasoning();
        _streamOpen = false;
        if (outcome.status == TurnStatus.cancelled) {
          _append(WorkspaceMessageKind.notice, 'Turn cancelled');
        } else if (outcome.failure != null) {
          _append(WorkspaceMessageKind.error, outcome.failure!.message);
        }
        state = state.copyWith(contextTokens: outcome.usage.totalTokens);
      case CompactionFinished(:final checkpoint):
        state = state.copyWith(contextTokens: checkpoint.inputTokensAfter);
        _append(
          WorkspaceMessageKind.notice,
          'Context compacted, kept ${checkpoint.keptRecentMessages} recent messages.',
        );
      case CompactionFailed(:final message):
        _append(WorkspaceMessageKind.error, 'Compaction failed: $message');
      default:
        break;
    }
  }

  void _appendDelta(WorkspaceMessageKind kind, String delta) {
    final messages = state.messages;
    final last = messages.lastOrNull;
    if (_streamOpen && last?.kind == kind) {
      state = state.copyWith(
        messages: [
          ...messages.sublist(0, messages.length - 1),
          last!.copyWith(text: last.text + delta),
        ],
      );
    } else {
      state = state.copyWith(
        messages: [
          ...messages,
          WorkspaceMessage(
            id: _nextId(),
            kind: kind,
            text: delta,
            startedAt: kind == WorkspaceMessageKind.reasoning
                ? DateTime.now()
                : null,
            isRunning: kind == WorkspaceMessageKind.reasoning,
          ),
        ],
      );
    }
    _streamOpen = true;
  }

  void _append(WorkspaceMessageKind kind, String text) {
    _streamOpen = false;
    state = state.copyWith(
      messages: [
        ...state.messages,
        WorkspaceMessage(id: _nextId(), kind: kind, text: text),
      ],
    );
  }

  /// Marks the latest streaming reasoning item complete.
  void _finishRunningReasoning() {
    final index = state.messages.lastIndexWhere(
      (message) =>
          message.kind == WorkspaceMessageKind.reasoning && message.isRunning,
    );
    if (index < 0) {
      return;
    }
    final messages = [...state.messages];
    messages[index] = messages[index].copyWith(isRunning: false);
    state = state.copyWith(messages: messages);
  }

  String _nextId() => 'local-${_localId++}';

  ModelDescriptor _descriptorFor(ModelRef ref) {
    for (final model in _environment.models) {
      if (model.ref == ref) {
        return model;
      }
    }
    return ModelDescriptor(ref: ref);
  }

  List<String> _selectedSkills(String text) {
    final available = {
      for (final skill in _environment.skills.summaries) skill.name,
    };
    final selected = <String>[];
    for (final token in text.split(RegExp(r'\s+'))) {
      if (!token.startsWith('/') || token.length == 1) {
        continue;
      }
      final name = token.substring(1);
      if (available.contains(name) && !selected.contains(name)) {
        selected.add(name);
      }
    }
    return selected;
  }

  /// Whether any timeline message carries image content.
  static bool _timelineHasImages(List<TimelineItem> timeline) {
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

  List<WorkspaceMessage> _messagesFromTimeline(List<TimelineItem> timeline) {
    final messages = <WorkspaceMessage>[];
    final calls = <ToolCallId, int>{};
    for (final item in timeline) {
      switch (item) {
        case UserMessageItem(:final content):
          final text = textFromContent(content);
          if (text.isNotEmpty) {
            messages.add(
              WorkspaceMessage(
                id: item.id.value,
                kind: WorkspaceMessageKind.user,
                text: text,
              ),
            );
          }
        case AssistantMessageItem(:final content, :final reasoning):
          if (reasoning.isNotEmpty) {
            messages.add(
              WorkspaceMessage(
                id: _nextId(),
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
}

/// Exposes the workspace state and its controller to presentation code.
final workspaceProvider =
    NotifierProvider.autoDispose<WorkspaceController, WorkspaceState>(
      WorkspaceController.new,
    );
