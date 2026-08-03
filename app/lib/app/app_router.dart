import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../features/workspace/presentation/workspace_page.dart';

/// Provides the application router and disposes it with its provider scope.
final appRouterProvider = Provider<GoRouter>((ref) {
  final router = GoRouter(
    routes: [
      GoRoute(path: '/', builder: (context, state) => const WorkspacePage()),
    ],
  );
  ref.onDispose(router.dispose);
  return router;
});
