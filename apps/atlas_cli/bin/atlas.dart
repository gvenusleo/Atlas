import 'dart:io';

import 'package:atlas_cli/atlas_cli.dart';

/// Runs Atlas and lets asynchronous resources close before process exit.
Future<void> main(List<String> args) async {
  exitCode = await runCli(args);
}
