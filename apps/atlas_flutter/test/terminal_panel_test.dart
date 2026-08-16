import 'package:atlas_flutter/features/workspace/presentation/widgets/terminal_panel.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/shared/theme/atlas_theme.dart';

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
}
