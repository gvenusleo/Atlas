import 'dart:io';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_cli/atlas_cli.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_tui/atlas_tui.dart';

/// The `atlas` command-line entry point.
///
/// Loads `~/.atlas/config.yaml`, composes one runtime, and starts the chat
/// interface against it. Running `atlas acp` serves the same runtime to ACP
/// clients over NDJSON stdio instead.
Future<void> main(List<String> args) async {
  const standalone = {'--help', '-h', '--version', '-V'};
  final command = args.isEmpty ? null : args.first;
  if (args.isNotEmpty &&
      command != 'acp' &&
      command != 'server' &&
      !(args.length == 1 && standalone.contains(command))) {
    stderr.writeln('unknown command or arguments: ${args.join(' ')}');
    stderr.writeln('usage: atlas [acp|server|--help|--version]');
    exit(64);
  }
  if (args.length == 1 && (command == '--help' || command == '-h')) {
    stdout.writeln('usage: atlas [acp|server]');
    return;
  }
  if (args.length == 1 && (command == '--version' || command == '-V')) {
    stdout.writeln('0.1.0');
    return;
  }
  final home = Platform.environment['HOME'] ?? '.';
  final configFile = File('$home/.atlas/config.yaml');
  final AtlasConfig config;
  try {
    config = loadConfig(configFile);
  } on ConfigLoadException catch (error) {
    stderr.writeln('cannot load ${configFile.path}: $error');
    exit(1);
  }

  if (args.isNotEmpty && args.first == 'acp') {
    if (args.length > 1) {
      stderr.writeln('usage: atlas acp (takes no arguments)');
      exit(64);
    }
    final runtime = composeRuntime(config);
    // The connection ends when the client closes stdin. Flush pending wire
    // output and exit explicitly so lingering storage handles do not keep
    // the process alive.
    await AcpServer(runtime, models: composeModels(config)).serve();
    await stdout.flush();
    exit(0);
  }
  if (args.isNotEmpty && args.first == 'server') {
    await runServerCommand(config, home: home, args: args.sublist(1));
    exit(0);
  }
  final runtime = composeRuntime(config);
  await runAtlasTui(
    runtime: runtime,
    models: composeModels(config),
    skills: loadSkillCatalog(),
  );
}
