import 'package:atlas_flutter/shared/markdown/atlas_markdown.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  test('parseMarkdownLink accepts http(s) and mailto', () {
    expect(
      parseMarkdownLink('https://example.com/docs'),
      Uri.parse('https://example.com/docs'),
    );
    expect(
      parseMarkdownLink('http://example.com'),
      Uri.parse('http://example.com'),
    );
    expect(
      parseMarkdownLink('mailto:atlas@example.com'),
      Uri.parse('mailto:atlas@example.com'),
    );
  });

  test('parseMarkdownLink rejects empty, relative, and unsafe schemes', () {
    expect(parseMarkdownLink(null), isNull);
    expect(parseMarkdownLink(''), isNull);
    expect(parseMarkdownLink('  '), isNull);
    expect(parseMarkdownLink('/local/path'), isNull);
    expect(parseMarkdownLink('javascript:alert(1)'), isNull);
    expect(parseMarkdownLink('file:///tmp/secret'), isNull);
  });

  testWidgets('tapping a markdown link launches the parsed URI', (
    tester,
  ) async {
    final launched = <Uri>[];
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: AtlasMarkdown(
          data: 'See [docs](https://example.com/docs) please.',
          launchLink: (uri) async {
            launched.add(uri);
            return true;
          },
        ),
      ),
    );
    await tester.pump();

    final text = tester.widget<Text>(find.byType(Text).first);
    final span = text.textSpan! as TextSpan;
    span.visitChildren((inlineSpan) {
      if (inlineSpan is TextSpan) {
        final recognizer = inlineSpan.recognizer;
        if (recognizer is TapGestureRecognizer) {
          recognizer.onTap?.call();
        }
      }
      return true;
    });

    expect(launched, [Uri.parse('https://example.com/docs')]);
  });

  testWidgets('tapping an unsafe markdown link does not launch', (
    tester,
  ) async {
    final launched = <Uri>[];
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: AtlasMarkdown(
          data: 'Do not open [this](javascript:alert(1)).',
          launchLink: (uri) async {
            launched.add(uri);
            return true;
          },
        ),
      ),
    );
    await tester.pump();

    final text = tester.widget<Text>(find.byType(Text).first);
    final span = text.textSpan! as TextSpan;
    span.visitChildren((inlineSpan) {
      if (inlineSpan is TextSpan) {
        final recognizer = inlineSpan.recognizer;
        if (recognizer is TapGestureRecognizer) {
          recognizer.onTap?.call();
        }
      }
      return true;
    });

    expect(launched, isEmpty);
  });

  testWidgets('a failing link launch does not surface an uncaught error', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: AtlasMarkdown(
          data: 'See [docs](https://example.com/docs) please.',
          launchLink: (uri) async => throw Exception('no handler'),
        ),
      ),
    );
    await tester.pump();

    final text = tester.widget<Text>(find.byType(Text).first);
    final span = text.textSpan! as TextSpan;
    span.visitChildren((inlineSpan) {
      if (inlineSpan is TextSpan) {
        final recognizer = inlineSpan.recognizer;
        if (recognizer is TapGestureRecognizer) {
          recognizer.onTap?.call();
        }
      }
      return true;
    });
    await tester.pump();

    // The swallowed launch failure must not leak into the test zone.
    expect(tester.takeException(), isNull);
  });

  testWidgets('body text uses the system font while code keeps its own', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: AtlasMarkdown(data: 'Hello 世界', fontFamily: 'MonoTest'),
      ),
    );
    await tester.pump();

    final body = tester.widget<MarkdownBody>(find.byType(MarkdownBody));
    expect(body.styleSheet!.p!.fontFamily, markdownBodyFontFamily);
    expect(body.styleSheet!.a!.fontFamily, markdownBodyFontFamily);
    expect(body.styleSheet!.code!.fontFamily, 'MonoTest');
  });

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
