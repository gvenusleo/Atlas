import 'dart:io';

import 'package:atlas_flutter/app/app_router.dart';
import 'package:atlas_flutter/app/atlas_app.dart';
import 'package:atlas_flutter/app/locale_mode.dart';
import 'package:atlas_flutter/app/theme_mode.dart';
import 'package:atlas_flutter/features/workspace/presentation/settings_page.dart';
import 'package:atlas_flutter/features/workspace/presentation/workspace_page.dart';
import 'package:atlas_flutter/features/workspace/presentation/workspace_metrics.dart';
import 'package:atlas_flutter/features/workspace/presentation/workspace_shell.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:flutter/foundation.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:window_manager/window_manager.dart';

void main() {
  testWidgets('a cold settings deep link returns to the workspace', (
    tester,
  ) async {
    tester.binding.platformDispatcher.defaultRouteNameTestValue =
        'atlas:///settings';
    addTearDown(
      tester.binding.platformDispatcher.clearDefaultRouteNameTestValue,
    );
    await tester.pumpWidget(const ProviderScope(child: AtlasApp()));
    await tester.pumpAndSettle();
    expect(find.byType(SettingsPage), findsOneWidget);
    await tester.tap(find.byKey(const ValueKey('atlas-settings-back')));
    await tester.pumpAndSettle();
    expect(find.byType(WorkspacePage), findsOneWidget);
    expect(tester.takeException(), isNull);

    // A platform URL delivered to the running app uses the same route tree.
    await tester.binding.defaultBinaryMessenger.handlePlatformMessage(
      'flutter/navigation',
      const JSONMethodCodec().encodeMethodCall(
        const MethodCall('pushRouteInformation', {
          'location': 'atlas:///settings',
        }),
      ),
      (_) {},
    );
    await tester.pumpAndSettle();
    expect(find.byType(SettingsPage), findsOneWidget);
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(find.byType(WorkspacePage), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testShell(
    'application follows the system brightness',
    const Size(1200, 760),
    (tester) async {
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
        expect(center.color, AtlasPalette.standard.light.canvas);
        final leftPanel = tester.widget<ColoredBox>(
          find
              .descendant(
                of: find.byKey(const ValueKey('atlas-left-panel')),
                matching: find.byType(ColoredBox),
              )
              .first,
        );
        expect(leftPanel.color, AtlasPalette.standard.light.panel);
        expect(
          AtlasColors.of(tester.element(find.text('New session'))),
          same(AtlasPalette.standard.light),
        );
        expect(_overlayStyleOf(tester), SystemUiOverlayStyle.dark);

        tester.platformDispatcher.platformBrightnessTestValue = Brightness.dark;
        await tester.pumpAndSettle();

        center = tester.widget<ColoredBox>(
          find.byKey(const ValueKey('atlas-center-panel')),
        );
        expect(center.color, AtlasPalette.standard.dark.canvas);
        expect(
          AtlasColors.of(tester.element(find.text('New session'))),
          same(AtlasPalette.standard.dark),
        );
        expect(_overlayStyleOf(tester), SystemUiOverlayStyle.light);
        if (Platform.isMacOS || Platform.isWindows || Platform.isLinux) {
          expect(calls.last.method, 'setBackgroundColor');
          final canvas = AtlasPalette.standard.dark.canvas;
          expect(calls.last.arguments, {
            'backgroundColorA': (canvas.a * 255).round(),
            'backgroundColorR': (canvas.r * 255).round(),
            'backgroundColorG': (canvas.g * 255).round(),
            'backgroundColorB': (canvas.b * 255).round(),
          });
        }
      } finally {
        tester.platformDispatcher.clearPlatformBrightnessTestValue();
        await tester.pump();
        messenger.setMockMethodCallHandler(channel, null);
      }
    },
  );

  testShell(
    'desktop starts with three correctly sized panels',
    const Size(1200, 760),
    (tester) async {
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
      expect(find.text('Sessions'), findsNothing);
      expect(find.text('Details'), findsNothing);
      expect(find.byType(WorkspacePage), findsOneWidget);
      expect(find.byType(WorkspaceShell), findsOneWidget);

      for (final key in const ['atlas-left-toggle', 'atlas-right-toggle']) {
        final button = find.descendant(
          of: find.byKey(ValueKey(key)),
          matching: find.byType(AnimatedContainer),
        );
        expect(
          tester.getSize(button),
          Size.square(WorkspaceMetrics.desktopToolbarButtonSize),
        );
      }
      expect(
        tester.getCenter(find.byKey(const ValueKey('atlas-left-toggle'))).dx,
        lessThan(tester.getTopRight(left).dx),
      );
      expect(
        tester.getCenter(find.byKey(const ValueKey('atlas-right-toggle'))).dx,
        greaterThan(tester.getTopLeft(right).dx),
      );
    },
  );

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

  testShell(
    'desktop side panels animate their occupied width',
    const Size(1200, 760),
    (tester) async {
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
    },
  );

  testShell(
    'desktop keeps sidebar toggles fixed during animations',
    const Size(1200, 760),
    (tester) async {
      final leftToggle = find.byKey(const ValueKey('atlas-left-toggle'));
      final rightToggle = find.byKey(const ValueKey('atlas-right-toggle'));
      final leftState = tester.state(leftToggle);
      final rightState = tester.state(rightToggle);
      final leftX = tester.getTopLeft(leftToggle).dx;
      final rightX = tester.getTopLeft(rightToggle).dx;

      await tester.tap(leftToggle);
      await tester.pump();
      expect(leftToggle, findsOneWidget);
      expect(rightToggle, findsOneWidget);
      expect(tester.state(leftToggle), same(leftState));

      await tester.pump(const Duration(milliseconds: 90));
      expect(tester.getTopLeft(leftToggle).dx, leftX);
      expect(tester.getTopLeft(rightToggle).dx, rightX);
      expect(leftToggle, findsOneWidget);
      expect(rightToggle, findsOneWidget);

      await tester.pumpAndSettle();
      expect(tester.getTopLeft(leftToggle).dx, leftX);
      expect(tester.state(leftToggle), same(leftState));

      await tester.tap(leftToggle);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 60));
      expect(tester.getTopLeft(leftToggle).dx, leftX);

      await tester.tap(leftToggle);
      await tester.pump();
      expect(tester.getTopLeft(leftToggle).dx, leftX);
      await tester.pump(const Duration(milliseconds: 60));
      expect(tester.getTopLeft(leftToggle).dx, leftX);
      expect(leftToggle, findsOneWidget);
      await tester.pumpAndSettle();

      await tester.tap(rightToggle);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 90));
      expect(tester.getTopLeft(rightToggle).dx, rightX);
      expect(tester.state(rightToggle), same(rightState));
      expect(leftToggle, findsOneWidget);
      expect(rightToggle, findsOneWidget);
      await tester.pumpAndSettle();
      expect(tester.getTopLeft(rightToggle).dx, rightX);
    },
  );

  testShell(
    'desktop sidebar toggles respect reduced motion',
    const Size(1200, 760),
    (tester) async {
      final center = find.byKey(const ValueKey('atlas-center-panel'));
      final initialCenterWidth = tester.getSize(center).width;

      await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
      await tester.pump();

      expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
      expect(
        tester.getSize(center).width,
        closeTo(initialCenterWidth + 232, 0.01),
      );
      expect(find.byKey(const ValueKey('atlas-left-toggle')), findsOneWidget);
    },
    disableAnimations: true,
  );

  testShell(
    'macOS titlebar keeps controls clear of the traffic lights',
    const Size(1200, 760),
    (tester) async {
      expect(find.byType(DragToMoveArea), findsNWidgets(3));
      expect(
        tester.getTopLeft(find.byKey(const ValueKey('atlas-left-toggle'))).dx,
        WorkspaceMetrics.macOSTrafficLightInset,
      );

      await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
      final leftToggle = find.byKey(const ValueKey('atlas-left-toggle'));
      expect(
        tester.getTopLeft(leftToggle).dx,
        WorkspaceMetrics.macOSTrafficLightInset,
      );
    },
  );

  testShell(
    'desktop resize handles adjust and clamp panel widths',
    const Size(1280, 760),
    (tester) async {
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
    },
  );

  testShell(
    'desktop resize handle backgrounds fill the hit area',
    const Size(1200, 760),
    (tester) async {
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
    },
  );

  testShell(
    'desktop header divider crosses both resize handles',
    const Size(1200, 760),
    (tester) async {
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
    },
  );

  testShell(
    'compact layout opens sessions and details as drawers',
    const Size(390, 844),
    (tester) async {
      expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
      expect(find.byKey(const ValueKey('atlas-right-panel')), findsNothing);
      expect(find.byTooltip('Open sessions'), findsOneWidget);
      expect(find.byTooltip('Open workspace tools'), findsOneWidget);

      for (final key in const ['atlas-left-toggle', 'atlas-right-toggle']) {
        final button = find.descendant(
          of: find.byKey(ValueKey(key)),
          matching: find.byType(AnimatedContainer),
        );
        expect(tester.getSize(button), const Size.square(44));
      }

      await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
      await tester.pumpAndSettle();
      expect(find.text('Runtime unavailable'), findsOneWidget);
      expect(find.byTooltip('Close sessions'), findsOneWidget);
      await tester.tap(find.byTooltip('Close sessions'));
      await tester.pumpAndSettle();

      await tester.tap(find.byKey(const ValueKey('atlas-right-toggle')));
      await tester.pumpAndSettle();
      expect(find.text('Runtime unavailable'), findsOneWidget);
      expect(find.byTooltip('Close workspace tools'), findsOneWidget);
    },
  );

  for (final platform in [TargetPlatform.android, TargetPlatform.iOS]) {
    testShell(
      'wide $platform windows use side panels and adapt when resized',
      const Size(1200, 760),
      platform: platform,
      (tester) async {
        expect(find.byKey(const ValueKey('atlas-left-panel')), findsOneWidget);
        expect(find.byKey(const ValueKey('atlas-right-panel')), findsOneWidget);
        final toggle = find.byKey(const ValueKey('atlas-left-toggle'));
        expect(tester.getSize(toggle).shortestSide, greaterThanOrEqualTo(44));

        tester.view.physicalSize = const Size(600, 760);
        await tester.pumpAndSettle();
        expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
        await tester.tap(find.byTooltip('Open sessions'));
        await tester.pumpAndSettle();
        expect(find.byTooltip('Close sessions'), findsOneWidget);
        await tester.tap(find.byTooltip('Close sessions'));
        await tester.pumpAndSettle();

        tester.view.physicalSize = const Size(1200, 760);
        await tester.pumpAndSettle();
        expect(find.byKey(const ValueKey('atlas-left-panel')), findsOneWidget);
        expect(tester.takeException(), isNull);
      },
    );
  }

  for (final size in [const Size(390, 844), const Size(1200, 760)]) {
    testShell('direct settings entry at $size returns to the workspace', size, (
      tester,
    ) async {
      final container = ProviderScope.containerOf(
        tester.element(find.byType(AtlasApp)),
      );
      final router = container.read(appRouterProvider);
      router.go('/settings');
      await tester.pumpAndSettle();
      expect(find.byType(SettingsPage), findsOneWidget);
      expect(router.canPop(), isTrue);
      await tester.tap(find.byKey(const ValueKey('atlas-settings-back')));
      await tester.pumpAndSettle();
      expect(router.routeInformationProvider.value.uri.path, '/');
      expect(find.byType(WorkspacePage), findsOneWidget);
      expect(tester.takeException(), isNull);

      router.go('/settings');
      await tester.pumpAndSettle();
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      expect(router.routeInformationProvider.value.uri.path, '/');
      expect(tester.takeException(), isNull);
    });
  }

  testShell(
    'desktop visibility survives compact layout transitions',
    const Size(1200, 760),
    (tester) async {
      await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);

      tester.view.physicalSize = const Size(390, 844);
      await tester.pumpAndSettle();
      expect(find.byTooltip('Open sessions'), findsOneWidget);
      await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
      await tester.pumpAndSettle();
      expect(find.text('Sessions'), findsNothing);
      await tester.tap(find.byTooltip('Close sessions'));
      await tester.pumpAndSettle();

      tester.view.physicalSize = const Size(1200, 760);
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('atlas-left-panel')), findsNothing);
      expect(find.byTooltip('Show sessions'), findsOneWidget);
    },
  );

  testShell(
    'a seeded dark preference overrides a light platform',
    const Size(1200, 760),
    child: ProviderScope(
      overrides: [
        themeModeProvider.overrideWith(
          () => ThemeModeController(initial: ThemeMode.dark),
        ),
      ],
      child: const AtlasApp(),
    ),
    (tester) async {
      // Widget tests report a light platform; the stored preference wins.
      final center = tester.widget<ColoredBox>(
        find.byKey(const ValueKey('atlas-center-panel')),
      );
      expect(center.color, AtlasPalette.standard.dark.canvas);
      expect(
        AtlasColors.of(tester.element(find.text('New session'))),
        same(AtlasPalette.standard.dark),
      );
      expect(_overlayStyleOf(tester), SystemUiOverlayStyle.light);
    },
  );

  testShell(
    'the settings page switches the running theme',
    const Size(1200, 760),
    (tester) async {
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();
      expect(find.byType(SettingsPage), findsOneWidget);

      await tester.tap(find.text('Dark'));
      await tester.pumpAndSettle();

      expect(
        Theme.of(tester.element(find.byType(SettingsPage))).brightness,
        Brightness.dark,
      );
      expect(
        tester
            .widget<ColoredBox>(
              find.byKey(
                const ValueKey('atlas-center-panel'),
                skipOffstage: false,
              ),
            )
            .color,
        AtlasPalette.standard.dark.canvas,
      );

      await tester.tap(find.byKey(const ValueKey('atlas-settings-back')));
      await tester.pumpAndSettle();
      expect(find.byType(SettingsPage), findsNothing);
      expect(find.byType(WorkspacePage), findsOneWidget);
    },
  );

  testShell(
    'the settings page switches the running language',
    const Size(1200, 760),
    child: ProviderScope(
      overrides: [
        languageProvider.overrideWith(
          () => LanguageController(initial: AppLanguage.english),
        ),
      ],
      child: const AtlasApp(),
    ),
    (tester) async {
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();
      expect(find.text('Language'), findsOneWidget);

      final themeSelector = find.byType(SegmentedButton<ThemeMode>);
      final languageSelector = find.byType(SegmentedButton<AppLanguage>);
      expect(themeSelector, findsOneWidget);
      expect(languageSelector, findsOneWidget);
      expect(
        tester.getTopRight(themeSelector).dx,
        tester.getTopRight(languageSelector).dx,
      );

      await tester.tap(find.text('简体中文'));
      await tester.pumpAndSettle();

      expect(find.text('设置'), findsOneWidget);
      expect(find.text('主题'), findsOneWidget);
      expect(find.byKey(const ValueKey('atlas-settings-back')), findsOneWidget);

      await tester.tap(find.text('English'));
      await tester.pumpAndSettle();

      expect(find.text('Settings'), findsOneWidget);
      expect(find.text('Theme'), findsOneWidget);
    },
  );

  testShell(
    'the settings page is a section rail beside a content pane',
    const Size(1200, 760),
    (tester) async {
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();

      // The rail uses the sessions sidebar surface, with navigation at its foot.
      final rail = find.byKey(const ValueKey('atlas-settings-rail'));
      expect(tester.getTopLeft(rail).dx, 0);
      expect(tester.getTopLeft(rail).dy, 0);
      expect(tester.getSize(rail).width, SettingsPage.railWidth);
      final back = find.byKey(const ValueKey('atlas-settings-back'));
      expect(find.descendant(of: rail, matching: back), findsOneWidget);
      expect(tester.getTopLeft(back).dx, 10);
      expect(tester.getBottomLeft(back).dy, greaterThan(700));
      expect(
        tester.getTopLeft(back).dy,
        greaterThan(
          tester
              .getBottomLeft(
                find.byKey(const ValueKey('atlas-settings-rail-connections')),
              )
              .dy,
        ),
      );
      expect(
        tester
            .widget<ColoredBox>(
              find
                  .descendant(of: rail, matching: find.byType(ColoredBox))
                  .first,
            )
            .color,
        AtlasPalette.standard.light.panel,
      );

      // The content column is centered in the pane, while its headings share
      // the same left edge as the theme choices and setting rows.
      final pane = find.byKey(const ValueKey('atlas-settings-pane'));
      const contentLeft =
          SettingsPage.railWidth +
          1 +
          (1200 - SettingsPage.railWidth - 1 - SettingsPage.columnWidth) / 2;
      expect(tester.getTopLeft(pane).dx, SettingsPage.railWidth + 1);
      expect(
        tester
            .getTopLeft(
              find.descendant(of: pane, matching: find.text('Appearance')),
            )
            .dx,
        contentLeft,
      );
      expect(tester.getTopLeft(find.text('Theme')).dx, contentLeft);

      final themeSelector = find.byType(SegmentedButton<ThemeMode>);
      final languageSelector = find.byType(SegmentedButton<AppLanguage>);
      expect(
        tester.getTopRight(themeSelector).dx,
        contentLeft + SettingsPage.columnWidth,
      );
      expect(
        tester.getTopRight(languageSelector).dx,
        tester.getTopRight(themeSelector).dx,
      );
      expect(tester.getTopLeft(find.text('Language')).dx, contentLeft);
      expect(
        find.byKey(const ValueKey('atlas-settings-rail-toggle')),
        findsOneWidget,
      );
      final backButton = tester.widget<TextButton>(back);
      expect(
        backButton.style!.overlayColor!.resolve({WidgetState.hovered}),
        Colors.transparent,
      );
    },
  );

  testShell(
    'settings rail toggles without losing the selected section',
    const Size(1200, 760),
    (tester) async {
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();
      await tester.tap(
        find.byKey(const ValueKey('atlas-settings-rail-connections')),
      );
      await tester.pumpAndSettle();

      final toggle = find.byKey(const ValueKey('atlas-settings-rail-toggle'));
      expect(
        tester.getTopLeft(toggle).dx,
        WorkspaceMetrics.macOSTrafficLightInset,
      );
      await tester.tap(toggle);
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('atlas-settings-rail')), findsNothing);
      expect(
        tester.getTopLeft(find.byKey(const ValueKey('atlas-settings-pane'))).dx,
        0,
      );
      expect(find.text('Remote connections'), findsOneWidget);
      expect(
        find.byKey(const ValueKey('atlas-settings-strip-appearance')),
        findsNothing,
      );
      expect(find.byKey(const ValueKey('atlas-settings-back')), findsOneWidget);

      await tester.tap(toggle);
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('atlas-settings-rail')), findsOneWidget);
      expect(find.text('Remote connections'), findsOneWidget);
      await tester.tap(
        find.byKey(const ValueKey('atlas-settings-rail-appearance')),
      );
      await tester.pumpAndSettle();
      expect(find.byType(SegmentedButton<ThemeMode>), findsOneWidget);
    },
  );

  testShell(
    'settings can return while its rail is collapsed',
    const Size(1200, 760),
    (tester) async {
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();
      await tester.tap(
        find.byKey(const ValueKey('atlas-settings-rail-toggle')),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('atlas-settings-back')));
      await tester.pumpAndSettle();
      expect(find.byType(SettingsPage), findsNothing);
      expect(find.byType(WorkspacePage), findsOneWidget);
    },
  );

  testShell(
    'the settings page stacks the selector on narrow windows',
    const Size(390, 844),
    platform: TargetPlatform.android,
    (tester) async {
      // Compact layouts keep the session panel in a drawer.
      await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();

      // Compact layouts drop the rail and keep the pane only.
      expect(find.byKey(const ValueKey('atlas-settings-rail')), findsNothing);

      final selector = find.byType(SegmentedButton<ThemeMode>);
      expect(
        tester.getTopLeft(selector).dy,
        greaterThan(tester.getBottomLeft(find.text('Theme')).dy),
      );
      expect(
        tester.getTopLeft(selector).dx,
        tester.getTopLeft(find.text('Theme')).dx,
      );
      expect(tester.getTopRight(selector).dx, lessThanOrEqualTo(390));
      final languageSelector = find.byType(SegmentedButton<AppLanguage>);
      expect(
        tester.getTopLeft(languageSelector).dy,
        greaterThan(tester.getBottomLeft(find.text('Language')).dy),
      );
      expect(
        tester.getTopLeft(languageSelector).dx,
        tester.getTopLeft(find.text('Language')).dx,
      );

      // The rail is gone, so the pane carries the section strip instead.
      await tester.tap(
        find.byKey(const ValueKey('atlas-settings-strip-connections')),
      );
      await tester.pumpAndSettle();
      expect(find.text('Theme'), findsNothing);
      expect(find.text('Add connection'), findsOneWidget);
    },
  );

  testShell(
    'settings theme selector fits a 375px screen',
    const Size(375, 760),
    platform: TargetPlatform.android,
    (tester) async {
      await tester.tap(find.byKey(const ValueKey('atlas-left-toggle')));
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();

      final selector = find.byType(SegmentedButton<ThemeMode>);
      expect(tester.getTopRight(selector).dx, lessThanOrEqualTo(375));
      expect(tester.takeException(), isNull);
    },
  );

  testShell(
    'settings content is centered on wide desktop windows',
    const Size(1567, 1080),
    (tester) async {
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();

      final pane = find.byKey(const ValueKey('atlas-settings-pane'));
      final left = tester.getTopLeft(find.text('Appearance').last).dx;
      final paneCenter = tester.getCenter(pane).dx;
      expect(left + SettingsPage.columnWidth / 2, closeTo(paneCenter, 0.01));
      expect(tester.getTopLeft(find.text('Theme')).dx, left);
      expect(tester.takeException(), isNull);
    },
  );

  testShell(
    'the rail switches to the ACP connections section',
    const Size(1200, 760),
    (tester) async {
      await tester.tap(find.byTooltip('Settings'));
      await tester.pumpAndSettle();
      expect(find.text('Theme'), findsOneWidget);

      await tester.tap(
        find.byKey(const ValueKey('atlas-settings-rail-connections')),
      );
      await tester.pumpAndSettle();

      // The appearance rows give way to the connection manager in the same
      // pane, so every settings surface lives on one page.
      expect(find.text('Theme'), findsNothing);
      expect(find.text('Add connection'), findsOneWidget);
      expect(find.text('Remote connections'), findsOneWidget);

      await tester.tap(
        find.byKey(const ValueKey('atlas-settings-rail-appearance')),
      );
      await tester.pumpAndSettle();
      expect(find.text('Theme'), findsOneWidget);
      expect(find.text('Add connection'), findsNothing);
    },
  );
}

/// Pumps the workspace shell at [size] on [platform] and restores the
/// platform override before the framework verifies test invariants.
void testShell(
  String description,
  Size size,
  Future<void> Function(WidgetTester tester) body, {
  TargetPlatform platform = TargetPlatform.macOS,
  bool disableAnimations = false,
  Widget? child,
}) {
  // Widget tests default to Android; desktop scenarios pin a desktop platform.
  testWidgets(description, (tester) async {
    debugDefaultTargetPlatformOverride = platform;
    try {
      if (disableAnimations) {
        tester.platformDispatcher.accessibilityFeaturesTestValue =
            const FakeAccessibilityFeatures(disableAnimations: true);
      }
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = size;
      addTearDown(tester.view.reset);
      await tester.pumpWidget(child ?? const ProviderScope(child: AtlasApp()));
      await tester.pumpAndSettle();
      await body(tester);
    } finally {
      tester.platformDispatcher.clearAccessibilityFeaturesTestValue();
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
