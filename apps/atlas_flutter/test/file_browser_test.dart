import 'dart:io';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/file_browser.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
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

  testWidgets('markdown files toggle between source and preview', (
    tester,
  ) async {
    File('${tempDir.path}/readme.md').writeAsStringSync('# Title\n\nbody');

    await pumpBrowser(tester);

    await tester.tap(find.text('readme.md'));
    await settle(tester);

    // Source mode by default; the toggle sits left of the close button.
    expect(find.textContaining('# Title'), findsOneWidget);
    expect(find.byType(MarkdownBody), findsNothing);
    expect(find.byTooltip('Toggle markdown preview'), findsOneWidget);

    await tester.tap(find.byTooltip('Toggle markdown preview'));
    await settle(tester);
    expect(find.byType(MarkdownBody), findsOneWidget);
    expect(find.text('Title'), findsOneWidget);
    expect(find.textContaining('# Title'), findsNothing);

    // Toggling again returns to the raw source.
    await tester.tap(find.byTooltip('Toggle markdown preview'));
    await settle(tester);
    expect(find.byType(MarkdownBody), findsNothing);
    expect(find.textContaining('# Title'), findsOneWidget);
  });

  testWidgets('non-markdown files offer no preview toggle', (tester) async {
    File('${tempDir.path}/notes.txt').writeAsStringSync('plain');

    await pumpBrowser(tester);

    await tester.tap(find.text('notes.txt'));
    await settle(tester);
    expect(find.text('plain'), findsOneWidget);
    expect(find.byTooltip('Toggle markdown preview'), findsNothing);
  });

  testWidgets('preview toolbar shows the path relative to the root', (
    tester,
  ) async {
    final sub = Directory('${tempDir.path}/sub')..createSync();
    File('${sub.path}/a.txt').writeAsStringSync('hello');

    await pumpBrowser(tester);
    await tester.tap(find.text('sub'));
    await settle(tester);
    await tester.tap(find.text('a.txt'));
    await settle(tester);

    expect(find.text('sub${Platform.pathSeparator}a.txt'), findsOneWidget);
  });

  testWidgets('file rows highlight with the raised color on hover', (
    tester,
  ) async {
    File('${tempDir.path}/a.txt').writeAsStringSync('hello');

    await pumpBrowser(tester);

    AnimatedContainer rowSurface() => tester.widget<AnimatedContainer>(
      find
          .ancestor(
            of: find.text('a.txt'),
            matching: find.byType(AnimatedContainer),
          )
          .first,
    );
    expect((rowSurface().decoration as BoxDecoration?)?.color, isNull);

    final gesture = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await gesture.addPointer(location: Offset.zero);
    addTearDown(gesture.removePointer);
    await tester.pump();
    await gesture.moveTo(tester.getCenter(find.text('a.txt')));
    await tester.pump(const Duration(milliseconds: 200));

    expect(
      (rowSurface().decoration as BoxDecoration?)?.color,
      AtlasColors.light.raised,
    );
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

  testWidgets('FileBrowserHost keeps expand state when switching sessions', (
    tester,
  ) async {
    final sub = Directory('${tempDir.path}/sub')..createSync();
    File('${sub.path}/a.txt').writeAsStringSync('hello');
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _EmptyProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(
          RuntimeEnvironment(
            runtime: runtime,
            models: [model],
            skills: _EmptySkillCatalog(),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory(tempDir.path),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MaterialApp(
          theme: buildAtlasTheme(Brightness.light),
          home: Scaffold(
            body: Consumer(
              builder: (context, ref, _) {
                final workspace = ref.watch(workspaceProvider);
                return FileBrowserHost(
                  sessionKey: workspace.activeKey,
                  workingDirectory: tempDir.path,
                );
              },
            ),
          ),
        ),
      ),
    );
    await settle(tester);
    await tester.tap(find.text('sub'));
    await settle(tester);
    expect(find.text('a.txt'), findsOneWidget);

    final controller = container.read(workspaceProvider.notifier);
    await controller.send('first session');
    final firstId = container.read(workspaceProvider).sessionId!;
    controller.newSession();
    await settle(tester);
    expect(find.text('a.txt'), findsNothing);

    await controller.resume(firstId);
    await settle(tester);
    expect(find.text('a.txt'), findsOneWidget);
  });
}

class _FixedWorkingDirectory extends WorkspaceWorkingDirectory {
  _FixedWorkingDirectory(this.path);

  final String path;

  @override
  String build() => path;
}

final class _EmptyProvider implements ModelProvider {
  _EmptyProvider(this.model);

  final ModelRef model;

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('ok')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class _EmptySkillCatalog implements SkillCatalog {
  @override
  Skill? lookup(String name) => null;

  @override
  List<SkillSummary> get summaries => const [];
}
