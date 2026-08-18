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
  final _modelMenuKey = GlobalKey();
  final _effortMenuKey = GlobalKey();
  final _layerLink = LayerLink();
  final _overlayPortalController = OverlayPortalController();
  var _selectedSuggestion = 0;
  var _modelOpen = false;
  var _effortOpen = false;

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
        child: Stack(
          clipBehavior: Clip.none,
          children: [
            Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (suggestions.isNotEmpty)
                  _SlashSuggestions(
                    suggestions: suggestions,
                    selected: _selectedSuggestion,
                    onSelected: _applySuggestion,
                  ),
                CompositedTransformTarget(
                  link: _layerLink,
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      color: colors.panel,
                      borderRadius: BorderRadius.circular(8),
                      border: Border.all(color: colors.divider),
                    ),
                    child: Padding(
                      padding: const EdgeInsets.all(12),
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
                              maxLines: 6,
                              keyboardType: TextInputType.multiline,
                              textInputAction: TextInputAction.newline,
                              style: TextStyle(
                                color: colors.textPrimary,
                                fontSize: 13,
                                height: 1.4,
                              ),
                              decoration: InputDecoration(
                                hintText: 'Message Atlas',
                                hintStyle: TextStyle(
                                  color: colors.textSecondary,
                                ),
                                border: InputBorder.none,
                                isCollapsed: true,
                              ),
                            ),
                          ),
                          const SizedBox(height: 6),
                          Row(
                            children: [
                              _ModelMenu(
                                models: environment.models,
                                activeModel: activeModel,
                                onTap: _toggleModel,
                              ),
                              const SizedBox(width: 4),
                              if (activeModel.reasoningEfforts.isNotEmpty)
                                _EffortMenu(
                                  efforts: activeModel.reasoningEfforts,
                                  value: reasoningEffort,
                                  onTap: _toggleEffort,
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
                                    width: 28,
                                    height: 28,
                                  ),
                                  onPressed: busy ? controller.cancel : _submit,
                                  icon: DecoratedBox(
                                    decoration: BoxDecoration(
                                      color: _canSend
                                          ? colors.textPrimary
                                          : colors.divider,
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
                                        color: _canSend
                                            ? colors.canvas
                                            : colors.textSecondary,
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
                ),
              ],
            ),
            OverlayPortal(
              controller: _overlayPortalController,
              overlayChildBuilder: (context) => Stack(
                clipBehavior: Clip.none,
                children: [
                  if (_modelOpen || _effortOpen)
                    Positioned.fill(
                      child: Listener(
                        behavior: HitTestBehavior.translucent,
                        onPointerDown: _handleOverlayPointerDown,
                        child: const SizedBox.expand(),
                      ),
                    ),
                  if (_modelOpen)
                    CompositedTransformFollower(
                      link: _layerLink,
                      showWhenUnlinked: false,
                      offset: Offset(
                        0,
                        -(environment.models.length * _ModelMenuCard.rowHeight +
                            _ModelMenuCard.cardPadding * 2 +
                            8),
                      ),
                      child: SizedBox(
                        width: 280,
                        child: _ModelMenuCard(
                          key: _modelMenuKey,
                          models: environment.models,
                          activeModel: activeModel,
                          onSelected: (model) {
                            setState(() => _modelOpen = false);
                            _overlayPortalController.hide();
                            controller.selectModel(model);
                          },
                        ),
                      ),
                    ),
                  if (_effortOpen)
                    CompositedTransformFollower(
                      link: _layerLink,
                      showWhenUnlinked: false,
                      offset: Offset(
                        0,
                        -(activeModel.reasoningEfforts.length *
                                _EffortMenuCard.rowHeight +
                            _EffortMenuCard.cardPadding * 2 +
                            8),
                      ),
                      child: SizedBox(
                        width: 150,
                        child: _EffortMenuCard(
                          key: _effortMenuKey,
                          efforts: activeModel.reasoningEfforts,
                          value: reasoningEffort,
                          onSelected: (value) {
                            setState(() => _effortOpen = false);
                            _overlayPortalController.hide();
                            controller.selectReasoningEffort(value);
                          },
                        ),
                      ),
                    ),
                ],
              ),
              child: const SizedBox.shrink(),
            ),
          ],
        ),
      ),
    );
  }

  /// Whether the composer has text to send.
  bool get _canSend => _textController.text.trim().isNotEmpty;

  /// Closes the floating menus when a pointer lands outside them.
  void _handleOverlayPointerDown(PointerDownEvent event) {
    if (!_modelOpen && !_effortOpen) {
      return;
    }
    final keys = [
      if (_modelOpen) _modelMenuKey,
      if (_effortOpen) _effortMenuKey,
    ];
    for (final key in keys) {
      final box = key.currentContext?.findRenderObject() as RenderBox?;
      if (box != null && box.size.contains(box.globalToLocal(event.position))) {
        return;
      }
    }
    setState(() {
      _modelOpen = false;
      _effortOpen = false;
    });
    _overlayPortalController.hide();
  }

  /// Opens or closes the model picker.
  void _toggleModel() {
    setState(() {
      _modelOpen = !_modelOpen;
      _effortOpen = false;
    });
    if (_modelOpen) {
      _overlayPortalController.show();
    } else {
      _overlayPortalController.hide();
    }
  }

  /// Opens or closes the reasoning-effort picker.
  void _toggleEffort() {
    setState(() {
      _effortOpen = !_effortOpen;
      _modelOpen = false;
    });
    if (_effortOpen) {
      _overlayPortalController.show();
    } else {
      _overlayPortalController.hide();
    }
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
    setState(() {
      if (_selectedSuggestion != 0 || _suggestions.isNotEmpty) {
        _selectedSuggestion = 0;
      }
    });
  }
}

class _SlashSuggestions extends StatelessWidget {
  const _SlashSuggestions({
    required this.suggestions,
    required this.selected,
    required this.onSelected,
  });

  static const _rowHeight = 36.0;

  final List<(String, String)> suggestions;
  final int selected;
  final ValueChanged<(String, String)> onSelected;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      padding: const EdgeInsets.all(4),
      decoration: BoxDecoration(
        color: colors.panel,
        borderRadius: BorderRadius.circular(AtlasRadii.surface),
        border: Border.all(color: colors.divider),
        boxShadow: [
          BoxShadow(
            color: colors.scrim.withValues(alpha: 0.08),
            blurRadius: 12,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Stack(
            children: [
              // Gliding highlight that moves to the selected row.
              AnimatedPositioned(
                duration: const Duration(milliseconds: 200),
                curve: Curves.easeOutCubic,
                left: 0,
                right: 0,
                top: selected * _rowHeight,
                height: _rowHeight,
                child: Container(
                  decoration: BoxDecoration(
                    color: colors.raised,
                    borderRadius: BorderRadius.circular(AtlasRadii.control),
                  ),
                ),
              ),
              Column(
                children: [
                  for (final suggestion in suggestions)
                    InkWell(
                      onTap: () => onSelected(suggestion),
                      child: Container(
                        height: _rowHeight,
                        padding: const EdgeInsets.symmetric(horizontal: 8),
                        child: Row(
                          children: [
                            Text(
                              '/${suggestion.$1}',
                              style: TextStyle(
                                color: colors.textPrimary,
                                fontFamily:
                                    WorkspaceMetrics.monospaceFontFamily,
                                fontSize: 12.5,
                                fontWeight: FontWeight.w500,
                              ),
                            ),
                            const SizedBox(width: 10),
                            Expanded(
                              child: Text(
                                suggestion.$2,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: TextStyle(
                                  color: colors.textSecondary,
                                  fontSize: 12,
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                ],
              ),
            ],
          ),
          Container(
            margin: const EdgeInsets.only(top: 4),
            padding: const EdgeInsets.fromLTRB(8, 6, 8, 4),
            decoration: BoxDecoration(
              border: Border(top: BorderSide(color: colors.divider)),
            ),
            child: Text(
              'Type to search commands',
              style: TextStyle(color: colors.textSecondary, fontSize: 11),
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
    required this.onTap,
  });

  final List<ModelDescriptor> models;
  final ModelDescriptor activeModel;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return InkWell(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      onTap: onTap,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            activeModel.name.isEmpty
                ? activeModel.ref.modelId.value
                : activeModel.name,
            style: TextStyle(
              color: colors.textSecondary,
              fontSize: 12,
              fontWeight: FontWeight.w500,
            ),
          ),
          const SizedBox(width: 4),
          Icon(LucideIcons.chevronDown, size: 12, color: colors.textSecondary),
        ],
      ),
    );
  }
}

/// Floating model picker card with a gliding highlight.
class _ModelMenuCard extends StatefulWidget {
  const _ModelMenuCard({
    super.key,
    required this.models,
    required this.activeModel,
    required this.onSelected,
  });

  static const rowHeight = 30.0;

  /// Vertical padding around the row list, used to size the floating card.
  static const cardPadding = 8.0;

  final List<ModelDescriptor> models;
  final ModelDescriptor activeModel;
  final ValueChanged<ModelDescriptor> onSelected;

  @override
  State<_ModelMenuCard> createState() => _ModelMenuCardState();
}

class _ModelMenuCardState extends State<_ModelMenuCard> {
  int? _hovered;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final selectedIndex = widget.models.indexWhere(
      (model) => model.ref == widget.activeModel.ref,
    );
    final highlightIndex = _hovered ?? selectedIndex;
    return Material(
      color: Colors.transparent,
      child: Container(
        padding: const EdgeInsets.all(_ModelMenuCard.cardPadding),
        decoration: BoxDecoration(
          color: colors.panel,
          borderRadius: BorderRadius.circular(AtlasRadii.surface),
          border: Border.all(color: colors.divider),
          boxShadow: [
            BoxShadow(
              color: colors.scrim.withValues(alpha: 0.08),
              blurRadius: 12,
              offset: const Offset(0, 4),
            ),
          ],
        ),
        child: Stack(
          children: [
            AnimatedPositioned(
              duration: const Duration(milliseconds: 200),
              curve: Curves.easeOutCubic,
              left: 0,
              right: 0,
              top: highlightIndex * _ModelMenuCard.rowHeight,
              height: _ModelMenuCard.rowHeight,
              child: Container(
                decoration: BoxDecoration(
                  color: colors.raised,
                  borderRadius: BorderRadius.circular(AtlasRadii.control),
                ),
              ),
            ),
            Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                for (final (index, model) in widget.models.indexed)
                  InkWell(
                    onHover: (hovered) =>
                        setState(() => _hovered = hovered ? index : null),
                    onTap: () => widget.onSelected(model),
                    child: Container(
                      height: _ModelMenuCard.rowHeight,
                      padding: const EdgeInsets.symmetric(horizontal: 8),
                      child: Row(
                        children: [
                          Expanded(
                            child: Text(
                              model.name.isEmpty
                                  ? model.ref.modelId.value
                                  : model.name,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                color: colors.textPrimary,
                                fontSize: 12.5,
                                fontWeight: FontWeight.w500,
                              ),
                            ),
                          ),
                          if (model.ref == widget.activeModel.ref)
                            Icon(
                              LucideIcons.check,
                              size: 13,
                              color: colors.textPrimary,
                            ),
                        ],
                      ),
                    ),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _EffortMenu extends StatelessWidget {
  const _EffortMenu({
    required this.efforts,
    required this.value,
    required this.onTap,
  });

  final List<ReasoningEffortOption> efforts;
  final String? value;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? (efforts.isEmpty ? '' : efforts.first.value);
    final label = efforts
        .where((effort) => effort.value == current)
        .map((effort) => effort.name.isEmpty ? effort.value : effort.name)
        .firstOrNull;
    return InkWell(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      onTap: onTap,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            label ?? current,
            style: TextStyle(
              color: colors.textSecondary,
              fontSize: 12,
              fontWeight: FontWeight.w500,
            ),
          ),
          const SizedBox(width: 4),
          Icon(LucideIcons.chevronDown, size: 12, color: colors.textSecondary),
        ],
      ),
    );
  }
}

/// Floating reasoning-effort picker card with a gliding highlight.
class _EffortMenuCard extends StatefulWidget {
  const _EffortMenuCard({
    super.key,
    required this.efforts,
    required this.value,
    required this.onSelected,
  });

  static const rowHeight = 30.0;

  /// Vertical padding around the row list, used to size the floating card.
  static const cardPadding = 8.0;

  final List<ReasoningEffortOption> efforts;
  final String? value;
  final ValueChanged<String?> onSelected;

  @override
  State<_EffortMenuCard> createState() => _EffortMenuCardState();
}

class _EffortMenuCardState extends State<_EffortMenuCard> {
  int? _hovered;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current =
        widget.value ??
        (widget.efforts.isEmpty ? '' : widget.efforts.first.value);
    final selectedIndex = widget.efforts.indexWhere(
      (effort) => effort.value == current,
    );
    final highlightIndex = _hovered ?? selectedIndex;
    return Material(
      color: Colors.transparent,
      child: Container(
        padding: const EdgeInsets.all(_EffortMenuCard.cardPadding),
        decoration: BoxDecoration(
          color: colors.panel,
          borderRadius: BorderRadius.circular(AtlasRadii.surface),
          border: Border.all(color: colors.divider),
          boxShadow: [
            BoxShadow(
              color: colors.scrim.withValues(alpha: 0.08),
              blurRadius: 12,
              offset: const Offset(0, 4),
            ),
          ],
        ),
        child: Stack(
          children: [
            AnimatedPositioned(
              duration: const Duration(milliseconds: 200),
              curve: Curves.easeOutCubic,
              left: 0,
              right: 0,
              top: highlightIndex * _EffortMenuCard.rowHeight,
              height: _EffortMenuCard.rowHeight,
              child: Container(
                decoration: BoxDecoration(
                  color: colors.raised,
                  borderRadius: BorderRadius.circular(AtlasRadii.control),
                ),
              ),
            ),
            Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                for (final (index, effort) in widget.efforts.indexed)
                  InkWell(
                    onHover: (hovered) =>
                        setState(() => _hovered = hovered ? index : null),
                    onTap: () => widget.onSelected(effort.value),
                    child: Container(
                      height: _EffortMenuCard.rowHeight,
                      padding: const EdgeInsets.symmetric(horizontal: 8),
                      child: Row(
                        children: [
                          Expanded(
                            child: Text(
                              effort.name.isEmpty ? effort.value : effort.name,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                color: colors.textPrimary,
                                fontSize: 12.5,
                                fontWeight: FontWeight.w500,
                              ),
                            ),
                          ),
                          if (effort.value == current)
                            Icon(
                              LucideIcons.check,
                              size: 13,
                              color: colors.textPrimary,
                            ),
                        ],
                      ),
                    ),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
