import 'dart:io';

import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';

/// Owns the runtime adapters for one CLI invocation.
final class CliRuntimeResources(AtlasConfig config) {
  /// Composes a runtime whose storage and HTTP connections can be closed.
  this
    : _store = DriftSessionStore.openFile(File(config.session.dbPath)),
      _http = DioHttpStreamClient() {
    runtime = composeRuntime(config, store: _store, httpClient: _http);
  }

  final DriftSessionStore _store;
  final DioHttpStreamClient _http;

  /// The single shared runtime used by the command.
  late final AgentRuntime runtime;

  Future<void>? _closing;

  /// Cancels and drains turns before releasing storage and HTTP resources.
  Future<void> close() => _closing ??= _close();

  Future<void> _close() async {
    try {
      await runtime.shutdown();
    } finally {
      _http.close();
      await _store.close();
    }
  }
}
