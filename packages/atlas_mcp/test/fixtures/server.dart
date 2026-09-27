import 'dart:async';
import 'dart:convert';
import 'dart:io';

// Independent JSON-RPC peer: no SDK implementation is shared with the client.
Future<void> main(List<String> args) async {
  final pidFile = Platform.environment['ATLAS_MCP_PID_FILE'];
  if (pidFile != null) File(pidFile).writeAsStringSync('$pid');
  if (args.contains('silent')) {
    await stdin.drain<void>();
    return;
  }
  final peer = FixturePeer()
    ..modern = args.contains('modern')
    ..repeatCursor = args.contains('repeat')
    ..invalidSchema = args.contains('invalid')
    ..oddNames = args.contains('names')
    ..bigSchema = args.contains('big-schema')
    ..bigDescription = args.contains('big-description')
    ..bigCatalog = args.contains('big-catalog');
  stderr.writeln('private-stderr-secret');
  final outputs = <Future<void>>[];
  await for (final line
      in stdin.transform(utf8.decoder).transform(const LineSplitter())) {
    final request = jsonDecode(line) as Map<String, dynamic>;
    outputs.add(
      peer.handle(request, (message) {
        stdout.writeln(jsonEncode(message));
      }),
    );
  }
  await Future.wait(outputs);
}

class FixturePeer {
  int mutations = 0;
  int cancellations = 0;
  final calls = <String>[];
  bool repeatCursor = false;
  bool modern = false;
  bool listChanged = true;
  bool invalidSchema = false;
  bool oddNames = false;
  bool bigSchema = false;
  bool bigDescription = false;
  bool bigCatalog = false;
  Object? subscriptionId;

  Future<void> handle(
    Map<String, dynamic> request,
    void Function(Map<String, dynamic>) send,
  ) async {
    final id = request['id'];
    final method = request['method'];
    final params = request['params'] as Map<String, dynamic>? ?? {};
    void result(Map<String, dynamic> value) => send({
      'jsonrpc': '2.0',
      'id': id,
      'result': {if (modern) 'resultType': 'complete', ...value},
    });
    void error(int code) => send({
      'jsonrpc': '2.0',
      'id': id,
      'error': {'code': code, 'message': 'private-secret'},
    });
    switch (method) {
      case 'server/discover':
        if (modern) {
          result({
            'resultType': 'complete',
            'supportedVersions': ['2026-07-28'],
            'ttlMs': 0,
            'cacheScope': 'private',
            'capabilities': {
              'tools': {'listChanged': listChanged},
            },
            '_meta': {
              'io.modelcontextprotocol/serverInfo': {
                'name': 'fixture',
                'version': '1',
              },
            },
          });
        } else {
          error(-32601);
        }
      case 'subscriptions/listen':
        subscriptionId = id;
        send({
          'jsonrpc': '2.0',
          'method': 'notifications/subscriptions/acknowledged',
          'params': {
            'notifications': {'toolsListChanged': true},
            '_meta': {'io.modelcontextprotocol/subscriptionId': id},
          },
        });
      case 'initialize':
        result({
          'protocolVersion': '2025-11-25',
          'capabilities': {
            'tools': {'listChanged': listChanged},
          },
          'serverInfo': {'name': 'fixture', 'version': '1'},
        });
      case 'notifications/initialized':
        break;
      case 'notifications/cancelled':
        cancellations++;
      case 'tools/list':
        final second = params['cursor'] != null;
        if (bigSchema) {
          result({
            'tools': [
              {
                'name': 'huge',
                'description': 'huge',
                'inputSchema': {
                  'type': 'object',
                  'properties': {
                    'text': {'type': 'string', 'description': 'x' * 70000},
                  },
                },
              },
            ],
          });
          break;
        }
        if (bigDescription) {
          result({
            'tools': [
              {
                'name': 'wordy',
                'description': 'y' * (1024 * 1024),
                'inputSchema': {'type': 'object'},
              },
            ],
          });
          break;
        }
        if (bigCatalog) {
          result({
            'tools': [
              for (var index = 0; index < 40; index++)
                {
                  'name': 'tool$index',
                  'description': 'tool',
                  'inputSchema': {
                    'type': 'object',
                    'properties': {
                      'text': {'type': 'string', 'description': 'z' * 40000},
                    },
                  },
                },
            ],
          });
          break;
        }
        result({
          if (modern) ...{'ttlMs': 0, 'cacheScope': 'private'},
          'tools': [
            for (final name
                in second
                    ? ['echo', 'hang', 'error', 'binary', 'empty']
                    : ['mutate', 'structured', 'large'])
              {
                'name': oddNames ? '${'x' * 90}.$name' : name,
                'description': name,
                'inputSchema': {
                  'type': invalidSchema ? 'array' : 'object',
                  'properties': {
                    'text': {'type': 'string'},
                  },
                  'additionalProperties': true,
                },
              },
          ],
          if (!second || repeatCursor) 'nextCursor': 'next',
        });
      case 'tools/call':
        final originalName = params['name'] as String;
        final name = oddNames ? originalName.split('.').last : originalName;
        calls.add(name);
        final arguments = params['arguments'] as Map<String, dynamic>? ?? {};
        if (name == 'hang') return;
        if (arguments['notify'] == true) {
          send({
            'jsonrpc': '2.0',
            'method': 'notifications/tools/list_changed',
            if (modern)
              'params': {
                '_meta': {
                  'io.modelcontextprotocol/subscriptionId': subscriptionId,
                },
              },
          });
        }
        if (arguments['exit'] == true) exit(17);
        if (name == 'mutate') mutations++;
        final progress =
            (params['_meta'] as Map<String, dynamic>?)?['progressToken'];
        if (progress != null) {
          send({
            'jsonrpc': '2.0',
            'method': 'notifications/progress',
            'params': {
              'progressToken': progress,
              'progress': 1,
              'total': 1,
              'message': 'finished',
            },
          });
        }
        if (arguments['mixed'] == true) {
          result({
            'content': [
              {'type': 'text', 'text': 'first'},
              {'type': 'image', 'data': 'YQ==', 'mimeType': 'image/png'},
              {
                'type': 'resource',
                'resource': {'uri': 'file:///note', 'text': 'embedded'},
              },
              {
                'type': 'resource_link',
                'uri': 'https://example.invalid/resource',
                'name': 'link',
              },
              {'type': 'text', 'text': 'last'},
            ],
          });
        } else if (name == 'structured') {
          result({
            'content': <Object?>[],
            'structuredContent': modern ? [42, 'answer'] : {'answer': 42},
          });
        } else if (name == 'binary') {
          result({
            'content': [
              {'type': 'image', 'data': 'YQ==', 'mimeType': 'image/png'},
            ],
          });
        } else if (name == 'empty') {
          result({'content': <Object?>[]});
        } else {
          result({
            'isError': name == 'error',
            'content': [
              {
                'type': 'text',
                'text': name == 'large'
                    ? '界' * 40000
                    : name == 'mutate'
                    ? '$mutations'
                    : arguments['text'] ??
                          '${Platform.environment['ATLAS_MCP_TEST']}|${Directory.current.path}',
              },
            ],
          });
        }
      default:
        if (id != null) error(-32601);
    }
  }
}
