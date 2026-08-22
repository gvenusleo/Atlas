import 'package:atlas_flutter/shared/markdown/atlas_markdown.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('block elements fill the width inside scrollable parents', (
    tester,
  ) async {
    // A vertical scroll parent hands the intrinsic Column a loose width
    // constraint; the infinity-width wrapper keeps blocks stretched.
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: Scaffold(
          body: SingleChildScrollView(
            child: AtlasMarkdown(data: '# Title\n\n```txt\nhi\n```\n'),
          ),
        ),
      ),
    );
    await tester.pump();

    expect(tester.getSize(find.byType(MarkdownBody)).width, 800);
    final codeBlock = tester.widget<DecoratedBox>(
      find.byWidgetPredicate(
        (widget) =>
            widget is DecoratedBox &&
            widget.decoration is BoxDecoration &&
            (widget.decoration as BoxDecoration).color != null,
      ),
    );
    expect(tester.getSize(find.byWidget(codeBlock)).width, 800);
  });
}
