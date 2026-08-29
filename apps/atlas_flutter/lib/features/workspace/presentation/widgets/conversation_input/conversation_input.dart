import 'dart:async';
import 'dart:math' as math;

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';
import 'package:morphnext/morphnext.dart';

import '../../../../../app/runtime_environment.dart';
import '../../../../../shared/theme/atlas_theme.dart';
import '../../../application/workspace_controller.dart';
import '../../../application/workspace_state.dart';
import '../../../data/image_attachment.dart';
import 'context_usage_ring.dart';
import 'effort_menu.dart';
import 'floating_menu_card.dart';
import 'input_attachments.dart';
import 'input_menu.dart';
import 'mode_menu.dart';
import 'model_menu.dart';
import 'slash_suggestions.dart';
import '../workspace_controls.dart';

/// Composer with model, reasoning effort, slash completion, send, and cancel.
class ConversationInput extends ConsumerStatefulWidget {
  /// Creates a composer bound to [sessionKey], or the focused session.
  const ConversationInput({super.key, this.sessionKey, this.active = true});

  /// Cache key of the session to compose. Null follows the focused session.
  final String? sessionKey;

  /// Whether this composer is the focused session's input.
  final bool active;

  @override
  ConsumerState<ConversationInput> createState() => _ConversationInputState();
}

class _ConversationInputState extends ConsumerState<ConversationInput> {
  final _textController = TextEditingController();
  final _focusNode = FocusNode();
  final _layerLink = LayerLink();
  final _overlayPortalController = OverlayPortalController();
  final _menuKeys = {for (final menu in InputMenu.values) menu: GlobalKey()};
  final _menus = InputMenuTracker();
  final _images = <PendingImage>[];
  var _selectedSuggestion = 0;
  var _composing = false;
  var _imeCommitPending = false;
  var _attaching = false;
  String? _dismissedText;
  var _lastText = '';

  @override
  void initState() {
    super.initState();
    _textController.addListener(_onTextChanged);
    HardwareKeyboard.instance.addHandler(_onHardwareKey);
  }

  @override
  void dispose() {
    HardwareKeyboard.instance.removeHandler(_onHardwareKey);
    _textController
      ..removeListener(_onTextChanged)
      ..dispose();
    _focusNode.dispose();
    super.dispose();
  }

  SessionWorkspace get _workspace {
    final state = ref.read(workspaceProvider);
    final key = widget.sessionKey;
    return key == null ? state.active : state.workspaces[key] ?? state.active;
  }

  @override
  void didUpdateWidget(covariant ConversationInput oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.active && !widget.active) {
      _closeMenus();
    } else if (!oldWidget.active && widget.active) {
      _focusNode.requestFocus();
    }
  }

  void _closeMenus() {
    if (_menus.open == null) {
      return;
    }
    setState(_menus.close);
    _overlayPortalController.hide();
  }

  @override
  Widget build(BuildContext context) {
    final sessionKey = widget.sessionKey;
    final colors = AtlasColors.of(context);
    final suggestions = _suggestions;
    final busy = ref.watch(
      workspaceProvider.select(
        (s) => sessionKey == null
            ? s.busy
            : s.workspaces[sessionKey]?.busy ?? false,
      ),
    );
    final activeModel = ref.watch(
      workspaceProvider.select(
        (s) => sessionKey == null
            ? s.activeModel
            : s.workspaces[sessionKey]?.activeModel ?? s.activeModel,
      ),
    );
    final reasoningEffort = ref.watch(
      workspaceProvider.select(
        (s) => sessionKey == null
            ? s.reasoningEffort
            : s.workspaces[sessionKey]?.reasoningEffort,
      ),
    );
    final mode = ref.watch(
      workspaceProvider.select(
        (s) => sessionKey == null ? s.mode : s.workspaces[sessionKey]?.mode,
      ),
    );
    final contextTokens = ref.watch(
      workspaceProvider.select(
        (s) => sessionKey == null
            ? s.contextTokens
            : s.workspaces[sessionKey]?.contextTokens ?? 0,
      ),
    );
    final controller = ref.read(workspaceProvider.notifier);
    final environment =
        ref.read(runtimeEnvironmentProvider).environment ??
        (throw StateError('runtime is not ready'));
    return Padding(
      padding: const EdgeInsets.fromLTRB(18, 0, 18, 18),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 760),
        child: Stack(
          clipBehavior: Clip.none,
          children: [
            Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (suggestions.isNotEmpty)
                  SlashSuggestions(
                    suggestions: suggestions,
                    selected: _selectedSuggestion,
                    onHighlighted: (index) =>
                        setState(() => _selectedSuggestion = index),
                    onSelected: _applySuggestion,
                  ),
                CompositedTransformTarget(
                  link: _layerLink,
                  child: Container(
                    decoration: BoxDecoration(
                      color: colors.canvas,
                      borderRadius: BorderRadius.circular(AtlasRadii.surface),
                      border: Border.all(color: colors.divider),
                      boxShadow: [
                        BoxShadow(
                          color: colors.scrim.withValues(alpha: 0.08),
                          blurRadius: 4,
                          offset: const Offset(0, 2),
                        ),
                      ],
                    ),
                    padding: const EdgeInsets.all(8),
                    child: Column(
                      children: [
                        if (_images.isNotEmpty)
                          PendingImageStrip(
                            images: _images,
                            onRemove: _removeImage,
                          ),
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
                              hintText: _images.isEmpty
                                  ? 'Message Atlas'
                                  : 'Add a caption, or send the image',
                              hintStyle: TextStyle(color: colors.textSecondary),
                              border: InputBorder.none,
                              enabledBorder: InputBorder.none,
                              focusedBorder: InputBorder.none,
                              errorBorder: InputBorder.none,
                              isCollapsed: true,
                              contentPadding: const EdgeInsets.symmetric(
                                horizontal: 6,
                                vertical: 12,
                              ),
                            ),
                          ),
                        ),
                        Row(
                          children: [
                            AttachImageButton(
                              enabled: !busy && _canAttachImages(activeModel),
                              attaching: _attaching,
                              onPressed: _pickImages,
                            ),
                            const SizedBox(width: 4),
                            ModelMenu(
                              models: environment.models,
                              activeModel: activeModel,
                              onTap: _toggleModel,
                            ),
                            const SizedBox(width: 4),
                            if (activeModel.reasoningEfforts.isNotEmpty)
                              EffortMenu(
                                efforts: activeModel.reasoningEfforts,
                                value: reasoningEffort,
                                onTap: _toggleEffort,
                              ),
                            if (environment.runtime.modeOptions.isNotEmpty)
                              ModeMenu(
                                modes: environment.runtime.modeOptions,
                                value: mode,
                                onTap: _toggleMode,
                              ),
                            const Spacer(),
                            if (contextTokens > 0)
                              Padding(
                                padding: const EdgeInsets.only(right: 8),
                                child: ContextUsageRing(
                                  usedTokens: contextTokens,
                                  contextWindow: activeModel.contextWindow,
                                ),
                              ),
                            Tooltip(
                              message: busy ? 'Stop' : 'Send',
                              child: WorkspaceHoverSurface(
                                key: const ValueKey('atlas-send-button'),
                                // Sending shifts to the accent color on hover.
                                color: _canSend
                                    ? colors.textPrimary
                                    : colors.divider,
                                hoveredColor: _canSend
                                    ? colors.accent
                                    : colors.divider,
                                borderRadius: BorderRadius.circular(
                                  AtlasRadii.control,
                                ),
                                child: GestureDetector(
                                  behavior: HitTestBehavior.opaque,
                                  onTap: busy
                                      ? () => controller.cancel(
                                          sessionKey: widget.sessionKey,
                                        )
                                      : _submit,
                                  child: SizedBox.square(
                                    dimension: 28,
                                    child: AnimatedMorphIcon(
                                      icon: busy
                                          ? LucideIcons.square
                                          : LucideIcons.arrowUp,
                                      size: 14,
                                      color: _canSend
                                          ? colors.canvas
                                          : colors.textSecondary,
                                      semanticLabel: busy ? 'Stop' : 'Send',
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
            OverlayPortal(
              controller: _overlayPortalController,
              overlayChildBuilder: (context) => Stack(
                clipBehavior: Clip.none,
                children: [
                  if (_menus.open != null)
                    Positioned.fill(
                      child: Listener(
                        behavior: HitTestBehavior.translucent,
                        onPointerDown: _handleOutsidePointerDown,
                        child: const SizedBox.expand(),
                      ),
                    ),
                  if (_menus.isOpen(InputMenu.model))
                    CompositedTransformFollower(
                      link: _layerLink,
                      showWhenUnlinked: false,
                      offset: Offset(
                        0,
                        -(math.min(environment.models.length, maxPickerRows) *
                                ModelMenuCard.rowHeight +
                            ModelMenuCard.cardPadding * 2 +
                            8),
                      ),
                      child: SizedBox(
                        width: 280,
                        child: ModelMenuCard(
                          key: _menuKeys[InputMenu.model],
                          models: environment.models,
                          activeModel: activeModel,
                          highlighted: _menus.highlight(InputMenu.model),
                          onHighlighted: (index) => setState(
                            () => _menus.setHighlight(InputMenu.model, index),
                          ),
                          onSelected: (model) {
                            setState(_menus.close);
                            _overlayPortalController.hide();
                            controller.selectModel(
                              model,
                              sessionKey: widget.sessionKey,
                            );
                          },
                        ),
                      ),
                    ),
                  if (_menus.isOpen(InputMenu.effort))
                    CompositedTransformFollower(
                      link: _layerLink,
                      showWhenUnlinked: false,
                      offset: Offset(
                        0,
                        -(math.min(
                                  activeModel.reasoningEfforts.length,
                                  maxPickerRows,
                                ) *
                                EffortMenuCard.rowHeight +
                            EffortMenuCard.cardPadding * 2 +
                            8),
                      ),
                      child: SizedBox(
                        width: 150,
                        child: EffortMenuCard(
                          key: _menuKeys[InputMenu.effort],
                          efforts: activeModel.reasoningEfforts,
                          value: reasoningEffort,
                          highlighted: _menus.highlight(InputMenu.effort),
                          onHighlighted: (index) => setState(
                            () => _menus.setHighlight(InputMenu.effort, index),
                          ),
                          onSelected: (effort) {
                            setState(_menus.close);
                            _overlayPortalController.hide();
                            controller.selectReasoningEffort(
                              effort.value,
                              sessionKey: widget.sessionKey,
                            );
                          },
                        ),
                      ),
                    ),
                  if (_menus.isOpen(InputMenu.mode))
                    CompositedTransformFollower(
                      link: _layerLink,
                      showWhenUnlinked: false,
                      offset: Offset(
                        0,
                        -(math.min(
                                  environment.runtime.modeOptions.length,
                                  maxPickerRows,
                                ) *
                                ModeMenuCard.rowHeight +
                            ModeMenuCard.cardPadding * 2 +
                            8),
                      ),
                      child: SizedBox(
                        width: 150,
                        child: ModeMenuCard(
                          key: _menuKeys[InputMenu.mode],
                          modes: environment.runtime.modeOptions,
                          value: mode,
                          highlighted: _menus.highlight(InputMenu.mode),
                          onHighlighted: (index) => setState(
                            () => _menus.setHighlight(InputMenu.mode, index),
                          ),
                          onSelected: (selected) {
                            setState(_menus.close);
                            _overlayPortalController.hide();
                            unawaited(
                              controller.selectMode(
                                selected.id,
                                sessionKey: widget.sessionKey,
                              ),
                            );
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

  /// Whether the composer has text or images to send.
  bool get _canSend =>
      _textController.text.trim().isNotEmpty || _images.isNotEmpty;

  bool _canAttachImages(ModelDescriptor model) =>
      model.inputCapabilities.contains(ModelInputCapability.image);

  /// Closes the floating menu when a pointer lands outside it.
  void _handleOutsidePointerDown(PointerDownEvent event) {
    final menu = _menus.open;
    if (menu == null) {
      return;
    }
    final box =
        _menuKeys[menu]?.currentContext?.findRenderObject() as RenderBox?;
    if (box != null && box.size.contains(box.globalToLocal(event.position))) {
      return;
    }
    setState(_menus.close);
    _overlayPortalController.hide();
  }

  /// Opens or closes the model picker.
  void _toggleModel() {
    final environment =
        ref.read(runtimeEnvironmentProvider).environment ??
        (throw StateError('runtime is not ready'));
    final activeModel = _workspace.activeModel;
    _toggleMenu(
      InputMenu.model,
      initialHighlight: environment.models.indexWhere(
        (model) => model.ref == activeModel.ref,
      ),
    );
  }

  /// Opens or closes the reasoning-effort picker.
  void _toggleEffort() {
    final activeModel = _workspace.activeModel;
    final current = _workspace.reasoningEffort;
    _toggleMenu(
      InputMenu.effort,
      initialHighlight: activeModel.reasoningEfforts.indexWhere(
        (effort) => effort.value == current,
      ),
    );
  }

  /// Opens or closes the session-mode picker.
  void _toggleMode() {
    final environment =
        ref.read(runtimeEnvironmentProvider).environment ??
        (throw StateError('runtime is not ready'));
    final current = _workspace.mode;
    _toggleMenu(
      InputMenu.mode,
      initialHighlight: environment.runtime.modeOptions.indexWhere(
        (option) => option.id == current,
      ),
    );
  }

  /// Toggles one floating menu, closing the others, and syncs the portal.
  void _toggleMenu(InputMenu menu, {required int initialHighlight}) {
    setState(() => _menus.toggle(menu, initialHighlight: initialHighlight));
    if (_menus.isOpen(menu)) {
      _overlayPortalController.show();
      _focusNode.requestFocus();
    } else {
      _overlayPortalController.hide();
    }
  }

  List<(String, String)> get _suggestions {
    final value = _textController.value;
    if (_dismissedText == value.text) {
      return const [];
    }
    final token = slashTokenAt(value.text, value.selection.baseOffset);
    if (token == null) {
      return const [];
    }
    final remoteCommands = _remoteCommands;
    final fallbackSkills = remoteCommands.isEmpty
        ? ref.read(runtimeEnvironmentProvider).environment!.skills.summaries
        : const <SkillSummary>[];
    final commands = <(String, String, bool)>[
      for (final command in builtInSlashCommands)
        (command.$1, command.$2, false),
      for (final command in remoteCommands)
        (command.name, '[Skill] ${command.description}', true),
      for (final skill in fallbackSkills)
        (skill.name, '[Skill] ${skill.description}', true),
    ];
    final candidates = token.skillsOnly
        ? [
            for (final command in commands)
              if (command.$3) command,
          ]
        : commands;
    return [
      for (final (name, description, _) in rankSlashCommands(
        token.query,
        candidates,
      ))
        (name, description),
    ];
  }

  /// Slash commands advertised by the agent for the focused session.
  List<AgentCommand> get _remoteCommands {
    final environment = ref.read(runtimeEnvironmentProvider).environment;
    final runtime = environment?.runtime;
    if (runtime == null) {
      return const [];
    }
    final sessionId = ref.read(workspaceProvider).sessionId;
    if (sessionId == null) {
      return const [];
    }
    return runtime.commandsFor(sessionId);
  }

  KeyEventResult _handleKey(FocusNode node, KeyEvent event) {
    if (event is! KeyDownEvent) {
      return KeyEventResult.ignored;
    }
    final suggestions = _suggestions;
    if (event.logicalKey == LogicalKeyboardKey.escape) {
      if (_menus.open != null || suggestions.isNotEmpty) {
        setState(_menus.close);
        _overlayPortalController.hide();
        if (suggestions.isNotEmpty) {
          // Dismiss the popup without clearing the draft; typing again
          // reopens it, matching the TUI behavior.
          _dismissedText = _textController.value.text;
        }
        return KeyEventResult.handled;
      }
      return KeyEventResult.ignored;
    }
    if (suggestions.isNotEmpty) {
      return _handleSuggestionKey(event, suggestions);
    }
    return switch (_menus.open) {
      InputMenu.model => _handleModelMenuKey(event),
      InputMenu.effort => _handleEffortMenuKey(event),
      InputMenu.mode => _handleModeMenuKey(event),
      null => _handleSendKey(event),
    };
  }

  /// Arrow keys move the suggestion highlight; tab or enter applies it.
  KeyEventResult _handleSuggestionKey(
    KeyEvent event,
    List<(String, String)> suggestions,
  ) {
    if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
      setState(() {
        _selectedSuggestion = (_selectedSuggestion + 1) % suggestions.length;
      });
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
      setState(() {
        _selectedSuggestion =
            (_selectedSuggestion - 1 + suggestions.length) % suggestions.length;
      });
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.tab ||
        event.logicalKey == LogicalKeyboardKey.enter) {
      _applySuggestion(suggestions[_selectedSuggestion]);
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  /// Arrow keys move the model highlight; enter selects it.
  KeyEventResult _handleModelMenuKey(KeyEvent event) {
    final models = ref.read(runtimeEnvironmentProvider).environment!.models;
    if (models.isEmpty) {
      return KeyEventResult.ignored;
    }
    if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
      setState(() => _menus.moveHighlight(InputMenu.model, 1, models.length));
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
      setState(() => _menus.moveHighlight(InputMenu.model, -1, models.length));
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.enter) {
      final model = models[_menus.highlight(InputMenu.model)];
      setState(_menus.close);
      _overlayPortalController.hide();
      ref.read(workspaceProvider.notifier).selectModel(model);
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  /// Arrow keys move the effort highlight; enter selects it.
  KeyEventResult _handleEffortMenuKey(KeyEvent event) {
    final efforts = ref.read(workspaceProvider).activeModel.reasoningEfforts;
    if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
      setState(() => _menus.moveHighlight(InputMenu.effort, 1, efforts.length));
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
      setState(
        () => _menus.moveHighlight(InputMenu.effort, -1, efforts.length),
      );
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.enter) {
      final effort = efforts[_menus.highlight(InputMenu.effort)];
      setState(_menus.close);
      _overlayPortalController.hide();
      ref.read(workspaceProvider.notifier).selectReasoningEffort(effort.value);
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  /// Arrow keys move the mode highlight; enter selects it.
  KeyEventResult _handleModeMenuKey(KeyEvent event) {
    final modes = ref
        .read(runtimeEnvironmentProvider)
        .environment!
        .runtime
        .modeOptions;
    if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
      setState(() => _menus.moveHighlight(InputMenu.mode, 1, modes.length));
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
      setState(() => _menus.moveHighlight(InputMenu.mode, -1, modes.length));
      return KeyEventResult.handled;
    }
    if (event.logicalKey == LogicalKeyboardKey.enter) {
      final mode = modes[_menus.highlight(InputMenu.mode)];
      setState(_menus.close);
      _overlayPortalController.hide();
      unawaited(ref.read(workspaceProvider.notifier).selectMode(mode.id));
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  /// Enter sends the draft unless shift is held or the IME is composing.
  KeyEventResult _handleSendKey(KeyEvent event) {
    if (event.logicalKey == LogicalKeyboardKey.enter &&
        !HardwareKeyboard.instance.isShiftPressed) {
      // IME confirmation uses Enter. Ignore it while composing so the engine
      // can commit the candidate, and skip the key that follows compositionend
      // on macOS in the same frame (composing is already false by then).
      if (_composing) {
        return KeyEventResult.ignored;
      }
      if (_imeCommitPending) {
        _imeCommitPending = false;
        return KeyEventResult.handled;
      }
      _submit();
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  void _applySuggestion((String, String) suggestion) {
    final value = _textController.value;
    final token = slashTokenAt(value.text, value.selection.baseOffset);
    if (token == null) {
      return;
    }
    // Replace only the active `/token`, preserving the rest of the draft,
    // and insert a trailing space so arguments can follow (like the TUI).
    final replacement = '/${suggestion.$1}';
    var newText =
        value.text.substring(0, token.start) +
        replacement +
        value.text.substring(token.end);
    var newOffset = token.start + replacement.length;
    if (newOffset >= newText.length) {
      newText = '$newText ';
      newOffset = newText.length;
    } else if (newText[newOffset] == ' ') {
      newOffset += 1;
    } else {
      newText =
          '${newText.substring(0, newOffset)} ${newText.substring(newOffset)}';
      newOffset += 1;
    }
    _textController.value = TextEditingValue(
      text: newText,
      selection: TextSelection.collapsed(offset: newOffset),
    );
    _focusNode.requestFocus();
  }

  Future<void> _submit() async {
    final text = _textController.text.trim();
    if (text.isEmpty && _images.isEmpty) {
      return;
    }
    final images = List<PendingImage>.from(_images);
    _textController.clear();
    setState(_images.clear);
    final sent = await ref
        .read(workspaceProvider.notifier)
        .send(
          text,
          sessionKey: widget.sessionKey,
          images: [for (final image in images) image.toContent()],
        );
    if (!mounted) {
      return;
    }
    if (!sent) {
      _textController.value = TextEditingValue(
        text: text,
        selection: TextSelection.collapsed(offset: text.length),
      );
      setState(() => _images.addAll(images));
    }
    _focusNode.requestFocus();
  }

  void _removeImage(int index) {
    setState(() => _images.removeAt(index));
  }

  Future<void> _pickImages() async {
    if (_attaching || !_canAttachImages(_workspace.activeModel)) {
      return;
    }
    setState(() => _attaching = true);
    try {
      final images = await ref.read(imagePickerProvider)();
      if (!mounted) {
        return;
      }
      _addImages(images);
    } finally {
      if (mounted) {
        setState(() => _attaching = false);
      }
    }
  }

  Future<void> _pasteImages() async {
    if (!_canAttachImages(_workspace.activeModel)) {
      return;
    }
    final images = await ref.read(imageClipboardProvider)();
    if (!mounted || images.isEmpty) {
      return;
    }
    _addImages(images);
  }

  void _addImages(List<PendingImage> images) {
    if (images.isEmpty) {
      return;
    }
    final controller = ref.read(workspaceProvider.notifier);
    final remaining = ImageAttachmentLimits.maxCount - _images.length;
    if (remaining <= 0) {
      controller.notify(
        'You can attach up to ${ImageAttachmentLimits.maxCount} images.',
        sessionKey: widget.sessionKey,
      );
      return;
    }
    final accepted = <PendingImage>[];
    var skippedLarge = false;
    var skippedCount = false;
    for (final image in images) {
      if (accepted.length >= remaining) {
        skippedCount = true;
        continue;
      }
      if (image.bytes.length > ImageAttachmentLimits.maxBytes) {
        skippedLarge = true;
        continue;
      }
      accepted.add(image);
    }
    if (accepted.isNotEmpty) {
      setState(() => _images.addAll(accepted));
    }
    if (skippedCount) {
      controller.notify(
        'You can attach up to ${ImageAttachmentLimits.maxCount} images.',
        sessionKey: widget.sessionKey,
      );
    } else if (skippedLarge) {
      controller.notify(
        'Images larger than 10 MB were skipped.',
        sessionKey: widget.sessionKey,
      );
    }
  }

  bool _onHardwareKey(KeyEvent event) {
    if (event is! KeyDownEvent ||
        !_focusNode.hasFocus ||
        event.logicalKey != LogicalKeyboardKey.keyV) {
      return false;
    }
    final paste =
        HardwareKeyboard.instance.isMetaPressed ||
        (HardwareKeyboard.instance.isControlPressed &&
            defaultTargetPlatform != TargetPlatform.macOS);
    if (!paste) {
      return false;
    }
    unawaited(_pasteImages());
    return false;
  }

  void _onTextChanged() {
    final composing = _textController.value.composing.isValid;
    if (_composing && !composing) {
      _imeCommitPending = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) {
          _imeCommitPending = false;
        }
      });
    }
    _composing = composing;
    final text = _textController.value.text;
    if (text != _lastText) {
      _lastText = text;
      _dismissedText = null;
    }
    setState(() {
      if (_selectedSuggestion != 0 || _suggestions.isNotEmpty) {
        _selectedSuggestion = 0;
      }
    });
  }
}
