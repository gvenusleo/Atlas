import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_markdown_plus_latex/flutter_markdown_plus_latex.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:markdown/markdown.dart' as md;
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';
import '../../application/workspace_message.dart';
import 'conversation_input.dart';
import 'workspace_controls.dart';
import '../workspace_metrics.dart';

/// Keeps one conversation pane per session so transcript and composer state
/// survive focus changes.
class SessionPaneHost extends ConsumerWidget {
  /// Creates a host for the focused session's transcript and composer.
  const SessionPaneHost({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final activeKey = ref.watch(
      workspaceProvider.select((state) => state.activeKey),
    );
    final keys = ref.watch(
      workspaceProvider.select((state) => [...state.workspaces.keys]),
    );
    final index = keys.indexOf(activeKey);
    return IndexedStack(
      index: index < 0 ? 0 : index,
      children: [
        for (final key in keys)
          _SessionPane(
            key: ValueKey('session-pane-$key'),
            sessionKey: key,
            active: key == activeKey,
          ),
      ],
    );
  }
}

class _SessionPane extends StatelessWidget {
  const _SessionPane({
    super.key,
    required this.sessionKey,
    required this.active,
  });

  final String sessionKey;
  final bool active;

  @override
  Widget build(BuildContext context) {
    return ExcludeFocus(
      excluding: !active,
      child: IgnorePointer(
        ignoring: !active,
        child: Column(
          children: [
            Expanded(child: ConversationView(sessionKey: sessionKey)),
            Align(
              alignment: Alignment.center,
              child: ConversationInput(sessionKey: sessionKey, active: active),
            ),
          ],
        ),
      ),
    );
  }
}

/// Scrollable conversation transcript with streaming Markdown and disclosures.
class ConversationView extends ConsumerStatefulWidget {
  /// Creates a transcript bound to [sessionKey], or the focused session.
  const ConversationView({super.key, this.sessionKey});

  /// Cache key of the session to render. Null follows the focused session.
  final String? sessionKey;

  @override
  ConsumerState<ConversationView> createState() => _ConversationViewState();
}

class _ConversationViewState extends ConsumerState<ConversationView> {
  final _scrollController = ScrollController();
  var _lastMessageCount = 0;
  var _lastTextLength = 0;

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final sessionKey = widget.sessionKey;
    final messages = ref.watch(
      workspaceProvider.select(
        (s) => sessionKey == null
            ? s.messages
            : s.workspaces[sessionKey]?.messages ?? const <WorkspaceMessage>[],
      ),
    );
    final textLength = messages.fold<int>(
      0,
      (length, message) => length + message.text.length,
    );
    if (messages.length != _lastMessageCount || textLength != _lastTextLength) {
      _lastMessageCount = messages.length;
      _lastTextLength = textLength;
      WidgetsBinding.instance.addPostFrameCallback((_) => _scrollToEnd());
    }

    if (messages.isEmpty) {
      return const _ConversationEmptyState();
    }
    return SelectionArea(
      child: Align(
        alignment: Alignment.topCenter,
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 760),
          child: ListView.builder(
            controller: _scrollController,
            padding: const EdgeInsets.fromLTRB(24, 28, 24, 20),
            itemCount: messages.length,
            itemBuilder: (context, index) =>
                _MessageView(message: messages[index]),
          ),
        ),
      ),
    );
  }

  void _scrollToEnd() {
    if (!mounted || !_scrollController.hasClients) {
      return;
    }
    _scrollController.jumpTo(_scrollController.position.maxScrollExtent);
  }
}

class _ConversationEmptyState extends StatelessWidget {
  const _ConversationEmptyState();

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              'Atlas',
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 18,
                fontWeight: FontWeight.w600,
              ),
            ),
            const SizedBox(height: 6),
            Text(
              'Start a conversation',
              style: TextStyle(color: colors.textSecondary, fontSize: 12),
            ),
          ],
        ),
      ),
    );
  }
}

class _MessageView extends StatelessWidget {
  const _MessageView({required this.message});

  final WorkspaceMessage message;

  @override
  Widget build(BuildContext context) {
    return switch (message.kind) {
      WorkspaceMessageKind.user => _UserMessage(message.text),
      WorkspaceMessageKind.assistant => _AssistantMessage(message.text),
      WorkspaceMessageKind.reasoning => _ReasoningMessage(message),
      WorkspaceMessageKind.tool => _ToolMessage(message),
      WorkspaceMessageKind.notice => _NoticeMessage(message.text),
      WorkspaceMessageKind.error => _ErrorMessage(message.text),
    };
  }
}

class _UserMessage extends StatelessWidget {
  const _UserMessage(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Align(
      alignment: Alignment.centerRight,
      child: Container(
        decoration: BoxDecoration(
          color: colors.panel,
          borderRadius: BorderRadius.circular(AtlasRadii.surface),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        margin: const EdgeInsets.symmetric(vertical: 12),
        child: SelectableText(
          text,
          style: TextStyle(
            color: colors.textPrimary,
            fontSize: 14,
            height: 1.5,
          ),
        ),
      ),
    );
  }
}

class _AssistantMessage extends StatelessWidget {
  const _AssistantMessage(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Padding(
      padding: const EdgeInsetsGeometry.symmetric(vertical: 6),
      child: MarkdownBody(
        data: text,
        selectable: false,
        softLineBreak: false,
        builders: {
          'latex': LatexElementBuilder(
            textStyle: TextStyle(color: colors.textPrimary),
          ),
        },
        paddingBuilders: {'hr': _HrPaddingBuilder()},
        checkboxBuilder: (bool checked) => Icon(
          checked ? LucideIcons.squareCheckBig : LucideIcons.square,
          size: 14,
          color: colors.textPrimary,
        ),
        extensionSet: md.ExtensionSet(
          [...md.ExtensionSet.gitHubFlavored.blockSyntaxes, LatexBlockSyntax()],
          [
            ...md.ExtensionSet.gitHubFlavored.inlineSyntaxes,
            LatexInlineSyntax(),
          ],
        ),
        styleSheet: MarkdownStyleSheet(
          a: TextStyle(color: colors.accent, fontSize: 14),
          p: TextStyle(color: colors.textPrimary, fontSize: 14, height: 1.5),
          code: TextStyle(
            color: colors.textPrimary,
            fontSize: 13,
            fontFamily: WorkspaceMetrics.monospaceFontFamily,
          ),
          h1: TextStyle(
            color: colors.textPrimary,
            fontSize: 20,
            fontWeight: FontWeight.w600,
          ),
          h2: TextStyle(
            color: colors.textPrimary,
            fontSize: 18,
            fontWeight: FontWeight.w600,
          ),
          h3: TextStyle(
            color: colors.textPrimary,
            fontSize: 16,
            fontWeight: FontWeight.w600,
          ),
          h4: TextStyle(
            color: colors.textPrimary,
            fontSize: 14,
            fontWeight: FontWeight.w600,
          ),
          h5: TextStyle(
            color: colors.textPrimary,
            fontSize: 14,
            fontWeight: FontWeight.w600,
          ),
          h6: TextStyle(
            color: colors.textPrimary,
            fontSize: 14,
            fontWeight: FontWeight.w600,
          ),
          codeblockDecoration: BoxDecoration(
            borderRadius: BorderRadius.circular(AtlasRadii.surface),
            color: colors.panel,
          ),
          blockquote: TextStyle(color: colors.textPrimary, fontSize: 14),
          blockquoteDecoration: BoxDecoration(
            border: Border(left: BorderSide(color: colors.divider, width: 2)),
          ),
          horizontalRuleDecoration: BoxDecoration(
            border: Border(top: BorderSide(color: colors.divider)),
          ),
          listBullet: TextStyle(color: colors.textSecondary, fontSize: 14),
          tableBody: TextStyle(color: colors.textPrimary, fontSize: 14),
          checkbox: TextStyle(color: colors.textPrimary, fontSize: 14),
        ),
      ),
    );
  }
}

/// Vertical padding around Markdown horizontal rules.
class _HrPaddingBuilder extends MarkdownPaddingBuilder {
  @override
  EdgeInsets getPadding() => const EdgeInsets.symmetric(vertical: 6);
}

/// Collapsed activity row for reasoning and tool results.
class _ActivityDisclosure extends StatefulWidget {
  const _ActivityDisclosure({
    required this.icon,
    required this.title,
    required this.child,
    this.isRunning = false,
    this.isError = false,
  });

  final IconData icon;
  final Widget title;
  final Widget child;
  final bool isRunning;
  final bool isError;

  @override
  State<_ActivityDisclosure> createState() => _ActivityDisclosureState();
}

class _ActivityDisclosureState extends State<_ActivityDisclosure>
    with TickerProviderStateMixin {
  late final AnimationController _pulse;
  late final AnimationController _expand;
  var _expanded = false;

  @override
  void initState() {
    super.initState();
    _pulse = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1200),
    );
    _expand =
        AnimationController(
          vsync: this,
          duration: const Duration(milliseconds: 180),
        )..addStatusListener((status) {
          if (status == AnimationStatus.dismissed && mounted) {
            setState(() {});
          }
        });
    _syncPulse();
  }

  @override
  void didUpdateWidget(covariant _ActivityDisclosure oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.isRunning != widget.isRunning) {
      _syncPulse();
    }
  }

  @override
  void dispose() {
    _pulse.dispose();
    _expand.dispose();
    super.dispose();
  }

  void _syncPulse() {
    if (widget.isRunning) {
      _pulse.repeat(reverse: true);
    } else {
      _pulse.stop();
      _pulse.value = 1;
    }
  }

  void _toggle() {
    setState(() => _expanded = !_expanded);
    if (_expanded) {
      _expand.forward();
    } else {
      _expand.reverse();
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final titleColor = widget.isError ? colors.error : colors.textSecondary;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SelectionContainer.disabled(
            child: WorkspaceHoverSurface(
              borderRadius: BorderRadius.circular(AtlasRadii.control),
              child: TextButton(
                style: ButtonStyle(
                  padding: WidgetStatePropertyAll(
                    const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
                  ),
                  foregroundColor: WidgetStatePropertyAll(titleColor),
                  overlayColor: const WidgetStatePropertyAll(
                    Colors.transparent,
                  ),
                  mouseCursor: WidgetStatePropertyAll(SystemMouseCursors.basic),
                ),
                onPressed: _toggle,
                child: Row(
                  children: [
                    FadeTransition(
                      opacity: widget.isRunning
                          ? Tween<double>(begin: 0.35, end: 1).animate(
                              CurvedAnimation(
                                parent: _pulse,
                                curve: Curves.easeInOut,
                              ),
                            )
                          : const AlwaysStoppedAnimation(1),
                      child: Icon(
                        _expanded ? LucideIcons.chevronDown : widget.icon,
                        size: 14,
                        color: titleColor,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Flexible(child: widget.title),
                  ],
                ),
              ),
            ),
          ),
          SizeTransition(
            sizeFactor: CurvedAnimation(
              parent: _expand,
              curve: Curves.easeOutCubic,
            ),
            alignment: Alignment.topLeft,
            child: _expanded || _expand.isAnimating
                ? Padding(
                    padding: const EdgeInsets.only(left: 12, top: 4, bottom: 4),
                    child: DecoratedBox(
                      decoration: BoxDecoration(
                        border: Border(left: BorderSide(color: colors.divider)),
                      ),
                      child: Padding(
                        padding: const EdgeInsets.only(left: 12),
                        child: widget.child,
                      ),
                    ),
                  )
                : const SizedBox.shrink(),
          ),
        ],
      ),
    );
  }
}

class _ReasoningMessage extends StatelessWidget {
  const _ReasoningMessage(this.message);

  final WorkspaceMessage message;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return _ActivityDisclosure(
      icon: LucideIcons.sparkle,
      title: Text(
        'Thinking',
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(color: colors.textSecondary, fontSize: 12),
      ),
      isRunning: message.isRunning,
      child: SelectableText(
        message.text,
        style: TextStyle(
          color: colors.textSecondary,
          fontSize: 12,
          height: 1.5,
        ),
      ),
    );
  }
}

class _ToolMessage extends ConsumerWidget {
  const _ToolMessage(this.message);

  final WorkspaceMessage message;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final colors = AtlasColors.of(context);
    final workingDirectory = ref.watch(
      workspaceProvider.select((s) => s.workingDirectory),
    );
    return _ActivityDisclosure(
      icon: _toolIcon(message.toolName),
      title: _ToolTitle(
        name: _displayToolName(message.toolName),
        detail: _toolDetail(message, workingDirectory),
        isError: message.isError,
      ),
      isRunning: message.isRunning,
      isError: message.isError,
      child: _toolBody(context, colors),
    );
  }

  Widget _toolBody(BuildContext context, AtlasColors colors) {
    if (message.toolName == 'plan') {
      return _PlanResult(arguments: message.arguments);
    }
    if (message.text.isEmpty) {
      return const SizedBox.shrink();
    }
    return SelectableText(
      message.text,
      style: TextStyle(
        color: colors.textSecondary,
        fontFamily: WorkspaceMetrics.monospaceFontFamily,
        fontSize: 12,
        height: 1.45,
      ),
    );
  }
}

class _PlanResult extends StatelessWidget {
  const _PlanResult({required this.arguments});

  final JsonObject? arguments;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final steps = _planSteps(arguments);
    if (steps.isEmpty) {
      return const SizedBox.shrink();
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (final step in steps)
          Padding(
            padding: const EdgeInsets.only(bottom: 4),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Padding(
                  padding: const EdgeInsets.only(top: 2),
                  child: Icon(
                    _planStatusIcon(step.status),
                    size: 12,
                    color: colors.textSecondary,
                  ),
                ),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    step.step,
                    style: TextStyle(
                      color: colors.textSecondary,
                      fontSize: 12,
                      height: 1.45,
                    ),
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }
}

final class _PlanStep {
  const _PlanStep({required this.step, required this.status});

  final String step;
  final String status;
}

List<_PlanStep> _planSteps(JsonObject? arguments) {
  final raw = arguments?['plan'];
  if (raw is! List) {
    return const [];
  }
  return [
    for (final item in raw)
      if (item is Map && item['step'] is String)
        _PlanStep(
          step: item['step'] as String,
          status: item['status'] is String ? item['status'] as String : '',
        ),
  ];
}

IconData _planStatusIcon(String status) => switch (status) {
  'completed' => LucideIcons.check,
  'in_progress' => LucideIcons.circleDot,
  _ => LucideIcons.circle,
};

/// Maps a tool name to its display icon, falling back to a generic wrench.
IconData _toolIcon(String? toolName) => switch (toolName) {
  'shell' => LucideIcons.terminal,
  'read' => LucideIcons.fileText,
  'write' => LucideIcons.filePen,
  'edit' => LucideIcons.pencil,
  'plan' => LucideIcons.listChecks,
  _ => LucideIcons.wrench,
};

class _ToolTitle extends StatelessWidget {
  const _ToolTitle({
    required this.name,
    required this.detail,
    required this.isError,
  });

  final String name;
  final String? detail;
  final bool isError;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final color = isError ? colors.error : colors.textSecondary;
    return Text.rich(
      TextSpan(
        children: [
          TextSpan(
            text: name,
            style: TextStyle(color: color, fontSize: 12),
          ),
          if (detail != null && detail!.isNotEmpty)
            TextSpan(
              text: ' ($detail)',
              style: TextStyle(
                color: color,
                fontSize: 12,
                fontFamily: WorkspaceMetrics.monospaceFontFamily,
              ),
            ),
        ],
      ),
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }
}

String? _toolDetail(WorkspaceMessage message, String workingDirectory) {
  final arguments = message.arguments ?? const <String, Object?>{};
  switch (message.toolName) {
    case 'read':
    case 'write':
    case 'edit':
      final path = arguments['path'];
      if (path is String && path.isNotEmpty) {
        return _relativePath(path, workingDirectory);
      }
    case 'shell':
      final command = arguments['command'];
      if (command is String && command.isNotEmpty) {
        return command.replaceAll(RegExp(r'\s+'), ' ');
      }
    case 'plan':
      final steps = _planSteps(message.arguments);
      if (steps.isNotEmpty) {
        final completed = steps
            .where((step) => step.status == 'completed')
            .length;
        return '$completed/${steps.length} completed';
      }
  }
  return null;
}

String _displayToolName(String? toolName) {
  final name = toolName ?? 'Tool';
  if (name.isEmpty) {
    return 'Tool';
  }
  return name[0].toUpperCase() + name.substring(1);
}

String _relativePath(String path, String workingDirectory) {
  final normalizedPath = path.replaceAll('\\', '/');
  final normalizedRoot = workingDirectory.replaceAll('\\', '/');
  final root = normalizedRoot.endsWith('/')
      ? normalizedRoot
      : '$normalizedRoot/';
  if (normalizedPath == normalizedRoot) {
    return WorkspaceMetrics.directoryLabel(normalizedPath);
  }
  if (normalizedPath.startsWith(root)) {
    return normalizedPath.substring(root.length);
  }
  return path;
}

class _NoticeMessage extends StatelessWidget {
  const _NoticeMessage(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Center(
      child: Text(
        text,
        textAlign: TextAlign.center,
        style: TextStyle(color: colors.textSecondary, fontSize: 11.5),
      ),
    );
  }
}

class _ErrorMessage extends StatelessWidget {
  const _ErrorMessage(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Align(
      alignment: Alignment.centerLeft,
      child: Text(
        text,
        style: TextStyle(color: colors.error, fontSize: 12, height: 1.45),
      ),
    );
  }
}
