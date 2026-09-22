import 'dart:convert';
import 'dart:io';

/// Minimal stdio peer recording only the test variables supplied by its caller.
Future<void> main(List<String> arguments) async {
  final record = File(arguments.single);
  await for (final line
      in stdin.transform(utf8.decoder).transform(const LineSplitter())) {
    final request = jsonDecode(line) as Map<String, dynamic>;
    final id = request['id'];
    if (id == null) continue;
    final result = switch (request['method']) {
      'initialize' => <String, Object?>{
        'protocolVersion': 1,
        'agentCapabilities': <String, Object?>{},
        'authMethods': <Object?>[],
      },
      'session/new' => <String, Object?>{'sessionId': 'environment-test'},
      _ => <String, Object?>{},
    };
    if (request['method'] == 'session/new') {
      await record.writeAsString(
        '${jsonEncode({'value': Platform.environment['ATLAS_ACP_ENV_TEST'], 'home': Platform.environment['HOME'], 'cwd': request['params']['cwd']})}\n',
        mode: FileMode.append,
      );
    }
    stdout.writeln(jsonEncode({'jsonrpc': '2.0', 'id': id, 'result': result}));
  }
}
