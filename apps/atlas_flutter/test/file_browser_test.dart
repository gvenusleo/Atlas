import 'dart:io';

import 'package:atlas_flutter/features/workspace/presentation/widgets/file_browser.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  late Directory tempDir;

  setUp(() {
    tempDir = Directory.systemTemp.createTempSync('atlas_file_browser_test');
  });

  tearDown(() {
    tempDir.deleteSync(recursive: true);
  });

  /// Lets real file-system IO finish, then renders the updated frame.
  /// Directory listing streams items across real and fake event loops, so
  /// alternate real async waits with frame pumps until the tree settles.
  Future<void> settle(WidgetTester tester) async {
    for (var i = 0; i < 20; i++) {
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 50)),
      );
      await tester.pump();
    }
    await tester.pump();
    await tester.pump();
  }

  Future<void> pumpBrowser(WidgetTester tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: Scaffold(body: FileBrowser(workingDirectory: tempDir.path)),
      ),
    );
    await settle(tester);
  }

  testWidgets('shows root entries and expands and collapses folders', (
    tester,
  ) async {
    final sub = Directory('${tempDir.path}/sub')..createSync();
    File('${sub.path}/a.txt').writeAsStringSync('hello');
    File('${tempDir.path}/b.txt').writeAsStringSync('world');

    await pumpBrowser(tester);

    expect(find.text('sub'), findsOneWidget);
    expect(find.text('b.txt'), findsOneWidget);
    expect(find.text('a.txt'), findsNothing);

    await tester.tap(find.text('sub'));
    await settle(tester);
    expect(find.text('a.txt'), findsOneWidget);

    await tester.tap(find.text('sub'));
    await settle(tester);
    expect(find.text('a.txt'), findsNothing);
  });

  testWidgets('keeps expanded children cached across collapse and expand', (
    tester,
  ) async {
    final sub = Directory('${tempDir.path}/sub')..createSync();
    File('${sub.path}/a.txt').writeAsStringSync('hello');

    await pumpBrowser(tester);

    await tester.tap(find.text('sub'));
    await settle(tester);
    expect(find.text('a.txt'), findsOneWidget);

    await tester.tap(find.text('sub'));
    await settle(tester);
    expect(find.text('a.txt'), findsNothing);

    // Re-expanding must not re-read the folder; the child reappears.
    await tester.tap(find.text('sub'));
    await settle(tester);
    expect(find.text('a.txt'), findsOneWidget);
  });

  testWidgets('previews a text file and returns to the tree', (tester) async {
    File('${tempDir.path}/b.txt').writeAsStringSync('hello world');

    await pumpBrowser(tester);

    await tester.tap(find.text('b.txt'));
    await settle(tester);
    expect(find.text('hello world'), findsOneWidget);

    await tester.tap(find.byTooltip('Back to files'));
    await settle(tester);
    expect(find.text('b.txt'), findsOneWidget);
    expect(find.text('hello world'), findsNothing);
  });

  testWidgets('rejects oversized files', (tester) async {
    File(
      '${tempDir.path}/big.txt',
    ).writeAsBytesSync(List.filled(600 * 1024, 65));

    await pumpBrowser(tester);

    await tester.tap(find.text('big.txt'));
    await settle(tester);
    expect(find.textContaining('512 KB'), findsOneWidget);
  });

  testWidgets('rejects binary files', (tester) async {
    File('${tempDir.path}/bin.dat').writeAsBytesSync([0, 1, 2, 3]);

    await pumpBrowser(tester);

    await tester.tap(find.text('bin.dat'));
    await settle(tester);
    expect(find.textContaining('Binary files'), findsOneWidget);
  });

  testWidgets('refresh reloads expanded folders', (tester) async {
    final sub = Directory('${tempDir.path}/sub')..createSync();
    File('${sub.path}/a.txt').writeAsStringSync('hello');

    await pumpBrowser(tester);

    await tester.tap(find.text('sub'));
    await settle(tester);
    expect(find.text('a.txt'), findsOneWidget);

    File('${sub.path}/new.txt').writeAsStringSync('new');
    await tester.tap(find.byTooltip('Refresh files'));
    await settle(tester);
    expect(find.text('new.txt'), findsOneWidget);
  });
}
