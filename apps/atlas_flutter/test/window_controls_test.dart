import 'package:atlas_flutter/shared/layout/atlas_layout_metrics.dart';
import 'package:atlas_flutter/shared/widgets/window_controls.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
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

  test('macOS never shows custom caption controls', () {
    // macOS keeps its native traffic lights; the toolbar must not add a
    // second button group at the top right.
    debugDefaultTargetPlatformOverride = TargetPlatform.macOS;
    expect(
      usesCaptionControls(desktop: 'GNOME:GNOME', sessionType: 'wayland'),
      isFalse,
    );
    expect(usesCaptionControls(desktop: '', sessionType: 'x11'), isFalse);
    debugDefaultTargetPlatformOverride = null;
  });

  test('windows always shows custom caption controls', () {
    // The hidden native title bar leaves no window buttons, so Windows
    // relies on the custom group regardless of desktop environment.
    debugDefaultTargetPlatformOverride = TargetPlatform.windows;
    expect(
      usesCaptionControls(desktop: 'Hyprland', sessionType: 'wayland'),
      isTrue,
    );
    expect(usesCaptionControls(desktop: '', sessionType: ''), isTrue);
    debugDefaultTargetPlatformOverride = null;
  });

  test('linux caption controls skip tiling window managers', () {
    // Hyprland/Sway/i3 drive window commands through keybindings, so the
    // toolbar must not reserve a corner for minimize/maximize/close.
    debugDefaultTargetPlatformOverride = TargetPlatform.linux;
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
        AtlasLayoutMetrics.usesIntegratedTitlebar,
        integrated,
        reason: '$platform should be $integrated',
      );
    }
    debugDefaultTargetPlatformOverride = null;
  });
}
