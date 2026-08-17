import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:window_manager/window_manager.dart';

import '../../../../app/platform_window.dart';
import '../../../../app/runtime_environment.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';
import '../workspace_metrics.dart';

const _builtInCommands = <(String, String)>[
  ('compact', 'Compact the conversation'),
  ('model', 'Choose a model'),
  ('new', 'Start a new session'),
  ('quit', 'Quit Atlas'),
  ('resume', 'Resume a previous session'),
];

/// Composer with model, reasoning effort, slash completion, send, and cancel.
class ConversationInput extends ConsumerStatefulWidget {
  /// Creates a composer bound to the workspace state.
  const ConversationInput({super.key});

  @override
  ConsumerState<ConversationInput> createState() => _ConversationInputState();
}

class _ConversationInputState extends ConsumerState<ConversationInput> {
  final _textController = TextEditingController();
  final _focusNode = FocusNode();
  var _selectedSuggestion = 0;

  @override
  void initState() {
    super.initState();
    _textController.addListener(_onTextChanged);
  }

  @override
  void dispose() {
    _textController
      ..removeListener(_onTextChanged)
      ..dispose();
    _focusNode.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final suggestions = _suggestions;
    final busy = ref.watch(workspaceProvider.select((s) => s.busy));
    final activeModel = ref.watch(
      workspaceProvider.select((s) => s.activeModel),
    );
    final reasoningEffort = ref.watch(
      workspaceProvider.select((s) => s.reasoningEffort),
    );
    final contextTokens = ref.watch(
      workspaceProvider.select((s) => s.contextTokens),
    );
    final controller = ref.read(workspaceProvider.notifier);
    final environment = ref.watch(runtimeEnvironmentProvider)!;
    return Padding(
      padding: const EdgeInsets.fromLTRB(18, 0, 18, 16),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 660),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (suggestions.isNotEmpty)
              _SlashSuggestions(
                suggestions: suggestions,
                selected: _selectedSuggestion,
                onSelected: _applySuggestion,
              ),
            DecoratedBox(
              decoration: BoxDecoration(
                color: colors.panel,
                borderRadius: BorderRadius.circular(AtlasRadii.surface),
                border: Border.all(color: colors.divider),
              ),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(12, 8, 8, 6),
                child: Column(
                  children: [
                    Focus(
                      onKeyEvent: _handleKey,
                      child: TextField(
                        key: const ValueKey('atlas-prompt-input'),
                        controller: _textController,
                        focusNode: _focusNode,
                        enabled: !busy,
                        minLines: 1,
                        maxLines: 7,
                        keyboardType: TextInputType.multiline,
                        textInputAction: TextInputAction.newline,
                        style: TextStyle(
                          color: colors.textPrimary,
                          fontSize: 14,
                          height: 1.45,
                        ),
                        decoration: InputDecoration(
                          hintText: 'Message Atlas',
                          hintStyle: TextStyle(color: colors.textSecondary),
                          border: InputBorder.none,
                          isCollapsed: true,
                          contentPadding: const EdgeInsets.symmetric(
                            vertical: 4,
                          ),
                        ),
                      ),
                    ),
                    const SizedBox(height: 4),
                    Row(
                      children: [
                        _ModelMenu(
                          models: environment.models,
                          activeModel: activeModel,
                          onChanged: controller.selectModel,
                        ),
                        const SizedBox(width: 4),
                        if (activeModel.reasoningEfforts.isNotEmpty)
                          _EffortMenu(
                            efforts: activeModel.reasoningEfforts,
                            value: reasoningEffort,
                            onChanged: controller.selectReasoningEffort,
                          ),
                        const Spacer(),
                        if (contextTokens > 0)
                          Padding(
                            padding: const EdgeInsets.only(right: 8),
                            child: Text(
                              '$contextTokens tokens',
                              style: TextStyle(
                                color: colors.textSecondary,
                                fontSize: 11,
                              ),
                            ),
                          ),
                        Tooltip(
                          message: busy ? 'Stop' : 'Send',
                          child: IconButton(
                            key: const ValueKey('atlas-send-button'),
                            padding: EdgeInsets.zero,
                            constraints: const BoxConstraints.tightFor(
                              width: 32,
                              height: 32,
                            ),
                            onPressed: busy ? controller.cancel : _submit,
                            icon: DecoratedBox(
                              decoration: BoxDecoration(
                                color: colors.accent,
                                borderRadius: BorderRadius.circular(
                                  AtlasRadii.control,
                                ),
                              ),
                              child: SizedBox.square(
                                dimension: 28,
                                child: Icon(
                                  busy
                                      ? LucideIcons.square
                                      : LucideIcons.arrowUp,
                                  size: 14,
                                  color: colors.canvas,
                                ),
                              ),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  List<(String, String)> get _suggestions {
    final value = _textController.text;
    if (!value.startsWith('/') || value.contains(RegExp(r'\s'))) {
      return const [];
    }
    final query = value.substring(1).toLowerCase();
    final skills = ref.read(runtimeEnvironmentProvider)!.skills.summaries;
    final commands = <(String, String)>[
      ..._builtInCommands,
      for (final skill in skills) (skill.name, skill.description),
    ];
    return commands
        .where((command) => command.$1.toLowerCase().startsWith(query))
        .take(5)
        .toList();
  }

  KeyEventResult _handleKey(FocusNode node, KeyEvent event) {
    if (event is! KeyDownEvent) {
      return KeyEventResult.ignored;
    }
    final suggestions = _suggestions;
    if (suggestions.isNotEmpty &&
        event.logicalKey == LogicalKeyboardKey.arrowDown) {
      setState(() {
        _selectedSuggestion = (_selectedSuggestion + 1) % suggestions.length;
      });
      return KeyEventResult.handled;
    }
    if (suggestions.isNotEmpty &&
        event.logicalKey == LogicalKeyboardKey.arrowUp) {
      setState(() {
        _selectedSuggestion =
            (_selectedSuggestion - 1 + suggestions.length) % suggestions.length;
      });
      return KeyEventResult.handled;
    }
    if (suggestions.isNotEmpty &&
        (event.logicalKey == LogicalKeyboardKey.tab ||
            event.logicalKey == LogicalKeyboardKey.enter)) {
      _applySuggestion(suggestions[_selectedSuggestion]);
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.enter &&
        !HardwareKeyboard.instance.isShiftPressed) {
      _submit();
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  void _applySuggestion((String, String) suggestion) {
    final takesArgument =
        suggestion.$1 == 'compact' || suggestion.$1 == 'resume';
    _textController.text = '/${suggestion.$1}${takesArgument ? ' ' : ''}';
    _textController.selection = TextSelection.collapsed(
      offset: _textController.text.length,
    );
    _focusNode.requestFocus();
  }

  Future<void> _submit() async {
    final text = _textController.text.trim();
    if (text.isEmpty) {
      return;
    }
    if (text == '/quit' && usesManagedDesktopWindow) {
      await windowManager.close();
      return;
    }
    _textController.clear();
    await ref.read(workspaceProvider.notifier).send(text);
    if (mounted) {
      _focusNode.requestFocus();
    }
  }

  void _onTextChanged() {
    if (_selectedSuggestion != 0 || _suggestions.isNotEmpty) {
      setState(() => _selectedSuggestion = 0);
    }
  }
}

class _SlashSuggestions extends StatelessWidget {
  const _SlashSuggestions({
    required this.suggestions,
    required this.selected,
    required this.onSelected,
  });

  final List<(String, String)> suggestions;
  final int selected;
  final ValueChanged<(String, String)> onSelected;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      decoration: BoxDecoration(
        color: colors.panel,
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (final (index, suggestion) in suggestions.indexed)
            InkWell(
              onTap: () => onSelected(suggestion),
              child: Container(
                height: 36,
                padding: const EdgeInsets.symmetric(horizontal: 10),
                color: index == selected ? colors.raised : Colors.transparent,
                child: Row(
                  children: [
                    SizedBox(
                      width: 108,
                      child: Text(
                        '/${suggestion.$1}',
                        style: TextStyle(
                          color: colors.textPrimary,
                          fontFamily: WorkspaceMetrics.monospaceFontFamily,
                          fontSize: 12,
                        ),
                      ),
                    ),
                    Expanded(
                      child: Text(
                        suggestion.$2,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                          color: colors.textSecondary,
                          fontSize: 11.5,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _ModelMenu extends StatelessWidget {
  const _ModelMenu({
    required this.models,
    required this.activeModel,
    required this.onChanged,
  });

  final List<ModelDescriptor> models;
  final ModelDescriptor activeModel;
  final ValueChanged<ModelDescriptor> onChanged;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return DropdownButtonHideUnderline(
      child: DropdownButton<String>(
        key: const ValueKey('atlas-model-menu'),
        value: activeModel.ref.toString(),
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        dropdownColor: colors.panel,
        icon: Icon(
          LucideIcons.chevronDown,
          size: 14,
          color: colors.textSecondary,
        ),
        isDense: true,
        style: TextStyle(color: colors.textSecondary, fontSize: 11.5),
        items: [
          for (final model in models)
            DropdownMenuItem(
              value: model.ref.toString(),
              child: Text(
                model.name.isEmpty ? model.ref.modelId.value : model.name,
              ),
            ),
        ],
        onChanged: (value) {
          if (value == null) {
            return;
          }
          onChanged(
            models.firstWhere((model) => model.ref.toString() == value),
          );
        },
      ),
    );
  }
}

class _EffortMenu extends StatelessWidget {
  const _EffortMenu({
    required this.efforts,
    required this.value,
    required this.onChanged,
  });

  final List<ReasoningEffortOption> efforts;
  final String? value;
  final ValueChanged<String?> onChanged;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? efforts.first.value;
    return DropdownButtonHideUnderline(
      child: DropdownButton<String>(
        key: const ValueKey('atlas-effort-menu'),
        value: current,
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        dropdownColor: colors.panel,
        icon: Icon(
          LucideIcons.chevronDown,
          size: 14,
          color: colors.textSecondary,
        ),
        isDense: true,
        style: TextStyle(color: colors.textSecondary, fontSize: 11.5),
        items: [
          for (final effort in efforts)
            DropdownMenuItem(
              value: effort.value,
              child: Text(effort.name.isEmpty ? effort.value : effort.name),
            ),
        ],
        onChanged: onChanged,
      ),
    );
  }
}
