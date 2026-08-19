import 'package:atlas_flutter/features/workspace/presentation/widgets/terminal_panel.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:terminal_view/terminal_view.dart';

void main() {
  testWidgets('terminal panel mounts and starts a shell', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.dark),
        home: const TerminalPanel(workingDirectory: '/tmp'),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.byType(TerminalPanel), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('uses Zed Ayu Light ANSI colors without bold-as-bright', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: const TerminalPanel(workingDirectory: '/tmp'),
      ),
    );
    await tester.pump();

    final view = tester.widget<TerminalView>(find.byType(TerminalView));
    expect(view.theme.foreground, const Color(0xFF5C6166));
    expect(view.theme.background, const Color(0xFFFCFCFC));
    expect(view.theme.white, const Color(0xFFFCFCFC));
    expect(view.theme.green, const Color(0xFF85B304));
    expect(view.theme.blue, const Color(0xFF3B9EE5));
    expect(view.theme.brightGreen, const Color(0xFFC7D98F));
    expect(view.theme.brightBlue, const Color(0xFFABCDF2));
    expect(view.theme.drawBoldTextInBrightColors, isFalse);
  });

  testWidgets('uses Zed Ayu Dark ANSI colors without bold-as-bright', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.dark),
        home: const TerminalPanel(workingDirectory: '/tmp'),
      ),
    );
    await tester.pump();

    final view = tester.widget<TerminalView>(find.byType(TerminalView));
    expect(view.theme.foreground, const Color(0xFFBFBDB6));
    expect(view.theme.background, const Color(0xFF0D1016));
    expect(view.theme.white, const Color(0xFFBFBDB6));
    expect(view.theme.green, const Color(0xFFAAD84C));
    expect(view.theme.brightBlack, const Color(0xFF545557));
    expect(view.theme.drawBoldTextInBrightColors, isFalse);
  });
}
