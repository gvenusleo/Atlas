import 'dart:io';

import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';

/// Owns the runtime adapters for one CLI invocation.
final class CliRuntimeResources._(
  final DriftSessionStore _store,
  final DioHttpStreamClient _http,
  final ComposedTools _tools,

  /// The single shared runtime used by the command.
  final AgentRuntime runtime,
) {
  /// Creates resources after successful asynchronous initialization.
  this;

  /// Discovers tools and composes a runtime, cleaning up partial startup.
  static Future<CliRuntimeResources> create(
    AtlasConfig config, {
    CancellationToken? cancellation,
  }) async {
    final logger = composeLogger(config);
    final tools = await composeTools(
      config,
      cancellation: cancellation,
      logger: logger,
    );
    DriftSessionStore? store;
    DioHttpStreamClient? http;
    try {
      cancellation?.throwIfCancelled();
      store = DriftSessionStore.openFile(File(config.session.dbPath));
      http = DioHttpStreamClient();
      final runtime = composeRuntime(
        config,
        store: store,
        httpClient: http,
        tools: tools.registry,
        logger: logger,
      );
      return CliRuntimeResources._(store, http, tools, runtime);
    } catch (_) {
      try {
        await tools.close();
      } finally {
        http?.close();
        await store?.close();
      }
      rethrow;
    }
  }

  Future<void>? _closing;

  /// Cancels and drains turns before releasing tools, HTTP and storage.
  Future<void> close() => _closing ??= _close();

  Future<void> _close() async {
    try {
      await runtime.shutdown();
    } finally {
      try {
        await _tools.close();
      } finally {
        _http.close();
        await _store.close();
      }
    }
  }
}
