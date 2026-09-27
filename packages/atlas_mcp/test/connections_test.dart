import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_mcp/atlas_mcp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'fixtures/server.dart';

void main() {
  final fixture = File('test/fixtures/server.dart').absolute.path;
  ToolContext context([CancellationToken? cancellation]) => ToolContext(
    sessionId: SessionId('session'),
    turnId: TurnId('turn'),
    workingDirectory: Directory.current.path,
    cancellation: cancellation,
  );
  Tool tool(McpConnections connections, String name) =>
      connections.tools.firstWhere((t) => t.descriptor.description == name);
  McpStdioOptions stdio(
    String name, {
    Duration call = const Duration(seconds: 2),
    List<String> extra = const [],
  }) => McpStdioOptions(
    name: name,
    command: Platform.resolvedExecutable,
    args: [fixture, ...extra],
    environment: {...Platform.environment, 'ATLAS_MCP_TEST': 'snapshot'},
    workingDirectory: Directory.current.path,
    callTimeout: call,
  );

  test(
    'stdio discovers all pages, namespaces servers and maps results',
    () async {
      final connections = await McpConnections.connect([
        stdio('one'),
        stdio('two'),
      ]);
      addTearDown(connections.close);
      expect(connections.tools, hasLength(16));
      final names = connections.tools.map((t) => t.descriptor.name).toSet();
      expect(names, hasLength(16));
      expect(
        names.every((n) => RegExp(r'^[A-Za-z0-9_-]{1,64}$').hasMatch(n)),
        isTrue,
      );
      final echo = await tool(connections, 'echo').execute(context(), {});
      expect(echo.content, 'snapshot|${Directory.current.path}');
      final mixed = await tool(
        connections,
        'echo',
      ).execute(context(), {'mixed': true});
      expect(mixed.isError, isFalse);
      expect(
        mixed.content,
        'first\n[Unsupported MCP image content omitted]\nfile:///note\nembedded\nhttps://example.invalid/resource\nlast',
      );
      final structured = await tool(
        connections,
        'structured',
      ).execute(context(), {});
      expect(jsonDecode(structured.content), {'answer': 42});
      expect(
        (await tool(connections, 'error').execute(context(), {})).isError,
        isTrue,
      );
      expect(
        (await tool(
          connections,
          'binary',
        ).execute(context(), {})).metadata['failure_kind'],
        'unsupported_content',
      );
      expect(
        (await tool(connections, 'empty').execute(context(), {})).isError,
        isFalse,
      );
      final large = await tool(connections, 'large').execute(context(), {});
      expect(large.metadata['truncated'], isTrue);
      expect(
        utf8.encode(large.content).length +
            utf8.encode(jsonEncode(large.metadata)).length,
        lessThanOrEqualTo(50 * 1024),
      );
      expect(large.content, isNot(contains('\uFFFD')));
    },
  );

  test('cancellation and deadline leave shared connection usable', () async {
    final connections = await McpConnections.connect([
      stdio('local', call: const Duration(milliseconds: 200)),
    ]);
    addTearDown(connections.close);
    final cancellation = CancellationToken();
    final pending = tool(
      connections,
      'hang',
    ).execute(context(cancellation), {});
    final echo = await tool(
      connections,
      'echo',
    ).execute(context(), {'text': 'parallel'});
    cancellation.cancel();
    expect(echo.content, 'parallel');
    expect((await pending).metadata['failure_kind'], 'cancelled');
    expect(
      (await tool(
        connections,
        'hang',
      ).execute(context(), {})).metadata['failure_kind'],
      'timeout',
    );
    expect(
      (await tool(
        connections,
        'echo',
      ).execute(context(), {'text': 'still usable'})).content,
      'still usable',
    );
  });

  test(
    'startup deadline closes a silent subprocess and reports safe identity',
    () async {
      final options = McpStdioOptions(
        name: 'silent',
        command: Platform.resolvedExecutable,
        args: [fixture, 'silent'],
        environment: Platform.environment,
        workingDirectory: Directory.current.path,
        startupTimeout: const Duration(milliseconds: 200),
      );
      await expectLater(
        McpConnections.connect([options]),
        throwsA(
          isA<McpConnectionException>()
              .having((e) => e.kind, 'kind', 'timeout')
              .having((e) => e.server, 'server', 'silent'),
        ),
      );
    },
  );

  test(
    'modern stdio retains scalar JSON and freezes catalog change notices',
    () async {
      final logger = _Logger();
      final connections = await McpConnections.connect([
        stdio('modern', extra: ['modern']),
      ], logger: logger);
      addTearDown(connections.close);
      final before = connections.tools.map((t) => t.descriptor.name).toList();
      final structured = await tool(
        connections,
        'structured',
      ).execute(context(), {});
      expect(jsonDecode(structured.content), [42, 'answer']);
      final progress = <ToolOutputSnapshot>[];
      final invocation = await tool(connections, 'echo').execute(
        ToolContext(
          sessionId: SessionId('modern'),
          turnId: TurnId('progress'),
          workingDirectory: Directory.current.path,
          onOutput: progress.add,
        ),
        {'text': 'changed', 'notify': true},
      );
      expect(invocation.content, 'changed');
      expect(progress, isNotEmpty);
      expect(
        logger.events.where(
          (e) => e.code == 'mcp.catalog_changed_restart_required',
        ),
        hasLength(1),
      );
      expect(connections.tools.map((t) => t.descriptor.name), before);
      expect(
        logger.events.map((e) => e.message).join(),
        isNot(contains('private-stderr-secret')),
      );
    },
  );

  test('long tool names retain original dispatch identity', () async {
    final connections = await McpConnections.connect([
      stdio('names', extra: ['names']),
    ]);
    addTearDown(connections.close);
    expect(
      connections.tools.every((t) => t.descriptor.name.length <= 64),
      isTrue,
    );
    expect(
      (await tool(
        connections,
        'echo',
      ).execute(context(), {'text': 'original'})).content,
      'original',
    );
  });

  for (final failure in ['repeat', 'invalid']) {
    test(
      '$failure catalog fails and releases a previously opened process',
      () async {
        final temp = await Directory.systemTemp.createTemp(
          'atlas_mcp_cleanup_',
        );
        addTearDown(() => temp.delete(recursive: true));
        final pidFile = File('${temp.path}/pid');
        final first = McpStdioOptions(
          name: 'first',
          command: Platform.resolvedExecutable,
          args: [fixture],
          environment: {
            ...Platform.environment,
            'ATLAS_MCP_PID_FILE': pidFile.path,
          },
          workingDirectory: temp.path,
        );
        await expectLater(
          McpConnections.connect([
            first,
            stdio('bad', extra: [failure]),
          ]),
          throwsA(isA<McpConnectionException>()),
        );
        expect(pidFile.existsSync(), isTrue);
        if (!Platform.isWindows) {
          final probe = await Process.run('/bin/kill', [
            '-0',
            pidFile.readAsStringSync(),
          ]);
          expect(probe.exitCode, isNot(0));
        }
      },
    );
  }

  for (final modern in [false, true]) {
    test(
      'HTTP cancellation isolates concurrent requests (${modern ? 'modern' : 'legacy'})',
      () async {
        final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
        addTearDown(() => server.close(force: true));
        final peer = FixturePeer()
          ..modern = modern
          ..listChanged = false;
        server.listen((request) async {
          if (request.method != 'POST') {
            request.response.statusCode = 405;
            await request.response.close();
            return;
          }
          final body = jsonDecode(
            await utf8.decoder.bind(request).join(),
          ) as Map<String, dynamic>;
          final messages = <Map<String, dynamic>>[];
          await peer.handle(body, messages.add);
          if ((body['params'] as Map<String, dynamic>?)?['name'] == 'hang') {
            return;
          }
          request.response.headers.contentType = ContentType.json;
          request.response.statusCode =
              !modern && body['method'] == 'server/discover'
              ? 400
              : messages.isEmpty
              ? 202
              : 200;
          if (messages.isNotEmpty) {
            request.response.write(jsonEncode(messages.last));
          }
          await request.response.close();
        });
        final connections = await McpConnections.connect([
          McpHttpOptions(
            name: 'http',
            url: Uri.parse('http://127.0.0.1:${server.port}/mcp'),
            callTimeout: const Duration(milliseconds: 300),
          ),
        ]);
        addTearDown(connections.close);
        final cancellation = CancellationToken();
        final hanging = tool(
          connections,
          'hang',
        ).execute(context(cancellation), {});
        expect(
          (await tool(
            connections,
            'echo',
          ).execute(context(), {'text': 'parallel'})).content,
          'parallel',
        );
        cancellation.cancel();
        final cancelled = await hanging;
        expect(cancelled.metadata['failure_kind'], 'cancelled');
        // The abandoned request cannot be aborted through the SDK, so Atlas
        // stops using that connection instead of leaking one per attempt.
        expect(cancelled.content, contains('restart Atlas'));
        expect(
          (await tool(
            connections,
            'echo',
          ).execute(context(), {'text': 'after'})).metadata['failure_kind'],
          'connection_abandoned',
        );
      },
    );
  }

  for (final modern in [false, true]) {
    test(
      'HTTP deadline stops a hung server from accumulating requests (${modern ? 'modern' : 'legacy'})',
      () async {
        var calls = 0;
        final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
        addTearDown(() => server.close(force: true));
        final peer = FixturePeer()
          ..modern = modern
          ..listChanged = false;
        server.listen((request) async {
          if (request.method != 'POST') {
            request.response.statusCode = 405;
            await request.response.close();
            return;
          }
          final body = jsonDecode(
            await utf8.decoder.bind(request).join(),
          ) as Map<String, dynamic>;
          final messages = <Map<String, dynamic>>[];
          await peer.handle(body, messages.add);
          if ((body['params'] as Map<String, dynamic>?)?['name'] == 'hang') {
            calls++;
            // Never answer: the request stays in flight until the process ends.
            await Completer<void>().future;
            return;
          }
          request.response.headers.contentType = ContentType.json;
          request.response.statusCode =
              !modern && body['method'] == 'server/discover'
              ? 400
              : messages.isEmpty
              ? 202
              : 200;
          if (messages.isNotEmpty) {
            request.response.write(jsonEncode(messages.last));
          }
          await request.response.close();
        });
        final connections = await McpConnections.connect([
          McpHttpOptions(
            name: 'http',
            url: Uri.parse('http://127.0.0.1:${server.port}/mcp'),
            callTimeout: const Duration(milliseconds: 200),
          ),
        ]);
        addTearDown(connections.close);
        final first = await tool(connections, 'hang').execute(context(), {});
        expect(first.metadata['failure_kind'], 'timeout');
        expect(calls, 1);
        for (var attempt = 0; attempt < 2; attempt++) {
          final next = await tool(connections, 'hang').execute(context(), {});
          expect(next.metadata['failure_kind'], 'connection_abandoned');
        }
        expect(calls, 1);
      },
    );
  }

  test('stdio unexpected exit never restarts or replays a tool', () async {
    final connections = await McpConnections.connect([stdio('exit')]);
    addTearDown(connections.close);
    final result = await tool(
      connections,
      'echo',
    ).execute(context(), {'exit': true});
    expect(result.isError, isTrue);
    expect(result.content, contains('not retried'));
    expect(
      (await tool(
        connections,
        'echo',
      ).execute(context(), {'text': 'after'})).isError,
      isTrue,
    );
  });

  for (final status in [401, 403]) {
    test('HTTP $status becomes a redacted authentication failure', () async {
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      addTearDown(() => server.close(force: true));
      server.listen((request) async {
        await request.drain<void>();
        request.response.statusCode = status;
        request.response.write('private-secret');
        await request.response.close();
      });
      await expectLater(
        McpConnections.connect([
          McpHttpOptions(
            name: 'auth',
            url: Uri.parse('http://127.0.0.1:${server.port}/mcp?secret=query'),
          ),
        ]),
        throwsA(
          isA<McpConnectionException>()
              .having((e) => e.kind, 'kind', 'authentication_error')
              .having((e) => e.toString(), 'safe', isNot(contains('secret'))),
        ),
      );
    });
  }

  for (final modern in [false, true]) {
    test(
      'oversized tool schema or catalog fails startup with a safe server name',
      () async {
        for (final mode in ['big-schema', 'big-catalog']) {
          await expectLater(
            McpConnections.connect([
              stdio('bad', extra: [mode]),
            ]),
            throwsA(
              isA<McpConnectionException>()
                  .having((e) => e.server, 'server', 'bad')
                  .having((e) => e.kind, 'kind', 'invalid_catalog_or_result'),
            ),
          );
        }
      },
    );

    test(
      'oversized tool description is truncated to the documented bound',
      () async {
        final connections = await McpConnections.connect([
          stdio('wordy', extra: ['big-description']),
        ]);
        addTearDown(connections.close);
        final description = connections.tools.single.descriptor.description;
        expect(utf8.encode(description).length, lessThan(2100));
        expect(description, contains('[MCP description truncated]'));
      },
    );

    for (final sse in [false, true]) {
      test(
        'HTTP ${modern ? 'modern' : 'legacy'} ${sse ? 'SSE' : 'JSON'} uses headers and never replays mutations',
        () async {
          final peer = FixturePeer()
            ..modern = modern
            ..listChanged = false;
          final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
          addTearDown(() => server.close(force: true));
          var rejectMutation = false;
          final headers = <String?>[];
          server.listen((request) async {
            headers.add(request.headers.value('authorization'));
            if (request.method != 'POST') {
              request.response.statusCode = 405;
              await request.response.close();
              return;
            }
            final body = jsonDecode(
              await utf8.decoder.bind(request).join(),
            ) as Map<String, dynamic>;
            final messages = <Map<String, dynamic>>[];
            await peer.handle(body, messages.add);
            if (rejectMutation && body['method'] == 'tools/call') {
              request.response.statusCode = 404;
              await request.response.close();
              return;
            }
            if (!modern && body['method'] == 'server/discover') {
              request.response.statusCode = 400;
            }
            if (body['method'] == 'initialize') {
              request.response.headers.set('Mcp-Session-Id', 'fixture-session');
            }
            if (messages.isEmpty) {
              request.response.statusCode = 202;
            } else if (sse && body['method'] != 'server/discover') {
              request.response.headers.contentType = ContentType(
                'text',
                'event-stream',
                charset: 'utf-8',
              );
              for (final message in messages) {
                request.response.write(
                  'event: message\ndata: ${jsonEncode(message)}\n\n',
                );
              }
            } else {
              request.response.headers.contentType = ContentType.json;
              request.response.write(jsonEncode(messages.last));
            }
            await request.response.close();
          });
          final connections = await McpConnections.connect([
            McpHttpOptions(
              name: 'remote',
              url: Uri.parse('http://127.0.0.1:${server.port}/mcp'),
              headers: {'Authorization': 'Bearer test-secret'},
            ),
          ]);
          addTearDown(connections.close);
          expect(
            (await tool(
              connections,
              'echo',
            ).execute(context(), {'text': 'remote'})).content,
            'remote',
          );
          final large = await tool(connections, 'large').execute(context(), {});
          expect(large.metadata['truncated'], isTrue);
          expect(utf8.encode(large.content).length, lessThan(50 * 1024));
          rejectMutation = true;
          final result = await tool(
            connections,
            'mutate',
          ).execute(context(), {});
          expect(result.isError, isTrue);
          expect(peer.mutations, 1);
          expect(result.content, isNot(contains('test-secret')));
          expect(headers, everyElement('Bearer test-secret'));
        },
      );
    }
  }
}

final class _Logger implements AtlasLogger {
  final events = <LogEvent>[];
  @override
  void log(LogEvent event) => events.add(event);
}
