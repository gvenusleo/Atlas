import 'dart:io';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_cli/atlas_cli.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';

/// The `atlas` command-line entry point.
///
/// Loads `~/.atlas/config.yaml`, composes one runtime, and starts the Nocterm
/// chat interface against it. Running `atlas acp` serves the same runtime to
/// ACP clients over NDJSON stdio instead.
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
  if (args.isNotEmpty && args.first == 'acp') {
    // The connection ends when the client closes stdin. Flush pending wire
    // output and exit explicitly so lingering storage handles do not keep
    // the process alive.
    await AcpServer(runtime).serve();
    await stdout.flush();
    exit(0);
  }
  await runApp(
    NoctermApp(
      child: AtlasTuiApp(
        runtime: runtime,
        models: composeModels(config),
        skills: loadSkillCatalog(),
        onQuit: shutdownApp,
      ),
    ),
  );
}
