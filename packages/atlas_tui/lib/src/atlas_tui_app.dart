import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:nocterm/nocterm.dart';

import 'chat_controller.dart';
import 'clipboard.dart';
import 'input_bar.dart';
import 'message_list.dart';
import 'slash_commands.dart';
import 'slash_completion.dart';
import 'slash_popup.dart';
import 'session_status_line.dart';
import 'turn_status_line.dart';

/// The root Nocterm application for Atlas.
///
/// Owns a [ChatController] for the injected runtime and lays out the message
/// list above the input bar. It never constructs providers, tools, or storage.
///
/// The theme follows the terminal: the framework detects light/dark and
/// applies a preset; this app then queries the real background color (OSC 11)
/// and layers a [TuiTheme] that keeps the preset accents but uses the
/// terminal color for the background and surface. In test bindings, where no
/// terminal is available, the preset theme is kept as-is.
final class AtlasTuiApp extends StatefulComponent {
  /// Creates the chat application.
  AtlasTuiApp({
    super.key,
    required this.runtime,
    required this.models,
    this.skills,
    this.workingDirectory,
    this.onQuit,
  });

  /// The runtime that executes turns.
  final AgentSession runtime;

  /// The models the user can switch to with `/model`.
  final List<ModelDescriptor> models;

  /// The skills the user can select with `/skillname`.
  final SkillCatalog? skills;

  /// The working directory for tool execution, or the process directory.
  final String? workingDirectory;

  /// Called when the user submits `/quit`.
  final void Function()? onQuit;

  @override
  State<AtlasTuiApp> createState() => _AtlasTuiAppState();
}

final class _AtlasTuiAppState extends State<AtlasTuiApp> {
  late final ChatController _controller;
  late final TextEditingController _textController;
  late SlashCompleter _slash;
  bool _pickingModel = false;
  bool _pickingEffort = false;
  bool _pickingSession = false;
  List<SessionSummary> _sessionSummaries = const [];
  List<SlashCommand> _sessionCommands = const [];
  ModelDescriptor? _pickedModel;
  Color? _background;

  @override
  void initState() {
    super.initState();
    _controller = ChatController(
      runtime: component.runtime,
      workingDirectory: component.workingDirectory ?? Directory.current.path,
    );
    _textController = TextEditingController();
    _slash = SlashCompleter(commands: _slashCommands);
    _controller.addListener(_refresh);
    _queryBackground();
  }

  /// Queries the terminal background color (OSC 11) so the theme can follow
  /// it. Skipped in test bindings, which have no real terminal.
  Future<void> _queryBackground() async {
    final binding = NoctermBinding.instance;
    if (binding is! TerminalBinding) {
      return;
    }
    final Color? background;
    try {
      background = await binding.terminal.getBackgroundColor(
        timeout: const Duration(milliseconds: 50),
      );
    } catch (_) {
      return; // Terminal does not answer OSC 11; keep the preset theme.
    }
    if (background == null || !mounted) {
      return;
    }
    setState(() => _background = background);
  }

  void _refresh() => setState(() {});

  /// Recomputes the slash popup after input changes.
  void _syncSlash() {
    _slash.sync(_textController.text, _textController.selection.baseOffset);
    setState(() {});
  }

  /// The commands shown in the completion popup: built-ins plus the skills
  /// the user can actually trigger with `/skillname`.
  List<SlashCommand> get _slashCommands => [
    ...slashCommands,
    for (final command in _agentCommands)
      if (validSlashCommandName(command.name) &&
          !slashCommands.any((item) => item.name == command.name))
        SlashCommand(
          name: command.name,
          description: '[Skill] ${command.description}',
          isSkill: true,
        ),
    for (final skill in component.skills?.summaries ?? const <SkillSummary>[])
      if (validSlashCommandName(skill.name) &&
          !slashCommands.any((command) => command.name == skill.name))
        SlashCommand(
          name: skill.name,
          description: '[Skill] ${skill.description}',
          isSkill: true,
        ),
  ];

  List<AgentCommand> get _agentCommands {
    final runtime = component.runtime;
    final session = _controller.sessionId;
    return runtime is PresentationAgentSession && session != null
        ? runtime.commandsFor(session)
        : const [];
  }

  /// The model catalog as slash-style commands for the picker popup.
  ///
  /// Rows show the display name only; lookup matches on that same label.
  List<SlashCommand> get _modelCommands => [
    for (final descriptor in component.models)
      SlashCommand(name: _modelLabel(descriptor)),
  ];

  /// The reasoning efforts of [model] as picker commands, showing the
  /// human-readable name only.
  List<SlashCommand> _effortCommands(ModelDescriptor model) => [
    for (final effort in model.reasoningEfforts)
      SlashCommand(name: _effortLabel(effort)),
  ];

  /// Display label for a model: its name, falling back to the model id.
  String _modelLabel(ModelDescriptor model) =>
      model.name.isEmpty ? model.ref.modelId.value : model.name;

  /// Display label for a reasoning effort: its name, falling back to value.
  String _effortLabel(ReasoningEffortOption effort) =>
      effort.name.isEmpty ? effort.value : effort.name;

  void _enterModelPick() {
    _pickingModel = true;
    _pickingEffort = false;
    _textController.clear();
    _slash = SlashCompleter(commands: _modelCommands);
    _slash.showAll();
    setState(() {});
  }

  /// Advances to the reasoning-effort stage for [model].
  void _enterEffortPick(ModelDescriptor model) {
    _pickedModel = model;
    _pickingModel = false;
    _pickingEffort = true;
    _slash = SlashCompleter(commands: _effortCommands(model));
    _slash.showAll();
    setState(() {});
  }

  /// Opens the session picker with the most recently updated sessions from
  /// the current working directory.
  ///
  /// The picker reuses the slash popup, exactly like the model picker.
  Future<void> _enterSessionPick() async {
    _pickingSession = true;
    _pickingModel = false;
    _pickingEffort = false;
    setState(() {});
    final SessionPage page;
    try {
      page = await _controller.listSessions();
    } catch (error) {
      _controller.addNotice('Cannot list sessions: $error');
      _exitPick();
      return;
    }
    if (!mounted || !_pickingSession) {
      return;
    }
    _sessionSummaries = page.items;
    _sessionCommands = [
      for (final summary in _sessionSummaries)
        SlashCommand(
          name: summary.title.isEmpty ? '(untitled)' : summary.title,
          description: summary.id.value,
        ),
    ];
    _slash = SlashCompleter(commands: _sessionCommands);
    _slash.showAll();
    setState(() {});
  }

  void _exitPick() {
    _pickingModel = false;
    _pickingEffort = false;
    _pickingSession = false;
    _pickedModel = null;
    _slash = SlashCompleter(commands: _slashCommands);
    setState(() {});
  }

  /// Applies a finished model selection, mirroring the Go picker: a model
  /// without efforts switches directly, one with a single effort uses it, and
  /// one with several advances to the effort stage.
  void _applyModelSelection(ModelDescriptor descriptor) {
    final efforts = descriptor.reasoningEfforts;
    switch (efforts.length) {
      case 0:
        _controller.setModel(
          descriptor.ref,
          displayName: _modelLabel(descriptor),
        );
      case 1:
        final effort = efforts.single;
        _controller.setModel(
          descriptor.ref,
          displayName: _modelLabel(descriptor),
          effort: effort.value,
          effortName: _effortLabel(effort),
        );
      default:
        _enterEffortPick(descriptor);
    }
  }

  /// Handles keys while the model picker is open; returns `true` when consumed.
  ///
  /// The input field is read-only in this mode, so events bubble here from the
  /// enclosing [Focusable] instead of the field's own key handler.
  bool _handleModelKey(KeyboardEvent event) {
    if (event.logicalKey == LogicalKey.arrowUp) {
      _slash.move(-1);
      setState(() {});
      return true;
    }
    if (event.logicalKey == LogicalKey.arrowDown) {
      _slash.move(1);
      setState(() {});
      return true;
    }
    if (event.logicalKey == LogicalKey.enter) {
      final command = _slash.selectedCommand;
      if (command != null) {
        final descriptor = component.models.firstWhere(
          (model) => _modelLabel(model) == command.name,
        );
        _applyModelSelection(descriptor);
      } else {
        _exitPick();
      }
      return true;
    }
    if (event.logicalKey == LogicalKey.escape) {
      _exitPick();
      return true;
    }
    return false;
  }

  /// Handles keys while the reasoning-effort picker is open.
  bool _handleEffortKey(KeyboardEvent event) {
    if (event.logicalKey == LogicalKey.arrowUp) {
      _slash.move(-1);
      setState(() {});
      return true;
    }
    if (event.logicalKey == LogicalKey.arrowDown) {
      _slash.move(1);
      setState(() {});
      return true;
    }
    if (event.logicalKey == LogicalKey.enter) {
      final command = _slash.selectedCommand;
      final model = _pickedModel;
      if (command != null && model != null) {
        final effort = model.reasoningEfforts.firstWhere(
          (option) => _effortLabel(option) == command.name,
        );
        _controller.setModel(
          model.ref,
          displayName: _modelLabel(model),
          effort: effort.value,
          effortName: _effortLabel(effort),
        );
      }
      _exitPick();
      return true;
    }
    if (event.logicalKey == LogicalKey.escape) {
      _exitPick();
      return true;
    }
    return false;
  }

  /// Handles keys while the session picker is open; returns `true` when
  /// consumed.
  ///
  /// Enter resolves the selected row by index so duplicated titles still
  /// select the right session; enter is ignored while the list is loading.
  bool _handleSessionKey(KeyboardEvent event) {
    if (event.logicalKey == LogicalKey.arrowUp) {
      _slash.move(-1);
      setState(() {});
      return true;
    }
    if (event.logicalKey == LogicalKey.arrowDown) {
      _slash.move(1);
      setState(() {});
      return true;
    }
    if (event.logicalKey == LogicalKey.enter) {
      final command = _slash.selectedCommand;
      if (command != null) {
        final index = _sessionCommands.indexOf(command);
        final summary = _sessionSummaries[index];
        _exitPick();
        _controller.resume(summary.id).then((resumed) {
          if (resumed) {
            _controller.addNotice(
              'Resumed: ${summary.title.isEmpty ? '(untitled)' : summary.title}',
            );
          }
        });
      }
      return true;
    }
    if (event.logicalKey == LogicalKey.escape) {
      _exitPick();
      return true;
    }
    return false;
  }

  /// Handles keys while the slash popup is open; returns `true` when consumed.
  bool _handleSlashKey(KeyboardEvent event) {
    if (!_slash.active) {
      return false;
    }
    bool handled = true;
    if (event.logicalKey == LogicalKey.arrowUp) {
      _slash.move(-1);
    } else if (event.logicalKey == LogicalKey.arrowDown) {
      _slash.move(1);
    } else if (event.logicalKey == LogicalKey.tab ||
        event.logicalKey == LogicalKey.enter) {
      final command = _slash.selectedCommand;
      if (command != null) {
        final result = _slash.applyToken(_textController.text, command.name);
        _textController.text = result.text;
        _textController.selection = TextSelection.collapsed(
          offset: result.offset,
        );
        _slash.dismiss(result.text);
      }
    } else if (event.logicalKey == LogicalKey.escape) {
      _slash.dismiss(_textController.text);
    } else {
      handled = false;
    }
    if (handled) {
      setState(() {});
    }
    return handled;
  }

  bool _handleKey(KeyboardEvent event) {
    if (_pickingModel) {
      return _handleModelKey(event);
    }
    if (_pickingSession) {
      return _handleSessionKey(event);
    }
    if (_pickingEffort) {
      return _handleEffortKey(event);
    }
    if (_handleSlashKey(event)) {
      return true;
    }
    if (event.logicalKey == LogicalKey.escape && _controller.busy) {
      _controller.cancelTurn();
      return true;
    }
    return false;
  }

  void _submit(String text) {
    if (_controller.busy) {
      // Keep the draft in the input bar until the running turn finishes.
      return;
    }
    final compactInstruction = compactCommandInstruction(text);
    if (compactInstruction != null) {
      _textController.clear();
      _slash.dismiss('');
      _controller.compact(instruction: compactInstruction);
      return;
    }
    final resumeSessionId = resumeCommandSessionID(text);
    if (resumeSessionId != null) {
      _textController.clear();
      _slash.dismiss('');
      if (resumeSessionId.isEmpty) {
        _enterSessionPick();
      } else {
        _controller.resume(SessionId(resumeSessionId)).then((resumed) {
          if (resumed) {
            _controller.addNotice('Resumed: $resumeSessionId');
          }
        });
      }
      return;
    }
    final command = parseSlashCommand(text);
    if (command != null) {
      _textController.clear();
      _slash.dismiss('');
      switch (command.name) {
        case modelCommandName:
          _enterModelPick();
        case newCommandName:
          _controller.reset();
        case quitCommandName:
          component.onQuit?.call();
      }
      return;
    }
    _textController.clear();
    _slash.dismiss('');
    _controller.send(text, selectedSkills: selectedSkillNames(text));
  }

  /// Copies text selected by dragging across the message list.
  void _copySelection(String text) {
    copyToClipboard(text);
  }

  /// The descriptor of the active model: the picker selection when set,
  /// otherwise the runtime default.
  ModelDescriptor _activeModel() {
    final ref = _controller.model ?? component.runtime.defaultModel;
    for (final model in component.models) {
      if (model.ref == ref) {
        return model;
      }
    }
    return ModelDescriptor(ref: ref);
  }

  /// The display name of the active reasoning effort, or null when the model
  /// has no efforts and none was selected.
  String? _activeEffortName(ModelDescriptor model) {
    final effort = _controller.reasoningEffort;
    if (effort != null) {
      for (final option in model.reasoningEfforts) {
        if (option.value == effort) {
          return option.name.isEmpty ? option.value : option.name;
        }
      }
      return effort;
    }
    if (model.reasoningEfforts.isEmpty) {
      return null;
    }
    final defaultEffort = model.reasoningEfforts.first;
    return defaultEffort.name.isEmpty
        ? defaultEffort.value
        : defaultEffort.name;
  }

  @override
  void dispose() {
    _controller.removeListener(_refresh);
    _controller.dispose();
    _textController.dispose();
    super.dispose();
  }

  @override
  Component build(BuildContext context) {
    final activeModel = _activeModel();
    final content = Focusable(
      focused: true,
      onKeyEvent: _handleKey,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: SelectionArea(
              onSelectionCompleted: _copySelection,
              child: MessageList(messages: _controller.messages),
            ),
          ),
          const SizedBox(height: 1),
          TurnStatusLine(
            phase: _controller.turnPhase,
            elapsed: _controller.turnElapsed,
            frame: _controller.frame,
          ),
          if (_slash.active)
            SlashPopup(
              matches: _slash.matches,
              selected: _slash.selected,
              showSlash: !(_pickingModel || _pickingEffort || _pickingSession),
              title: _pickingEffort && _pickedModel != null
                  ? 'Select reasoning effort for ${_modelLabel(_pickedModel!)}'
                  : _pickingModel
                  ? 'Select model'
                  : _pickingSession
                  ? 'Resume session'
                  : null,
            ),
          InputBar(
            controller: _textController,
            busy: _controller.busy,
            readOnly: _pickingModel || _pickingEffort || _pickingSession,
            onSubmitted: _submit,
            onChanged: (_) => _syncSlash(),
            onKeyEvent: _pickingModel || _pickingEffort || _pickingSession
                ? null
                : _handleKey,
          ),
          SessionStatusLine(
            modelName: _modelLabel(activeModel),
            effortName: _activeEffortName(activeModel),
            contextTokens: _controller.contextTokens,
            contextWindow: activeModel.contextWindow,
          ),
        ],
      ),
    );
    final background = _background;
    if (background == null) {
      return content;
    }
    final preset = TuiTheme.of(context);
    final theme = preset.copyWith(background: background, surface: background);
    return SizedBox.expand(
      child: ColoredBox(
        color: theme.background,
        foregroundColor: theme.onBackground,
        obscure: true,
        child: TuiTheme(data: theme, child: content),
      ),
    );
  }
}

/// Runs the Atlas chat application until the user quits.
///
/// Owns the Nocterm bootstrap ([runApp] with a [NoctermApp] root) so the
/// terminal framework stays a dependency of atlas_tui and composition roots
/// never import nocterm directly.
///
/// [onQuit] is invoked when the user submits `/quit`; it defaults to
/// [shutdownApp] so the process exits cleanly.
Future<void> runAtlasTui({
  required AgentSession runtime,
  required List<ModelDescriptor> models,
  SkillCatalog? skills,
  String? workingDirectory,
  void Function()? onQuit,
}) {
  return runApp(
    NoctermApp(
      child: AtlasTuiApp(
        runtime: runtime,
        models: models,
        skills: skills,
        workingDirectory: workingDirectory,
        onQuit: onQuit ?? shutdownApp,
      ),
    ),
  );
}
