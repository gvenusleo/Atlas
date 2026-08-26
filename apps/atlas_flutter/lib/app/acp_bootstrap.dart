import 'dart:async';
import 'dart:io';

import 'package:acpd_io/acpd_io.dart';
import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';

import 'acp_connections.dart';
import 'runtime_environment.dart';

/// Starts [connection] as a child process and wraps it in an [AcpClient].
///
/// The returned environment injects the ACP client as the runtime, so
/// presentation code consumes a remote agent exactly like the local runtime.
/// The model catalog is discovered from a temporary session's config options
/// and the temporary session is removed again.
Future<RuntimeBootstrap> bootstrapAcpClient(AcpConnection connection) async {
  try {
    final agent = await AcpAgent.start(
      AcpAgentConfig(command: connection.command, args: connection.arguments),
    );
    final client = AcpClient(agent.transport);
    await client.connect();
    final home =
        Platform.environment['HOME'] ??
        Platform.environment['USERPROFILE'] ??
        Directory.current.path;
    final probe = await client.createSession(workingDirectory: home);
    final catalog = client.catalog;
    try {
      await client.deleteSession(probe.id);
    } on Object {
      // The probe session is harmless; leaving it behind is acceptable.
    }
    return RuntimeBootstrap.ready(
      RuntimeEnvironment(
        runtime: client,
        models: catalog,
        skills: const _EmptySkillCatalog(),
        onClose: () async {
          await client.close();
          await agent.close();
        },
      ),
    );
  } on Object catch (error) {
    return RuntimeBootstrap.failed('Cannot start ACP server: $error');
  }
}

/// An empty skill catalog: skills live on the ACP server side.
final class _EmptySkillCatalog implements SkillCatalog {
  const _EmptySkillCatalog();

  @override
  Skill? lookup(String name) => null;

  @override
  List<SkillSummary> get summaries => const [];
}
