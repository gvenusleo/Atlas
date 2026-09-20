import 'dart:io';

import 'package:args/args.dart';
import 'package:args/command_runner.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tui/atlas_tui.dart';
import 'package:io/io.dart' show ExitCode;
import 'package:stack_trace/stack_trace.dart';

import 'acp_command.dart';
import 'cache_command.dart';
import 'compose.dart';
import 'runtime_resources.dart';
import 'server_command.dart';
import 'version.dart';

/// Runs one invocation, routing diagnostics separately from command output.
Future<int> runCli(List<String> args, {AtlasCommandRunner? runner}) async {
  final cli = runner ?? AtlasCommandRunner();
  try {
    return await cli.run(args) ?? ExitCode.success.code;
  } on UsageException catch (error) {
    cli.err
      ..writeln(error.message)
      ..writeln()
      ..writeln(error.usage);
    return ExitCode.usage.code;
  } on ConfigLoadException catch (error) {
    cli.err.writeln('atlas: $error');
    return ExitCode.config.code;
  } catch (error, stack) {
    cli.err.writeln('atlas: $error');
    if (cli.verbose) cli.err.writeln(Trace.from(stack).terse);
    return ExitCode.software.code;
  } finally {
    await Future.wait([
      if (cli.out case final IOSink sink) sink.flush(),
      if (cli.err case final IOSink sink) sink.flush(),
    ]);
  }
}

/// Routes Atlas commands without owning the agent loop or presentation.
final class AtlasCommandRunner extends CommandRunner<int> {
  /// Creates a runner with injectable output and configuration for tests.
  AtlasCommandRunner({
    StringSink? out,
    StringSink? err,
    String? home,
    this._configLoader,
    bool Function()? terminalAvailable,
  }) : out = out ?? stdout,
       err = err ?? stderr,
       home =
           home ??
           Platform.environment['HOME'] ??
           Platform.environment['USERPROFILE'] ??
           '.',
       _terminalAvailable = terminalAvailable ?? (() => supportsAtlasTui),
       super('atlas', 'A local general-purpose AI agent.') {
    argParser
      ..addFlag(
        'version',
        abbr: 'V',
        negatable: false,
        help: 'Print the package version.',
      )
      ..addFlag(
        'verbose',
        abbr: 'v',
        negatable: false,
        help: 'Include a terse stack trace for unexpected failures.',
      );
    addCommand(_AcpCommand(this));
    addCommand(_ServerCommand(this));
    addCommand(_CacheCommand(this));
  }

  /// Command results and explicitly requested help.
  final StringSink out;

  /// Warnings, errors, and usage after invalid input.
  final StringSink err;

  /// The home directory containing the Atlas configuration.
  final String home;
  final AtlasConfig Function()? _configLoader;
  final bool Function() _terminalAvailable;

  /// Whether unexpected failures include stack traces.
  bool verbose = false;

  @override
  String get invocation => 'atlas [command] [arguments]';

  @override
  String get usageFooter =>
      'With no command, starts the interactive terminal UI.';

  @override
  void printUsage() => out.writeln(usage);

  /// Loads configuration only after command help and validation are complete.
  AtlasConfig loadConfiguration() =>
      _configLoader?.call() ?? loadConfig(File('$home/.atlas/config.yaml'));

  @override
  Future<int?> runCommand(ArgResults topLevelResults) async {
    verbose = topLevelResults.flag('verbose');
    if (topLevelResults.flag('version')) {
      if (topLevelResults.command != null || topLevelResults.rest.isNotEmpty) {
        usageException('--version cannot be combined with a command.');
      }
      out.writeln(packageVersion);
      return ExitCode.success.code;
    }
    if (topLevelResults.command != null ||
        topLevelResults.rest.isNotEmpty ||
        topLevelResults.flag('help')) {
      return super.runCommand(topLevelResults);
    }
    if (!_terminalAvailable()) {
      err.writeln(
        'atlas: the TUI requires interactive stdin/stdout, ANSI '
        'support, and NO_COLOR to be unset. Use atlas --help for commands.',
      );
      return ExitCode.usage.code;
    }
    final config = loadConfiguration();
    final resources = CliRuntimeResources(config);
    try {
      await runAtlasTui(
        runtime: resources.runtime,
        models: composeModels(config),
        skills: loadSkillCatalog(),
      );
      return ExitCode.success.code;
    } finally {
      await resources.close();
    }
  }
}

abstract class _AtlasCommand extends Command<int> {
  _AtlasCommand(this.cli);
  final AtlasCommandRunner cli;

  @override
  bool get takesArguments => false;

  @override
  void printUsage() => cli.out.writeln(usage);

  T validate<T>(T Function(ArgResults) parse) {
    try {
      return parse(argResults!);
    } on FormatException catch (error) {
      usageException(error.message);
    } on ArgumentError catch (error) {
      usageException('${error.message}');
    }
  }
}

final class _AcpCommand extends _AtlasCommand {
  _AcpCommand(super.cli);

  @override
  String get name => 'acp';
  @override
  String get description => 'Serve ACP over NDJSON stdin/stdout.';

  @override
  Future<int> run() async {
    await runAcpCommand(cli.loadConfiguration());
    return ExitCode.success.code;
  }
}

final class _ServerCommand extends _AtlasCommand {
  _ServerCommand(super.cli) {
    addServerOptions(argParser);
  }

  @override
  String get name => 'server';
  @override
  String get description => 'Serve ACP over an authenticated WebSocket.';

  @override
  Future<int> run() async {
    final options = validate(ServerOptions.fromResults);
    // Token rotation does not need providers, storage, or a listening socket.
    await runServerCommand(
      options: options,
      home: cli.home,
      loadConfiguration: cli.loadConfiguration,
      out: cli.out,
      err: cli.err,
    );
    return ExitCode.success.code;
  }
}

final class _CacheCommand extends _AtlasCommand {
  _CacheCommand(super.cli) {
    addCacheOptions(argParser);
  }

  @override
  String get name => 'cache';
  @override
  String get description => 'Report prompt-cache reuse from recorded turns.';

  @override
  Future<int> run() async {
    final options = validate(CacheOptions.fromResults);
    final config = cli.loadConfiguration();
    final store = DriftSessionStore.openFile(File(config.session.dbPath));
    try {
      return await runCacheCommand(
        store,
        config: config,
        options: options,
        out: cli.out,
        err: cli.err,
      );
    } finally {
      await store.close();
    }
  }
}
