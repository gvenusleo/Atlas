import 'dart:async';
import 'dart:math' as math;

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:morphnext/morphnext.dart';

import '../../../../app/runtime_environment.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';
import '../../application/workspace_state.dart';
import '../../data/image_attachment.dart';
import '../workspace_metrics.dart';
import 'workspace_controls.dart';

const _builtInCommands = <(String, String)>[
  ('compact', 'Compact the conversation'),
];

/// Maximum slash suggestion rows shown before the popup scrolls.
const _maxSlashPopupRows = 5;

/// Maximum rows shown in the floating model picker; the window follows the
/// highlight so long remote catalogs stay within the viewport.
const _maxModelPopupRows = 8;

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
  final _modelMenuKey = GlobalKey();
  final _effortMenuKey = GlobalKey();
  final _modeMenuKey = GlobalKey();
  final _layerLink = LayerLink();
  final _overlayPortalController = OverlayPortalController();
  var _selectedSuggestion = 0;
  var _modelHighlighted = 0;
  var _effortHighlighted = 0;
  var _modeHighlighted = 0;
  var _modelOpen = false;
  var _effortOpen = false;
  var _modeOpen = false;
  var _composing = false;
  var _imeCommitPending = false;
  var _attaching = false;
  String? _dismissedText;
  var _lastText = '';
  final _images = <PendingImage>[];

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
    if (!_modelOpen && !_effortOpen && !_modeOpen) {
      return;
    }
    setState(() {
      _modelOpen = false;
      _effortOpen = false;
      _modeOpen = false;
    });
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
                  _SlashSuggestions(
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
                          _PendingImageStrip(
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
                            _AttachImageButton(
                              enabled: !busy && _canAttachImages(activeModel),
                              attaching: _attaching,
                              onPressed: _pickImages,
                            ),
                            const SizedBox(width: 4),
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
                            if (environment.runtime.modeOptions.isNotEmpty)
                              _ModeMenu(
                                modes: environment.runtime.modeOptions,
                                value: mode,
                                onTap: _toggleMode,
                              ),
                            const Spacer(),
                            if (contextTokens > 0)
                              Padding(
                                padding: const EdgeInsets.only(right: 8),
                                child: _ContextUsageRing(
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
                        -(math.min(
                                  environment.models.length,
                                  _maxModelPopupRows,
                                ) *
                                _ModelMenuCard.rowHeight +
                            _ModelMenuCard.cardPadding * 2 +
                            8),
                      ),
                      child: SizedBox(
                        width: 280,
                        child: _ModelMenuCard(
                          key: _modelMenuKey,
                          models: environment.models,
                          activeModel: activeModel,
                          highlighted: _modelHighlighted,
                          onHighlighted: (index) =>
                              setState(() => _modelHighlighted = index),
                          onSelected: (model) {
                            setState(() => _modelOpen = false);
                            _overlayPortalController.hide();
                            controller.selectModel(
                              model,
                              sessionKey: widget.sessionKey,
                            );
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
                        -(math.min(
                                  activeModel.reasoningEfforts.length,
                                  _maxModelPopupRows,
                                ) *
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
                          highlighted: _effortHighlighted,
                          onHighlighted: (index) =>
                              setState(() => _effortHighlighted = index),
                          onSelected: (effort) {
                            setState(() => _effortOpen = false);
                            _overlayPortalController.hide();
                            controller.selectReasoningEffort(
                              effort.value,
                              sessionKey: widget.sessionKey,
                            );
                          },
                        ),
                      ),
                    ),
                  if (_modeOpen)
                    CompositedTransformFollower(
                      link: _layerLink,
                      showWhenUnlinked: false,
                      offset: Offset(
                        0,
                        -(math.min(
                                  environment.runtime.modeOptions.length,
                                  _maxModelPopupRows,
                                ) *
                                _ModeMenuCard.rowHeight +
                            _ModeMenuCard.cardPadding * 2 +
                            8),
                      ),
                      child: SizedBox(
                        width: 150,
                        child: _ModeMenuCard(
                          key: _modeMenuKey,
                          modes: environment.runtime.modeOptions,
                          value: mode,
                          highlighted: _modeHighlighted,
                          onHighlighted: (index) =>
                              setState(() => _modeHighlighted = index),
                          onSelected: (selected) {
                            setState(() => _modeOpen = false);
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

  /// Closes the floating menus when a pointer lands outside them.
  void _handleOverlayPointerDown(PointerDownEvent event) {
    if (!_modelOpen && !_effortOpen && !_modeOpen) {
      return;
    }
    final keys = [
      if (_modelOpen) _modelMenuKey,
      if (_effortOpen) _effortMenuKey,
      if (_modeOpen) _modeMenuKey,
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
      _modeOpen = false;
    });
    _overlayPortalController.hide();
  }

  /// Opens or closes the model picker.
  void _toggleModel() {
    final environment =
        ref.read(runtimeEnvironmentProvider).environment ??
        (throw StateError('runtime is not ready'));
    final activeModel = _workspace.activeModel;
    setState(() {
      _modelOpen = !_modelOpen;
      _effortOpen = false;
      if (_modelOpen) {
        _modelHighlighted = environment.models.indexWhere(
          (model) => model.ref == activeModel.ref,
        );
        if (_modelHighlighted < 0) {
          _modelHighlighted = 0;
        }
      }
    });
    if (_modelOpen) {
      _overlayPortalController.show();
      _focusNode.requestFocus();
    } else {
      _overlayPortalController.hide();
    }
  }

  /// Opens or closes the reasoning-effort picker.
  void _toggleEffort() {
    final workspace = _workspace;
    final activeModel = workspace.activeModel;
    final current = workspace.reasoningEffort;
    setState(() {
      _effortOpen = !_effortOpen;
      _modelOpen = false;
      _modeOpen = false;
      if (_effortOpen) {
        _effortHighlighted = activeModel.reasoningEfforts.indexWhere(
          (effort) => effort.value == current,
        );
        if (_effortHighlighted < 0) {
          _effortHighlighted = 0;
        }
      }
    });
    if (_effortOpen) {
      _overlayPortalController.show();
      _focusNode.requestFocus();
    } else {
      _overlayPortalController.hide();
    }
  }

  /// Opens or closes the session-mode picker.
  void _toggleMode() {
    final environment =
        ref.read(runtimeEnvironmentProvider).environment ??
        (throw StateError('runtime is not ready'));
    final current = _workspace.mode;
    setState(() {
      _modeOpen = !_modeOpen;
      _modelOpen = false;
      _effortOpen = false;
      if (_modeOpen) {
        _modeHighlighted = environment.runtime.modeOptions.indexWhere(
          (option) => option.id == current,
        );
        if (_modeHighlighted < 0) {
          _modeHighlighted = 0;
        }
      }
    });
    if (_modeOpen) {
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
    final token = _slashTokenAt(value.text, value.selection.baseOffset);
    if (token == null) {
      return const [];
    }
    final skills = ref
        .read(runtimeEnvironmentProvider)
        .environment!
        .skills
        .summaries;
    final commands = <(String, String, bool)>[
      for (final command in _builtInCommands) (command.$1, command.$2, false),
      for (final skill in skills)
        (skill.name, '[Skill] ${skill.description}', true),
      for (final command in _remoteCommands)
        (command.name, command.description, false),
    ];
    final candidates = token.skillsOnly
        ? [
            for (final command in commands)
              if (command.$3) command,
          ]
        : commands;
    return [
      for (final (name, description, _) in _rank(token.query, candidates))
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
      if (_modelOpen || _effortOpen || _modeOpen || suggestions.isNotEmpty) {
        setState(() {
          _modelOpen = false;
          _effortOpen = false;
          _modeOpen = false;
        });
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
      if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
        setState(() {
          _selectedSuggestion = (_selectedSuggestion + 1) % suggestions.length;
        });
        return KeyEventResult.handled;
      }
      if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
        setState(() {
          _selectedSuggestion =
              (_selectedSuggestion - 1 + suggestions.length) %
              suggestions.length;
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
    if (_modelOpen) {
      final models = ref.read(runtimeEnvironmentProvider).environment!.models;
      if (models.isEmpty) {
        return KeyEventResult.ignored;
      }
      if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
        setState(() {
          _modelHighlighted = (_modelHighlighted + 1) % models.length;
        });
        return KeyEventResult.handled;
      }
      if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
        setState(() {
          _modelHighlighted =
              (_modelHighlighted - 1 + models.length) % models.length;
        });
        return KeyEventResult.handled;
      }
      if (event.logicalKey == LogicalKeyboardKey.enter) {
        setState(() => _modelOpen = false);
        _overlayPortalController.hide();
        ref
            .read(workspaceProvider.notifier)
            .selectModel(models[_modelHighlighted]);
        return KeyEventResult.handled;
      }
      return KeyEventResult.ignored;
    }
    if (_effortOpen) {
      final efforts = ref.read(workspaceProvider).activeModel.reasoningEfforts;
      if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
        setState(() {
          _effortHighlighted = (_effortHighlighted + 1) % efforts.length;
        });
        return KeyEventResult.handled;
      }
      if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
        setState(() {
          _effortHighlighted =
              (_effortHighlighted - 1 + efforts.length) % efforts.length;
        });
        return KeyEventResult.handled;
      }
      if (event.logicalKey == LogicalKeyboardKey.enter) {
        setState(() => _effortOpen = false);
        _overlayPortalController.hide();
        ref
            .read(workspaceProvider.notifier)
            .selectReasoningEffort(efforts[_effortHighlighted].value);
        return KeyEventResult.handled;
      }
      return KeyEventResult.ignored;
    }
    if (_modeOpen) {
      final modes = ref
          .read(runtimeEnvironmentProvider)
          .environment!
          .runtime
          .modeOptions;
      if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
        setState(() {
          _modeHighlighted = (_modeHighlighted + 1) % modes.length;
        });
        return KeyEventResult.handled;
      }
      if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
        setState(() {
          _modeHighlighted =
              (_modeHighlighted - 1 + modes.length) % modes.length;
        });
        return KeyEventResult.handled;
      }
      if (event.logicalKey == LogicalKeyboardKey.enter) {
        setState(() => _modeOpen = false);
        _overlayPortalController.hide();
        unawaited(
          ref
              .read(workspaceProvider.notifier)
              .selectMode(modes[_modeHighlighted].id),
        );
        return KeyEventResult.handled;
      }
      return KeyEventResult.ignored;
    }
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
    final token = _slashTokenAt(value.text, value.selection.baseOffset);
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

/// The slash token under [offset], or null when the cursor is not inside a
/// `/name` token.
///
/// Mirrors the TUI completer: a token whose surroundings are not all
/// whitespace only completes skills, so built-ins stay whole-line commands.
({int start, int end, String query, bool skillsOnly})? _slashTokenAt(
  String text,
  int offset,
) {
  if (text.isEmpty) {
    return null;
  }
  final column = offset.clamp(0, text.length);
  if (column > 0 && _isWhitespace(text[column - 1])) {
    return null;
  }
  var start = column;
  while (start > 0 && !_isWhitespace(text[start - 1])) {
    start--;
  }
  var end = column;
  while (end < text.length && !_isWhitespace(text[end])) {
    end++;
  }
  if (start >= end || text[start] != '/') {
    return null;
  }
  final query = text.substring(start + 1, end);
  if (query.isNotEmpty && !_validAgentCommandName(query)) {
    return null;
  }
  final skillsOnly = '${text.substring(0, start)}${text.substring(end)}'
      .trim()
      .isNotEmpty;
  return (start: start, end: end, query: query, skillsOnly: skillsOnly);
}

/// Ranks commands: exact name first, then prefix, then substring.
List<(String, String, bool)> _rank(
  String query,
  List<(String, String, bool)> commands,
) {
  if (query.isEmpty) {
    return List.of(commands);
  }
  final exact = <(String, String, bool)>[];
  final prefix = <(String, String, bool)>[];
  final contains = <(String, String, bool)>[];
  final lower = query.toLowerCase();
  for (final command in commands) {
    final name = command.$1.toLowerCase();
    if (name == lower) {
      exact.add(command);
    } else if (name.startsWith(lower)) {
      prefix.add(command);
    } else if (name.contains(lower)) {
      contains.add(command);
    }
  }
  return [...exact, ...prefix, ...contains];
}

/// Whether [char] is a whitespace separator delimiting slash tokens.
bool _isWhitespace(String char) =>
    char == ' ' || char == '\t' || char == '\n' || char == '\r';

/// Whether [name] is a valid slash token, matching the TUI character set.
bool _validAgentCommandName(String name) {
  if (name.isEmpty) {
    return false;
  }
  for (final code in name.codeUnits) {
    final isLetter =
        (code >= 0x41 && code <= 0x5A) || (code >= 0x61 && code <= 0x7A);
    final isDigit = code >= 0x30 && code <= 0x39;
    if (!isLetter && !isDigit && code != 0x5F && code != 0x2D && code != 0x2E) {
      return false;
    }
  }
  return true;
}

/// First visible row of a scrolling popup window that keeps the highlight
/// centered, mirroring the TUI slash popup.
int _windowStart(int selected, int count, int maxRows) {
  if (count <= maxRows) {
    return 0;
  }
  final top = selected - (maxRows - 1) ~/ 2;
  return top.clamp(0, count - maxRows);
}

class _AttachImageButton extends StatelessWidget {
  const _AttachImageButton({
    required this.enabled,
    required this.attaching,
    required this.onPressed,
  });

  final bool enabled;
  final bool attaching;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Tooltip(
      message: enabled
          ? 'Attach image'
          : 'Current model does not support images',
      child: WorkspaceHoverSurface(
        enabled: enabled && !attaching,
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        child: IconButton(
          key: const ValueKey('atlas-attach-image'),
          padding: EdgeInsets.zero,
          constraints: const BoxConstraints.tightFor(width: 28, height: 28),
          style: IconButton.styleFrom(overlayColor: Colors.transparent),
          onPressed: enabled && !attaching ? onPressed : null,
          icon: Icon(
            LucideIcons.paperclip,
            size: 14,
            color: enabled ? colors.textSecondary : colors.divider,
          ),
        ),
      ),
    );
  }
}

class _PendingImageStrip extends StatelessWidget {
  const _PendingImageStrip({required this.images, required this.onRemove});

  final List<PendingImage> images;
  final ValueChanged<int> onRemove;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(6, 4, 6, 4),
      child: Align(
        alignment: Alignment.centerLeft,
        child: Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final (index, image) in images.indexed)
              _PendingImageChip(image: image, onRemove: () => onRemove(index)),
          ],
        ),
      ),
    );
  }
}

class _PendingImageChip extends StatelessWidget {
  const _PendingImageChip({required this.image, required this.onRemove});

  final PendingImage image;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return SizedBox(
      width: 56,
      height: 56,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          Positioned.fill(
            child: ClipRRect(
              borderRadius: BorderRadius.circular(AtlasRadii.control),
              child: Image.memory(
                image.bytes,
                fit: BoxFit.cover,
                gaplessPlayback: true,
              ),
            ),
          ),
          Positioned(
            top: -4,
            right: -4,
            child: Tooltip(
              message: 'Remove image',
              child: Material(
                color: colors.canvas,
                shape: const CircleBorder(),
                child: InkWell(
                  customBorder: const CircleBorder(),
                  onTap: onRemove,
                  child: SizedBox.square(
                    dimension: 18,
                    child: Icon(
                      LucideIcons.x,
                      size: 11,
                      color: colors.textSecondary,
                    ),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Floating picker card with a gliding highlight driven by hover or keys.
class _FloatingMenuCard<T> extends StatelessWidget {
  const _FloatingMenuCard({
    super.key,
    required this.items,
    required this.selectedIndex,
    required this.highlighted,
    required this.onHighlighted,
    required this.onSelected,
    required this.rowHeight,
    required this.cardPadding,
    required this.itemBuilder,
    this.maxVisibleRows,
  });

  final List<T> items;
  final int selectedIndex;
  final int highlighted;
  final ValueChanged<int> onHighlighted;
  final ValueChanged<T> onSelected;
  final double rowHeight;
  final double cardPadding;
  final Widget Function(BuildContext context, T item, int index) itemBuilder;

  /// Maximum rows rendered at once; the window follows the highlight.
  final int? maxVisibleRows;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final rows = maxVisibleRows == null
        ? items.length
        : math.min(maxVisibleRows!, items.length);
    final windowStart = maxVisibleRows == null
        ? 0
        : _windowStart(highlighted, items.length, maxVisibleRows!);
    final visible = maxVisibleRows == null
        ? items
        : items.sublist(windowStart, windowStart + rows);
    return Material(
      color: Colors.transparent,
      child: Container(
        padding: EdgeInsets.all(cardPadding),
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
        child: Stack(
          children: [
            AnimatedPositioned(
              duration: const Duration(milliseconds: 200),
              curve: Curves.easeOutCubic,
              left: 0,
              right: 0,
              top: (highlighted - windowStart) * rowHeight,
              height: rowHeight,
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
                for (final (index, item) in visible.indexed)
                  InkWell(
                    onHover: (hovered) => onHighlighted(
                      hovered ? windowStart + index : selectedIndex,
                    ),
                    onTap: () => onSelected(item),
                    child: Container(
                      height: rowHeight,
                      padding: const EdgeInsets.symmetric(horizontal: 8),
                      child: itemBuilder(context, item, windowStart + index),
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

class _SlashSuggestions extends StatelessWidget {
  const _SlashSuggestions({
    required this.suggestions,
    required this.selected,
    required this.onHighlighted,
    required this.onSelected,
  });

  static const _rowHeight = 30.0;

  final List<(String, String)> suggestions;
  final int selected;
  final ValueChanged<int> onHighlighted;
  final ValueChanged<(String, String)> onSelected;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      child: _FloatingMenuCard(
        items: suggestions,
        selectedIndex: 0,
        highlighted: selected,
        onHighlighted: onHighlighted,
        onSelected: onSelected,
        rowHeight: _rowHeight,
        cardPadding: 8,
        maxVisibleRows: _maxSlashPopupRows,
        itemBuilder: (context, suggestion, index) => Row(
          children: [
            Text(
              '/${suggestion.$1}',
              style: TextStyle(
                color: colors.textPrimary,
                fontFamily: WorkspaceMetrics.monospaceFontFamily,
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
                style: TextStyle(color: colors.textSecondary, fontSize: 12),
              ),
            ),
          ],
        ),
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
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: TextButton(
        style: ButtonStyle(
          padding: WidgetStatePropertyAll(
            const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
          ),
          overlayColor: const WidgetStatePropertyAll(Colors.transparent),
        ),
        onPressed: onTap,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Icon(LucideIcons.package, size: 12, color: colors.textSecondary),
            const SizedBox(width: 6),
            Text(
              activeModel.name.isEmpty
                  ? activeModel.ref.modelId.value
                  : activeModel.name,
              style: TextStyle(color: colors.textSecondary, fontSize: 12),
            ),
          ],
        ),
      ),
    );
  }
}

/// Floating model picker card with a gliding highlight.
class _ModelMenuCard extends StatelessWidget {
  const _ModelMenuCard({
    super.key,
    required this.models,
    required this.activeModel,
    required this.highlighted,
    required this.onHighlighted,
    required this.onSelected,
  });

  static const rowHeight = 30.0;

  /// Vertical padding around the row list, used to size the floating card.
  static const cardPadding = 8.0;

  final List<ModelDescriptor> models;
  final ModelDescriptor activeModel;
  final int highlighted;
  final ValueChanged<int> onHighlighted;
  final ValueChanged<ModelDescriptor> onSelected;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final selectedIndex = models.indexWhere(
      (model) => model.ref == activeModel.ref,
    );
    return _FloatingMenuCard(
      items: models,
      selectedIndex: selectedIndex,
      highlighted: highlighted,
      onHighlighted: onHighlighted,
      onSelected: onSelected,
      rowHeight: rowHeight,
      cardPadding: cardPadding,
      maxVisibleRows: _maxModelPopupRows,
      itemBuilder: (context, model, index) => Row(
        children: [
          Expanded(
            child: Text(
              model.name.isEmpty ? model.ref.modelId.value : model.name,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: colors.textPrimary,
                fontSize: 12.5,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
          if (model.ref == activeModel.ref)
            Icon(LucideIcons.check, size: 13, color: colors.textPrimary),
        ],
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
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: TextButton(
        style: ButtonStyle(
          padding: WidgetStatePropertyAll(
            const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
          ),
          overlayColor: const WidgetStatePropertyAll(Colors.transparent),
        ),
        onPressed: onTap,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Icon(LucideIcons.brain, size: 12, color: colors.textSecondary),
            const SizedBox(width: 6),
            Text(
              label ?? current,
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Session-mode selector shown when the agent advertises operating modes.
class _ModeMenu extends StatelessWidget {
  const _ModeMenu({
    required this.modes,
    required this.value,
    required this.onTap,
  });

  final List<ModeOption> modes;
  final String? value;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? (modes.isEmpty ? '' : modes.first.id);
    final label = modes
        .where((option) => option.id == current)
        .map((option) => option.name.isEmpty ? option.id : option.name)
        .firstOrNull;
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: TextButton(
        style: ButtonStyle(
          padding: WidgetStatePropertyAll(
            const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
          ),
          overlayColor: const WidgetStatePropertyAll(Colors.transparent),
        ),
        onPressed: onTap,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Icon(LucideIcons.layoutGrid, size: 12, color: colors.textSecondary),
            const SizedBox(width: 6),
            Text(
              label ?? current,
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Floating session-mode picker card with a gliding highlight.
class _ModeMenuCard extends StatelessWidget {
  const _ModeMenuCard({
    super.key,
    required this.modes,
    required this.value,
    required this.highlighted,
    required this.onHighlighted,
    required this.onSelected,
  });

  static const rowHeight = 30.0;

  /// Vertical padding around the row list, used to size the floating card.
  static const cardPadding = 8.0;

  final List<ModeOption> modes;
  final String? value;
  final int highlighted;
  final ValueChanged<int> onHighlighted;
  final ValueChanged<ModeOption> onSelected;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? (modes.isEmpty ? '' : modes.first.id);
    final selectedIndex = modes.indexWhere((option) => option.id == current);
    return _FloatingMenuCard(
      items: modes,
      selectedIndex: selectedIndex,
      highlighted: highlighted,
      onHighlighted: onHighlighted,
      onSelected: onSelected,
      rowHeight: rowHeight,
      cardPadding: cardPadding,
      maxVisibleRows: _maxModelPopupRows,
      itemBuilder: (context, option, index) => Row(
        children: [
          Expanded(
            child: Text(
              option.name.isEmpty ? option.id : option.name,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: colors.textPrimary,
                fontSize: 12.5,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
          if (option.id == current)
            Icon(LucideIcons.check, size: 13, color: colors.textPrimary),
        ],
      ),
    );
  }
}

/// Floating reasoning-effort picker card with a gliding highlight.
class _EffortMenuCard extends StatelessWidget {
  const _EffortMenuCard({
    super.key,
    required this.efforts,
    required this.value,
    required this.highlighted,
    required this.onHighlighted,
    required this.onSelected,
  });

  static const rowHeight = 30.0;

  /// Vertical padding around the row list, used to size the floating card.
  static const cardPadding = 8.0;

  final List<ReasoningEffortOption> efforts;
  final String? value;
  final int highlighted;
  final ValueChanged<int> onHighlighted;
  final ValueChanged<ReasoningEffortOption> onSelected;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? (efforts.isEmpty ? '' : efforts.first.value);
    final selectedIndex = efforts.indexWhere(
      (effort) => effort.value == current,
    );
    return _FloatingMenuCard(
      items: efforts,
      selectedIndex: selectedIndex,
      highlighted: highlighted,
      onHighlighted: onHighlighted,
      onSelected: onSelected,
      rowHeight: rowHeight,
      cardPadding: cardPadding,
      maxVisibleRows: _maxModelPopupRows,
      itemBuilder: (context, effort, index) => Row(
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
            Icon(LucideIcons.check, size: 13, color: colors.textPrimary),
        ],
      ),
    );
  }
}

/// Formats a token count for compact hover labels such as `128k`.
String compactTokenCount(int tokens) {
  final value = tokens < 0 ? 0 : tokens;
  if (value < 1000) {
    return '$value';
  }
  if (value % 1000 == 0) {
    return '${value ~/ 1000}k';
  }
  final tenths = (value / 100).round() / 10;
  if (tenths == tenths.truncateToDouble()) {
    return '${tenths.toInt()}k';
  }
  return '${tenths}k';
}

/// Formats context usage as `10% · 10k/100k`.
String contextUsageLabel(int usedTokens, int contextWindow) {
  final used = compactTokenCount(usedTokens);
  if (contextWindow <= 0) {
    return used;
  }
  final percent = usedTokens <= 0
      ? 0
      : (usedTokens * 100 / contextWindow).clamp(0, 100).round();
  return '$percent% · $used/${compactTokenCount(contextWindow)}';
}

/// Quiet context-usage ring shown beside the send control.
class _ContextUsageRing extends StatelessWidget {
  const _ContextUsageRing({
    required this.usedTokens,
    required this.contextWindow,
  });

  final int usedTokens;
  final int contextWindow;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final progress = contextWindow <= 0
        ? 0.0
        : (usedTokens / contextWindow).clamp(0.0, 1.0);
    final fill = progress >= 0.95
        ? colors.error
        : progress >= 0.8
        ? colors.accent
        : colors.textSecondary;
    final label = contextUsageLabel(usedTokens, contextWindow);
    return Tooltip(
      message: label,
      child: SizedBox.square(
        key: const ValueKey('atlas-context-usage'),
        dimension: 18,
        child: CustomPaint(
          painter: _ContextUsageRingPainter(
            progress: progress,
            trackColor: colors.divider,
            fillColor: fill,
          ),
        ),
      ),
    );
  }
}

class _ContextUsageRingPainter extends CustomPainter {
  _ContextUsageRingPainter({
    required this.progress,
    required this.trackColor,
    required this.fillColor,
  });

  final double progress;
  final Color trackColor;
  final Color fillColor;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    const stroke = 2.0;
    final radius = (math.min(size.width, size.height) - stroke) / 2;
    final track = Paint()
      ..color = trackColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke;
    canvas.drawCircle(center, radius, track);
    if (progress <= 0) {
      return;
    }
    final fill = Paint()
      ..color = fillColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = StrokeCap.round;
    canvas.drawArc(
      Rect.fromCircle(center: center, radius: radius),
      -math.pi / 2,
      2 * math.pi * progress,
      false,
      fill,
    );
  }

  @override
  bool shouldRepaint(covariant _ContextUsageRingPainter oldDelegate) {
    return oldDelegate.progress != progress ||
        oldDelegate.trackColor != trackColor ||
        oldDelegate.fillColor != fillColor;
  }
}
