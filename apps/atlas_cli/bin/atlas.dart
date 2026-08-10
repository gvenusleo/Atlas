import 'dart:io';

import 'package:atlas_cli/atlas_cli.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';

/// The `atlas` command-line entry point.
///
/// Loads `~/.atlas/config.yaml`, composes one runtime, and starts the Nocterm
/// chat interface against it.
Future<void> main(List<String> args) async {
  final home = Platform.environment['HOME'] ?? '.';
  final configFile = File('$home/.atlas/config.yaml');
  final AtlasConfig config;
  try {
    config = loadConfig(configFile);
  } on ConfigLoadException catch (error) {
    stderr.writeln('cannot load ${configFile.path}: $error');
    exit(1);
  }

  final runtime = composeRuntime(config);
  await runApp(AtlasTuiApp(runtime: runtime));
}
