import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/remote_connection/application/connection_profiles_controller.dart';
import 'package:atlas_flutter/features/remote_connection/data/remote_connections.dart';
import 'package:atlas_flutter/features/remote_connection/presentation/remote_connect_view.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/details_panel.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'workspace_support.dart';

void main() {
  testWidgets('remote sessions hide local files and terminal tools', (
    tester,
  ) async {
    debugDefaultTargetPlatformOverride = TargetPlatform.macOS;
    try {
      await _pumpDetails(tester, isRemote: true);
      expect(find.byTooltip('Files'), findsNothing);
      expect(find.byTooltip('Terminal'), findsNothing);
      expect(find.textContaining('Remote session'), findsOneWidget);
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
  });

  testWidgets('local sessions keep the files and terminal tools', (
    tester,
  ) async {
    debugDefaultTargetPlatformOverride = TargetPlatform.macOS;
    try {
      await _pumpDetails(tester, isRemote: false);
      expect(find.byTooltip('Files'), findsOneWidget);
      expect(find.byTooltip('Terminal'), findsOneWidget);
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
  });

  testWidgets('remote profiles are lazy and reachable in a short window', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 300);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          remoteProfilesProvider.overrideWith(
            () => _SavedProfiles([
              for (var i = 0; i < 100; i++)
                RemoteConnectionProfile(
                  name: 'Computer $i',
                  wsUrl: 'ws://localhost:9000/acp',
                  token: '',
                ),
            ]),
          ),
        ],
        child: const _Host(),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Computer 99'), findsNothing);
    expect(tester.takeException(), isNull);
    await tester.scrollUntilVisible(find.text('Computer 99'), 600);
    expect(find.text('Computer 99'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('remote entry scrolls at landscape and large text sizes', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(600, 200);
    tester.view.devicePixelRatio = 1;
    tester.platformDispatcher.textScaleFactorTestValue = 2;
    addTearDown(tester.view.reset);
    addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          remoteProfilesProvider.overrideWith(() => _SavedProfiles([])),
        ],
        child: const _Host(),
      ),
    );
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    await tester.scrollUntilVisible(find.text('Add connection'), 150);
    expect(find.text('Add connection').hitTestable(), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('mobile without a runtime shows the remote connect view', (
    tester,
  ) async {
    debugDefaultTargetPlatformOverride = TargetPlatform.android;
    try {
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      await tester.pumpWidget(const ProviderScope(child: _Host()));
      await tester.pumpAndSettle();
      expect(
        find.text('Connect to the Atlas on your computer'),
        findsOneWidget,
      );
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
  });
}

/// Minimal host rendering the details panel inside the workspace plumbing.
Future<void> _pumpDetails(WidgetTester tester, {required bool isRemote}) async {
  final model = ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('local')),
  );
  final store = DriftSessionStore.inMemory();
  addTearDown(store.close);
  final runtime = AgentRuntime(
    store: store,
    provider: FakeProvider(model.ref),
    tools: LocalToolRegistry(const []),
    ids: SecureIdGenerator(),
    defaultModel: model.ref,
  );
  final environment = RuntimeEnvironment(
    runtime: runtime,
    models: [model],
    isRemote: isRemote,
  );
  final container = ProviderContainer(
    overrides: [
      runtimeEnvironmentProvider.overrideWith(
        () => RuntimeEnvironmentController(local: environment),
      ),
      workspaceWorkingDirectoryProvider.overrideWith(
        () => FixedWorkingDirectory('/tmp'),
      ),
    ],
  );
  addTearDown(container.dispose);
  await tester.pumpWidget(
    UncontrolledProviderScope(
      container: container,
      child: MaterialApp(
        theme: buildAtlasTheme(AtlasPalette.standard, Brightness.light),
        home: Scaffold(
          body: Row(
            children: [
              const Expanded(child: SizedBox()),
              SizedBox(width: 300, child: DetailsPanel()),
            ],
          ),
        ),
      ),
    ),
  );
  // The local file browser performs real directory IO and keeps a loading
  // spinner running; settle is not applicable here.
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 400));
}

class const _Host() extends StatelessWidget {
  @override
  Widget build(BuildContext context) =>
      const MaterialApp(home: Scaffold(body: RemoteConnectView()));
}

class _SavedProfiles(final List<RemoteConnectionProfile> profiles)
    extends RemoteProfilesController {
  @override
  Future<List<RemoteConnectionProfile>> build() async => profiles;
}
