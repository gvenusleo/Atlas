import 'dart:convert';

import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_markdown_plus_latex/flutter_markdown_plus_latex.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:markdown/markdown.dart' as md;
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';
import '../../application/workspace_message.dart';
import '../workspace_metrics.dart';

/// Scrollable conversation transcript with streaming Markdown and disclosures.
class ConversationView extends ConsumerStatefulWidget {
  /// Creates a transcript bound to the workspace state.
  const ConversationView({super.key});

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
    final messages = ref.watch(workspaceProvider.select((s) => s.messages));
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
          child: ListView.separated(
            controller: _scrollController,
            padding: const EdgeInsets.fromLTRB(24, 28, 24, 20),
            itemCount: messages.length,
            itemBuilder: (context, index) =>
                _MessageView(message: messages[index]),
            separatorBuilder: (_, _) => const SizedBox(height: 6),
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
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 780),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: colors.panel,
            borderRadius: BorderRadius.circular(AtlasRadii.surface),
          ),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
            child: SelectableText(
              text,
              style: TextStyle(
                color: colors.textPrimary,
                fontSize: 14,
                height: 1.5,
              ),
            ),
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
    return Align(
      alignment: Alignment.centerLeft,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 880),
        child: MarkdownBody(
          data: text,
          selectable: false,
          softLineBreak: false,
          builders: {
            'pre': _CodeBlockBuilder(),
            'code': _InlineCodeBuilder(),
            'latex': LatexElementBuilder(
              textStyle: TextStyle(color: colors.textPrimary),
            ),
          },
          checkboxBuilder: (bool checked) => Icon(
            checked ? LucideIcons.squareCheckBig : LucideIcons.square,
            size: 14,
            color: colors.textPrimary,
          ),
          extensionSet: md.ExtensionSet(
            [
              ...md.ExtensionSet.gitHubFlavored.blockSyntaxes,
              LatexBlockSyntax(),
            ],
            [
              ...md.ExtensionSet.gitHubFlavored.inlineSyntaxes,
              LatexInlineSyntax(),
            ],
          ),
          styleSheet: MarkdownStyleSheet(
            a: TextStyle(color: colors.accent, fontSize: 14),
            p: TextStyle(color: colors.textPrimary, fontSize: 14, height: 1.5),
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
            code: TextStyle(
              color: colors.textPrimary,
              backgroundColor: colors.raised,
              fontSize: 12,
              fontFamily: WorkspaceMetrics.monospaceFontFamily,
            ),
            codeblockDecoration: BoxDecoration(
              color: colors.raised,
              borderRadius: BorderRadius.circular(AtlasRadii.surface),
            ),
            blockquote: TextStyle(color: colors.textPrimary, fontSize: 14),
            blockquoteDecoration: BoxDecoration(
              border: Border(left: BorderSide(color: colors.divider, width: 2)),
            ),
            blockquotePadding: const EdgeInsets.only(left: 12),
            horizontalRuleDecoration: BoxDecoration(
              border: Border(top: BorderSide(color: colors.divider)),
            ),
            listBullet: TextStyle(color: colors.textSecondary, fontSize: 14),
            tableBody: TextStyle(color: colors.textPrimary, fontSize: 14),
            checkbox: TextStyle(color: colors.textPrimary, fontSize: 14),
          ),
        ),
      ),
    );
  }
}

/// Renders fenced code blocks full-width with a language label.
class _CodeBlockBuilder extends MarkdownElementBuilder {
  @override
  Widget visitElementAfterWithContext(
    BuildContext context,
    md.Element element,
    TextStyle? preferredStyle,
    TextStyle? parentStyle,
  ) {
    final colors = AtlasColors.of(context);
    final code = element.children
        ?.whereType<md.Element>()
        .where((child) => child.tag == 'code')
        .firstOrNull;
    final language = code?.attributes['class']?.replaceFirst('language-', '');
    return SizedBox(
      width: double.infinity,
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: colors.panel,
          borderRadius: BorderRadius.circular(AtlasRadii.surface),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (language != null && language.isNotEmpty) ...[
              Text(
                language,
                style: TextStyle(
                  color: colors.textSecondary,
                  fontFamily: WorkspaceMetrics.monospaceFontFamily,
                  fontSize: 11,
                ),
              ),
              const SizedBox(height: 6),
            ],
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: SelectableText(
                element.textContent.trimRight(),
                style: TextStyle(
                  color: colors.textPrimary,
                  fontFamily: WorkspaceMetrics.monospaceFontFamily,
                  fontSize: 12,
                  height: 1.45,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Renders inline code with a rounded raised background.
class _InlineCodeBuilder extends MarkdownElementBuilder {
  @override
  Widget visitElementAfterWithContext(
    BuildContext context,
    md.Element element,
    TextStyle? preferredStyle,
    TextStyle? parentStyle,
  ) {
    final colors = AtlasColors.of(context);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1.5),
      decoration: BoxDecoration(
        color: colors.panel,
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        element.textContent,
        style: TextStyle(
          color: colors.textPrimary,
          fontFamily: WorkspaceMetrics.monospaceFontFamily,
          fontSize: 12,
        ),
      ),
    );
  }
}

class _ToolExpansionTile extends StatelessWidget {
  const _ToolExpansionTile({
    required this.icon,
    required this.title,
    required this.child,
  });

  final IconData icon;
  final Widget title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Theme(
      data: Theme.of(context).copyWith(dividerColor: Colors.transparent),
      child: ExpansionTile(
        tilePadding: EdgeInsets.zero,
        childrenPadding: const EdgeInsets.only(left: 6),
        dense: true,
        visualDensity: VisualDensity.compact,
        showTrailingIcon: false,
        title: Row(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Icon(icon, size: 14, color: colors.textSecondary),
            const SizedBox(width: 8),
            title,
          ],
        ),
        children: [
          Container(
            width: double.infinity,
            padding: const EdgeInsets.only(left: 12),
            decoration: BoxDecoration(
              border: BoxBorder.fromLTRB(
                left: BorderSide(color: colors.divider),
              ),
            ),
            child: child,
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
    return _ToolExpansionTile(
      icon: LucideIcons.sparkle,
      title: Row(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Text(
            'Thinking',
            style: TextStyle(color: colors.textSecondary, fontSize: 12),
          ),
          if (message.startedAt != null) ...[
            const SizedBox(width: 4),
            Text(
              '(${_elapsedLabel(message.startedAt!)})',
              style: TextStyle(color: colors.textSecondary, fontSize: 12),
            ),
          ],
        ],
      ),
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

class _ToolMessage extends StatelessWidget {
  const _ToolMessage(this.message);

  final WorkspaceMessage message;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final statusColor = message.isError ? colors.error : colors.textSecondary;
    final details = const JsonEncoder.withIndent(
      '  ',
    ).convert(message.arguments ?? {});
    return Material(
      color: colors.panel,
      borderRadius: BorderRadius.circular(AtlasRadii.surface),
      clipBehavior: Clip.antiAlias,
      child: Theme(
        data: Theme.of(context).copyWith(dividerColor: Colors.transparent),
        child: ExpansionTile(
          tilePadding: const EdgeInsets.symmetric(horizontal: 10),
          childrenPadding: const EdgeInsets.fromLTRB(12, 0, 12, 12),
          dense: true,
          visualDensity: VisualDensity.compact,
          leading: message.isRunning
              ? SizedBox.square(
                  dimension: 14,
                  child: CircularProgressIndicator(
                    strokeWidth: 1.5,
                    color: colors.accent,
                  ),
                )
              : Icon(
                  message.isError
                      ? LucideIcons.circleAlert
                      : LucideIcons.circleCheck,
                  size: 16,
                  color: statusColor,
                ),
          title: Row(
            children: [
              Expanded(
                child: Text(
                  message.toolName ?? 'Tool',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    color: colors.textPrimary,
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                  ),
                ),
              ),
              if (!message.isRunning && message.startedAt != null)
                Text(
                  _elapsedLabel(message.startedAt!),
                  style: TextStyle(color: colors.textSecondary, fontSize: 10.5),
                ),
            ],
          ),
          subtitle: details == '{}'
              ? null
              : Text(
                  _singleLineArguments(message.arguments ?? {}),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(color: colors.textSecondary, fontSize: 11),
                ),
          children: [
            if (details != '{}') _CodeBlock(details),
            if (details != '{}' && message.text.isNotEmpty)
              const SizedBox(height: 8),
            if (message.text.isNotEmpty) _CodeBlock(message.text),
          ],
        ),
      ),
    );
  }

  static String _singleLineArguments(Map<String, Object?> arguments) =>
      arguments.entries
          .map((entry) => '${entry.key}: ${entry.value}')
          .join(', ')
          .replaceAll('\n', ' ');
}

class _CodeBlock extends StatelessWidget {
  const _CodeBlock(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Align(
      alignment: Alignment.centerLeft,
      child: SelectableText(
        text,
        style: TextStyle(
          color: colors.textSecondary,
          fontFamily: WorkspaceMetrics.monospaceFontFamily,
          fontSize: 12,
          height: 1.45,
        ),
      ),
    );
  }
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

String _elapsedLabel(DateTime startedAt) {
  final seconds = DateTime.now().difference(startedAt).inSeconds;
  if (seconds < 60) {
    return 'Worked for ${seconds}s';
  }
  return 'Worked for ${seconds ~/ 60}m ${seconds % 60}s';
}
