import 'dart:convert';
import 'dart:io';

import 'package:args/command_runner.dart';
import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:path/path.dart' as p;

import 'cli.dart';

/// Registers model catalog, configuration validation, and saved-key commands.
void addModelCommands(AtlasCommandRunner cli) {
  final models = _Group(
    'models',
    'List configured models or refresh the offline catalog.',
  );
  models.addSubcommand(
    _Action('list', 'List model IDs and context limits.', () async {
      for (final model in composeModels(cli.loadConfiguration())) {
        cli.out.writeln(
          '${model.ref}\t${model.contextWindow}\t${model.maxOutputTokens}\t${model.name}',
        );
      }
      return 0;
    }),
  );
  models.addSubcommand(
    _Action('refresh', 'Refresh models.dev metadata for subsequent starts.', () async {
      final catalog = await ModelCatalog.refresh(
        Directory(p.join(cli.home, '.atlas')),
      );
      final count = catalog.providers.values.fold(
        0,
        (sum, models) => sum + models.length,
      );
      cli.out.writeln(
        'Updated $count models. Restart active Atlas hosts to use the catalog.',
      );
      return 0;
    }),
  );
  cli.addCommand(models);
  final config = _Group('config', 'Validate Atlas JSON configuration.');
  config.addSubcommand(
    _Action('validate', 'Validate settings.json, models.json and mcp.json without connecting.', () async {
      final models = composeModels(cli.loadConfiguration());
      cli.out.writeln(
        'Configuration valid: ${models.length} models. Credentials are resolved at request time.',
      );
      return 0;
    }),
  );
  cli.addCommand(config);
  final auth = _Group('auth', 'Manage saved API keys in auth.json.');
  auth.addSubcommand(_AuthCommand(cli, remove: false));
  auth.addSubcommand(_AuthCommand(cli, remove: true));
  cli.addCommand(auth);
}

final class _Group(
  @override final String name,
  @override final String description,
) extends Command<int> {
  @override
  void printUsage() => (runner as AtlasCommandRunner).out.writeln(usage);
}

final class _Action(
  @override final String name,
  @override final String description,
  final Future<int> Function() action,
) extends Command<int> {
  @override
  bool get takesArguments => false;
  @override
  Future<int> run() => action();
  @override
  void printUsage() => (runner as AtlasCommandRunner).out.writeln(usage);
}

final class _AuthCommand(
  final AtlasCommandRunner cli, {
  required final bool remove,
}) extends Command<int> {
  @override
  String get name => remove ? 'remove' : 'set';
  @override
  void printUsage() => cli.out.writeln(usage);
  @override
  String get description => remove
      ? 'Remove a saved key: atlas auth remove <provider>.'
      : 'Save a key from stdin: atlas auth set <provider>.';
  @override
  Future<int> run() async {
    final args = argResults!.rest;
    if (args.length != 1 || args.single.isEmpty || args.single.contains('/')) {
      usageException('Supply one provider ID.');
    }
    final provider = args.single;
    String? key;
    if (!remove) {
      if (stdin.hasTerminal) {
        cli.err.write('API key: ');
        final echo = stdin.echoMode;
        try {
          stdin.echoMode = false;
          key = stdin.readLineSync();
        } finally {
          stdin.echoMode = echo;
          cli.err.writeln();
        }
      } else {
        final bytes = <int>[];
        await for (final chunk in stdin) {
          if (bytes.length + chunk.length > 65536) {
            throw const ProviderAuthException('API key input is too large');
          }
          bytes.addAll(chunk);
        }
        key = utf8.decode(bytes).trim();
      }
      if (key == null || key.trim().isEmpty) {
        throw const ProviderAuthException('API key must not be empty');
      }
    }
    try {
      await AuthStore(File(p.join(cli.home, '.atlas', 'auth.json')))
          .setKey(provider, key);
    } on SafeMessageException catch (error) {
      cli.err.writeln(error.safeMessage);
      return 78;
    }
    cli.out.writeln(
      remove
          ? 'Removed saved credentials for $provider.'
          : 'Saved credentials for $provider.',
    );
    return 0;
  }
}
