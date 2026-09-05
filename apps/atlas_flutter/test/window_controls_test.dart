import 'package:atlas_flutter/features/workspace/presentation/widgets/workspace_controls.dart';
import 'package:atlas_flutter/features/workspace/presentation/workspace_metrics.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  // The window_manager plugin resolves through a method channel; widget
  // tests run it against a recording mock so button actions are observable.
  Future<List<MethodCall>> pumpControls(WidgetTester tester) async {
    const channel = MethodChannel('window_manager');
    final calls = <MethodCall>[];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          calls.add(call);
          return call.method == 'isMaximized' ? false : null;
        });
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: Center(child: AtlasWindowControls())),
      ),
    );
    await tester.pumpAndSettle();
    return calls;
  }

  testWidgets('renders minimize, maximize and close controls', (tester) async {
    await pumpControls(tester);
    expect(
      find.descendant(
        of: find.byType(AtlasWindowControls),
        matching: find.byType(WorkspaceToolbarButton),
      ),
      findsNWidgets(3),
    );
  });

  testWidgets('minimize sends the minimize call', (tester) async {
    final calls = await pumpControls(tester);
    await tester.tap(find.byTooltip('Minimize'));
    await tester.pumpAndSettle();
    expect(calls.map((c) => c.method), contains('minimize'));
  });

  testWidgets('maximize toggles based on the restored state', (tester) async {
    const channel = MethodChannel('window_manager');
    final calls = <MethodCall>[];
    void record(MethodCall call) => calls.add(call);
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          record(call);
          return call.method == 'isMaximized' ? false : null;
        });
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: Center(child: AtlasWindowControls())),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Maximize'));
    await tester.pumpAndSettle();
    expect(calls.where((c) => c.method == 'maximize'), isNotEmpty);
    expect(calls.where((c) => c.method == 'unmaximize'), isEmpty);

    calls.clear();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          record(call);
          return call.method == 'isMaximized' ? true : null;
        });
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Maximize'));
    await tester.pumpAndSettle();
    expect(calls.where((c) => c.method == 'unmaximize'), isNotEmpty);
    expect(calls.where((c) => c.method == 'maximize'), isEmpty);
  });

  testWidgets('close sends the close call', (tester) async {
    final calls = await pumpControls(tester);
    await tester.tap(find.byTooltip('Close'));
    await tester.pumpAndSettle();
    expect(calls.map((c) => c.method), contains('close'));
  });

  test('caption controls skip tiling window managers', () {
    // The gate sits behind the desktop-titlebar getter, which reads
    // defaultTargetPlatform; pin a desktop value for the matrix below.
    debugDefaultTargetPlatformOverride = TargetPlatform.macOS;
    // Hyprland/Sway/i3 drive window commands through keybindings, so the
    // toolbar must not reserve a corner for minimize/maximize/close.
    expect(
      usesCaptionControls(desktop: 'Hyprland', sessionType: 'wayland'),
      isFalse,
    );
    expect(
      usesCaptionControls(desktop: 'GNOME:GNOME', sessionType: 'wayland'),
      isTrue,
    );
    expect(usesCaptionControls(desktop: '', sessionType: 'x11'), isTrue);
    debugDefaultTargetPlatformOverride = null;
  });

  testWidgets('maximize button shows the restore glyph while maximized', (
    tester,
  ) async {
    const channel = MethodChannel('window_manager');
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          return call.method == 'isMaximized' ? true : null;
        });
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: Center(child: AtlasWindowControls())),
      ),
    );
    await tester.pumpAndSettle();
    // The maximize button flips to its restore glyph and tooltip.
    expect(find.byTooltip('Restore'), findsOneWidget);
    expect(find.byTooltip('Maximize'), findsNothing);
  });

  test('desktop platforms use the integrated titlebar', () {
    // The full shell test suite pins macOS; this pins the remaining desktop
    // platforms so the getter flip stays observable.
    for (final platform in TargetPlatform.values) {
      debugDefaultTargetPlatformOverride = platform;
      final integrated =
          platform == TargetPlatform.macOS ||
          platform == TargetPlatform.windows ||
          platform == TargetPlatform.linux;
      expect(
        WorkspaceMetrics.usesIntegratedTitlebar,
        integrated,
        reason: '$platform should be $integrated',
      );
    }
    debugDefaultTargetPlatformOverride = null;
  });
}
