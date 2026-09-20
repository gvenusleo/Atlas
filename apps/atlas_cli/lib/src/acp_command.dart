import 'dart:async';
import 'dart:io';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';

import 'runtime_resources.dart';
import 'termination_signals.dart';

/// Serves ACP until stdin closes or a termination signal arrives.
Future<void> runAcpCommand(AtlasConfig config) async {
  final resources = CliRuntimeResources(config);
  final signals = TerminationSignals();
  final input = StreamController<List<int>>();
  final subscription = stdin.listen(
    input.add,
    onError: input.addError,
    onDone: input.close,
  );
  final serving = AcpServer(
    resources.runtime,
    models: composeModels(config),
  ).serve(input: input.stream);
  try {
    await Future.any([serving, signals.interrupted]);
  } finally {
    try {
      await resources.runtime.shutdown();
      await subscription.cancel();
      await input.close();
      await serving;
    } finally {
      await signals.close();
      await resources.close();
    }
  }
}
