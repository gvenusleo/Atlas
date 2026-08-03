import 'dart:io';

import 'package:atlas_app/app/app_router.dart';
import 'package:atlas_app/app/atlas_app.dart';
import 'package:atlas_app/features/workspace/presentation/workspace_page.dart';
import 'package:atlas_app/features/workspace/presentation/workspace_shell.dart';
import 'package:atlas_app/shared/theme/atlas_theme.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:window_manager/window_manager.dart';

void main() {
  test('router starts at the workspace route', () {
    final container = ProviderContainer();
    addTearDown(container.dispose);

    final router = container.read(appRouterProvider);

    expect(router.routeInformationProvider.value.uri.path, '/');
  });

  test('light and dark themes expose their semantic palettes', () {
    final light = buildAtlasTheme(Brightness.light);
    final dark = buildAtlasTheme(Brightness.dark);

    expect(light.brightness, Brightness.light);
    expect(light.extension<AtlasColors>(), same(AtlasColors.light));
    expect(light.scaffoldBackgroundColor, AtlasColors.light.canvas);
    expect(dark.brightness, Brightness.dark);
    expect(dark.extension<AtlasColors>(), same(AtlasColors.dark));
    expect(dark.scaffoldBackgroundColor, AtlasColors.dark.canvas);
  });

  testShell('application follows the system brightness', const Size(1200, 760), (
    tester,
  ) async {
    const channel = MethodChannel('window_manager');
    final calls = <MethodCall>[];
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      return null;
    });
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.light;

    try {
      await tester.pumpAndSettle();
      var center = tester.widget<ColoredBox>(
        find.byKey(const ValueKey('atlas-center-panel')),
      );
      expect(center.color, AtlasColors.light.canvas);
      final leftPanel = tester.widget<ColoredBox>(
        find
            .descendant(
              of: find.byKey(const ValueKey('atlas-left-panel')),
              matching: find.byType(ColoredBox),
            )
            .first,
      );
      expect(leftPanel.color, AtlasColors.light.panel);
      expect(
        AtlasColors.of(tester.element(find.text('New session'))),
        same(AtlasColors.light),
      );
      expect(_overlayStyleOf(tester), SystemUiOverlayStyle.dark);

      tester.platformDispatcher.platformBrightnessTestValue = Brightness.dark;
      await tester.pumpAndSettle();

      center = tester.widget<ColoredBox>(
        find.byKey(const ValueKey('atlas-center-panel')),
      );
      expect(center.color, AtlasColors.dark.canvas);
      expect(
        AtlasColors.of(tester.element(find.text('New session'))),
        same(AtlasColors.dark),
      );
      expect(_overlayStyleOf(tester), SystemUiOverlayStyle.light);
      if (Platform.isMacOS || Platform.isWindows || Platform.isLinux) {
        expect(calls.last.method, 'setBackgroundColor');
        expect(calls.last.arguments, {
          'backgroundColorA': 255,
          'backgroundColorR': 32,
          'backgroundColorG': 35,
          'backgroundColorB': 42,
        });
      }
    } finally {
      tester.platformDispatcher.clearPlatformBrightnessTestValue();
      await tester.pump();
      messenger.setMockMethodCallHandler(channel, null);
    }
  });

  testShell('desktop starts with three correctly sized panels', const Size(1200, 760), (
    tester,
  ) async {
    final left = find.byKey(const ValueKey('atlas-left-panel'));
    final center = find.byKey(const ValueKey('atlas-center-panel'));
    final right = find.byKey(const ValueKey('atlas-right-panel'));
    expect(left, findsOneWidget);
    expect(center, findsOneWidget);
    expect(right, findsOneWidget);
    expect(tester.getSize(left).width, 224);
    expect(tester.getSize(right).width, 260);
    expect(tester.getSize(center).width, greaterThan(680));
    expect(find.text('New session'), findsOneWidget);
    expect(find.byType(WorkspacePage), findsOneWidget);
    expect(find.byType(WorkspaceShell), findsOneWidget);

    for (final key in const ['atlas-left-toggle', 'atlas-right-toggle']) {
      final button = find.descendant(
        of: find.byKey(ValueKey(key)),
        matching: find.byType(AnimatedContainer),
      );
      expect(tester.getSize(button), const Size.square(40));
    }
    expect(
      tester.getCenter(find.byKey(const ValueKey('atlas-left-toggle'))).dx,
      lessThan(tester.getTopRight(left).dx),
    );
    expect(
      tester.getCenter(find.byKey(const ValueKey('atlas-right-toggle'))).dx,
      greaterThan(tester.getTopLeft(right).dx),
    );
  });

  testShell('desktop side panels toggle independently', const Size(1200, 760), (
    tester,
  ) async {
    final center = find.byKey(const ValueKey('atlas-center-panel'));
    final initialCenterWidth = tester.getSize(center).width;

    await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
    expect(find.byKey(const ValueKey('atlas-right-panel')), findsOneWidget);
    expect(tester.getSize(center).width, greaterThan(initialCenterWidth));
    expect(find.byTooltip('Show sessions'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('atlas-right-toggle')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('atlas-right-panel')), findsNothing);
    expect(tester.getSize(center).width, 1200);

    await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('atlas-right-toggle')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('atlas-left-panel')), findsOneWidget);
    expect(find.byKey(const ValueKey('atlas-right-panel')), findsOneWidget);
  });

  testShell('desktop side panels animate their occupied width', const Size(1200, 760), (
    tester,
  ) async {
    final center = find.byKey(const ValueKey('atlas-center-panel'));
    final initialWidth = tester.getSize(center).width;

    await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 90));
    expect(tester.getSize(center).width, greaterThan(initialWidth));
    expect(tester.getSize(center).width, lessThan(initialWidth + 232));
    await tester.pumpAndSettle();
    expect(tester.getSize(center).width, closeTo(initialWidth + 232, 0.01));

    await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 90));
    expect(tester.getSize(center).width, greaterThan(initialWidth));
    expect(tester.getSize(center).width, lessThan(initialWidth + 232));
    await tester.pumpAndSettle();
    expect(tester.getSize(center).width, closeTo(initialWidth, 0.01));

    await tester.tap(find.byKey(const ValueKey('atlas-right-toggle')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 90));
    expect(tester.getSize(center).width, greaterThan(initialWidth));
    expect(tester.getSize(center).width, lessThan(initialWidth + 268));
    await tester.pumpAndSettle();
    expect(tester.getSize(center).width, closeTo(initialWidth + 268, 0.01));
  });

  testShell('macOS titlebar keeps controls clear of the traffic lights', const Size(1200, 760), (
    tester,
  ) async {
    expect(find.byType(DragToMoveArea), findsNWidgets(3));
    expect(
      tester.getTopLeft(find.text('Sessions')).dx,
      greaterThanOrEqualTo(88),
    );

    await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
    final leftToggle = find.byKey(const ValueKey('atlas-left-toggle'));
    expect(tester.getTopLeft(leftToggle).dx, greaterThanOrEqualTo(76));
  });

  testShell('desktop resize handles adjust and clamp panel widths', const Size(1280, 760), (
    tester,
  ) async {
    final left = find.byKey(const ValueKey('atlas-left-panel'));
    final right = find.byKey(const ValueKey('atlas-right-panel'));

    await tester.drag(
      find.byKey(const ValueKey('atlas-left-resize-handle')),
      const Offset(80, 0),
    );
    await tester.pumpAndSettle();
    expect(tester.getSize(left).width, closeTo(304, 0.01));

    await tester.drag(
      find.byKey(const ValueKey('atlas-right-resize-handle')),
      const Offset(-70, 0),
    );
    await tester.pumpAndSettle();
    expect(tester.getSize(right).width, closeTo(330, 0.01));

    await tester.drag(
      find.byKey(const ValueKey('atlas-left-resize-handle')),
      const Offset(-1000, 0),
    );
    await tester.drag(
      find.byKey(const ValueKey('atlas-right-resize-handle')),
      const Offset(1000, 0),
    );
    await tester.pumpAndSettle();
    expect(tester.getSize(left).width, closeTo(184, 0.01));
    expect(tester.getSize(right).width, closeTo(220, 0.01));
  });

  testShell('desktop resize handle backgrounds fill the hit area', const Size(1200, 760), (
    tester,
  ) async {
    for (final key in const [
      'atlas-left-resize-handle',
      'atlas-right-resize-handle',
    ]) {
      final handle = find.byKey(ValueKey(key));
      final backgrounds = find.descendant(
        of: handle,
        matching: find.byType(ColoredBox),
      );
      final handleSize = tester.getSize(handle);

      expect(backgrounds, findsNWidgets(2));
      for (final background in backgrounds.evaluate()) {
        expect(
          tester.getSize(find.byWidget(background.widget)).height,
          handleSize.height,
        );
      }
    }
  });

  testShell('desktop header divider crosses both resize handles', const Size(1200, 760), (
    tester,
  ) async {
    final centerDivider = find
        .descendant(
          of: find.byKey(const ValueKey('atlas-center-panel')),
          matching: find.byType(Divider),
        )
        .first;
    final expectedTop = tester.getTopLeft(centerDivider).dy;

    for (final key in const [
      'atlas-left-resize-handle',
      'atlas-right-resize-handle',
    ]) {
      final handle = find.byKey(ValueKey(key));
      final divider = find.descendant(
        of: handle,
        matching: find.byKey(const ValueKey('atlas-resize-header-divider')),
      );

      expect(divider, findsOneWidget);
      expect(tester.getSize(divider), const Size(8, 1));
      expect(tester.getTopLeft(divider).dy, expectedTop);
    }
  });

  testShell('compact layout opens sessions and details as drawers', const Size(390, 844), (
    tester,
  ) async {
    expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
    expect(find.byKey(const ValueKey('atlas-right-panel')), findsNothing);
    expect(find.byTooltip('Open sessions'), findsOneWidget);
    expect(find.byTooltip('Open details'), findsOneWidget);

    for (final key in const ['atlas-left-toggle', 'atlas-right-toggle']) {
      final button = find.descendant(
        of: find.byKey(ValueKey(key)),
        matching: find.byType(AnimatedContainer),
      );
      expect(tester.getSize(button), const Size.square(44));
    }

    await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
    await tester.pumpAndSettle();
    expect(find.text('Sessions'), findsOneWidget);
    expect(find.text('No sessions yet'), findsOneWidget);
    expect(find.byTooltip('Close sessions'), findsOneWidget);
    await tester.tap(find.byTooltip('Close sessions'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('atlas-right-toggle')));
    await tester.pumpAndSettle();
    expect(find.text('Details'), findsOneWidget);
    expect(find.text('No active session'), findsOneWidget);
    expect(find.byTooltip('Close details'), findsOneWidget);
  });

  testShell(
    'mobile platforms always use the compact layout',
    const Size(1200, 760),
    platform: TargetPlatform.android,
    (tester) async {
      expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
      expect(find.byKey(const ValueKey('atlas-right-panel')), findsNothing);
      expect(find.byTooltip('Open sessions'), findsOneWidget);
      expect(find.byTooltip('Open details'), findsOneWidget);

      await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
      await tester.pumpAndSettle();
      expect(find.text('Sessions'), findsOneWidget);
      expect(find.byTooltip('Close sessions'), findsOneWidget);
    },
  );

  testShell('desktop visibility survives compact layout transitions', const Size(1200, 760), (
    tester,
  ) async {
    await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);

    tester.view.physicalSize = const Size(390, 844);
    await tester.pumpAndSettle();
    expect(find.byTooltip('Open sessions'), findsOneWidget);
    await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
    await tester.pumpAndSettle();
    expect(find.text('Sessions'), findsOneWidget);
    Navigator.of(tester.element(find.text('Sessions'))).pop();
    await tester.pumpAndSettle();

    tester.view.physicalSize = const Size(1200, 760);
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
    expect(find.byTooltip('Show sessions'), findsOneWidget);
  });
}

/// Pumps the workspace shell at [size] on [platform] and restores the
/// platform override before the framework verifies test invariants.
void testShell(
  String description,
  Size size,
  Future<void> Function(WidgetTester tester) body, {
  TargetPlatform platform = TargetPlatform.macOS,
}) {
  // Widget tests default to Android; desktop scenarios pin a desktop platform.
  testWidgets(description, (tester) async {
    debugDefaultTargetPlatformOverride = platform;
    try {
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = size;
      addTearDown(tester.view.reset);
      await tester.pumpWidget(const ProviderScope(child: AtlasApp()));
      await tester.pumpAndSettle();
      await body(tester);
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
  });
}

SystemUiOverlayStyle _overlayStyleOf(WidgetTester tester) {
  return tester
      .widget<AnnotatedRegion<SystemUiOverlayStyle>>(
        find.byType(AnnotatedRegion<SystemUiOverlayStyle>),
      )
      .value;
}
