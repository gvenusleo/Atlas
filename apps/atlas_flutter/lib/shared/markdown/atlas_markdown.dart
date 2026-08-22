import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_markdown_plus_latex/flutter_markdown_plus_latex.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:markdown/markdown.dart' as md;
import 'package:material_ui/material_ui.dart';

import '../theme/atlas_theme.dart';

/// Renders Markdown with the Atlas conversation styling.
///
/// Supports GitHub-flavored Markdown plus inline and block LaTeX, matching
/// how assistant messages are rendered in the transcript.
class AtlasMarkdown extends StatelessWidget {
  /// Creates a Markdown renderer for [data].
  const AtlasMarkdown({super.key, required this.data, this.fontFamily});

  /// Markdown source to render.
  final String data;

  /// Monospace font family used for code; falls back to `monospace`.
  final String? fontFamily;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final codeFont = fontFamily ?? 'monospace';
    // Force the intrinsic Column to full width so block elements like code
    // fences fill the available space inside scrollable parents.
    return SizedBox(
      width: double.infinity,
      child: MarkdownBody(
        data: data,
        selectable: false,
        softLineBreak: false,
        fitContent: false,
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
          pPadding: const EdgeInsets.symmetric(vertical: 2),
          code: TextStyle(
            color: colors.textPrimary,
            fontSize: 14,
            fontFamily: codeFont,
          ),
          h1: TextStyle(
            color: colors.textPrimary,
            fontSize: 20,
            fontWeight: FontWeight.w600,
          ),
          h1Padding: const EdgeInsets.only(top: 12, bottom: 6),
          h2: TextStyle(
            color: colors.textPrimary,
            fontSize: 18,
            fontWeight: FontWeight.w600,
          ),
          h2Padding: const EdgeInsets.only(top: 12, bottom: 6),
          h3: TextStyle(
            color: colors.textPrimary,
            fontSize: 16,
            fontWeight: FontWeight.w600,
          ),
          h3Padding: const EdgeInsets.only(top: 12, bottom: 6),
          h4: TextStyle(
            color: colors.textPrimary,
            fontSize: 14,
            fontWeight: FontWeight.w600,
          ),
          h4Padding: const EdgeInsets.only(top: 12, bottom: 6),
          h5: TextStyle(
            color: colors.textPrimary,
            fontSize: 14,
            fontWeight: FontWeight.w600,
          ),
          h5Padding: const EdgeInsets.only(top: 12, bottom: 6),
          h6: TextStyle(
            color: colors.textPrimary,
            fontSize: 14,
            fontWeight: FontWeight.w600,
          ),
          h6Padding: const EdgeInsets.only(top: 12, bottom: 6),
          codeblockDecoration: BoxDecoration(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
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
          tableBorder: TableBorder.all(color: colors.divider),
          tableCellsPadding: const EdgeInsets.all(8),
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
