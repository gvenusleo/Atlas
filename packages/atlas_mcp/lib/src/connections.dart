import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:crypto/crypto.dart';
import 'package:mcp_dart/mcp_dart.dart' as mcp;

import 'options.dart';
import 'result_conversion.dart';

/// Maximum UTF-8 bytes kept from one server-provided tool description.
const _maxToolDescriptionBytes = 2048;

/// Maximum encoded size of one advertised tool input schema.
const _maxToolSchemaBytes = 64 * 1024;

/// Maximum encoded catalog metadata accepted from one server.
const _maxCatalogBytes = 1024 * 1024;

/// A redacted connection failure safe for startup diagnostics.
final class const McpConnectionException(
  /// Configured server identifier.
  final String server,

  /// Stable failure classification.
  final String kind,
) implements SafeMessageException {
  /// Creates a safe connection failure.
  this;

  @override
  String get safeMessage => 'MCP server "$server": $kind';

  @override
  String? get diagnosticDetail => null;

  @override
  String toString() => safeMessage;
}

/// Owns MCP connections and their immutable discovered tool wrappers.
final class McpConnections._(final List<_Connection> _connections) {
  /// Creates an owner after successful discovery.
  this;

  /// Connects servers in configuration order, closing all on partial failure.
  static Future<McpConnections> connect(
    List<McpServerOptions> servers, {
    CancellationToken? cancellation,
    AtlasLogger logger = const NoopLogger(),
  }) async {
    // SDK logs include unredacted protocol payloads and exception strings.
    mcp.silenceMcpLogs();
    final connections = <_Connection>[];
    final names = <String>{};
    final toolNames = <String>{};
    try {
      for (final options in servers) {
        cancellation?.throwIfCancelled();
        if (!names.add(options.name)) {
          throw McpConnectionException(options.name, 'duplicate server name');
        }
        final connection = _Connection(options, logger);
        connections.add(connection);
        await connection.open(cancellation);
        for (final tool in connection.tools) {
          if (!toolNames.add(tool.descriptor.name)) {
            throw McpConnectionException(options.name, 'tool name collision');
          }
        }
      }
      cancellation?.throwIfCancelled();
      return McpConnections._(connections);
    } catch (_) {
      await Future.wait(connections.map((c) => c.close()));
      rethrow;
    }
  }

  /// Tools in stable server and tool-name order.
  List<Tool> get tools => List.unmodifiable([
    for (final connection in _connections) ...connection.tools,
  ]);

  Future<void>? _closing;

  /// Releases every connection, even if one close fails.
  Future<void> close() => _closing ??= Future.wait(
    _connections.map((connection) => connection.close()),
  ).then((_) {});
}

final class _Connection(
  final McpServerOptions options,
  final AtlasLogger logger,
) {
  this;

  final client = _CatalogClient();
  late final mcp.Transport transport;
  final tools = <Tool>[];
  final _httpState = _HttpState();
  StreamSubscription<List<int>>? _stderr;
  Future<void>? _closing;
  bool _closed = false;
  bool _catalogNotice = false;
  bool _stderrNotice = false;
  bool _transportNotice = false;
  StreamSubscription<mcp.JsonRpcNotification>? _notifications;

  void log(String kind) {
    try {
      logger.log(
        LogEvent(
          level: LogLevel.warn,
          code: 'mcp.$kind',
          message: 'MCP server "${options.name}": $kind',
          fields: {'server': options.name},
          occurredAt: DateTime.now().toUtc(),
        ),
      );
    } catch (_) {
      // Diagnostic I/O must not interrupt protocol handling or cleanup.
    }
  }

  Future<void> open(CancellationToken? cancellation) async {
    transport = switch (options) {
      McpStdioOptions(
        :final command,
        :final args,
        :final environment,
        :final workingDirectory,
      ) =>
        _StdioTransport(
          mcp.StdioServerParameters(
            command: command,
            args: args,
            environment: environment,
            includeParentEnvironment: false,
            workingDirectory: workingDirectory,
            stderrMode: ProcessStartMode.normal,
            restartOnUnexpectedExit: false,
          ),
          _attachStderr,
        ),
      McpHttpOptions(:final url, :final headers) => _HttpTransport(
        url,
        server: options.name,
        state: _httpState,
        opts: mcp.StreamableHttpClientTransportOptions(
          requestInit: {'headers': headers},
          reconnectionOptions: const mcp.StreamableHttpReconnectionOptions(
            initialReconnectionDelay: 1000,
            maxReconnectionDelay: 5000,
            reconnectionDelayGrowFactor: 1.5,
            maxRetries: 0,
          ),
        ),
      ),
    };
    client.onclose = () => _closed = true;
    client.onerror = (_) {
      if (!_transportNotice) {
        _transportNotice = true;
        log('transport_error');
      }
    };
    client.setNotificationHandler<mcp.JsonRpcToolListChangedNotification>(
      mcp.Method.notificationsToolsListChanged,
      (_) async {
        if (!_catalogNotice) {
          _catalogNotice = true;
          log('catalog_changed_restart_required');
        }
      },
      (_, meta) => mcp.JsonRpcToolListChangedNotification(meta: meta),
    );
    final cancelled = Completer<void>();
    final subscription = cancellation?.whenCancelled.asStream().listen((_) {
      cancelled.completeError(const TurnCancelledException());
    });
    try {
      cancellation?.throwIfCancelled();
      await Future.any([_discover(), cancelled.future])
          .timeout(options.startupTimeout);
      cancellation?.throwIfCancelled();
    } catch (error) {
      await close();
      if (error is TurnCancelledException) rethrow;
      throw McpConnectionException(options.name, _failureKind(error));
    } finally {
      await subscription?.cancel();
    }
  }

  Future<void> _discover() async {
    await client.connect(transport);
    final seenCursors = <String>{};
    final seenNames = <String>{};
    final found = <({mcp.Tool tool, String description})>[];
    var catalogBytes = 0;
    String? cursor;
    do {
      if (_closed) throw StateError('closed');
      final page = await client.listTools(
        params: mcp.ListToolsRequest(cursor: cursor),
      );
      for (final tool in page.tools) {
        if (!seenNames.add(tool.name)) {
          throw const FormatException('duplicate tool');
        }
        if (tool.execution?.taskSupport == 'required') {
          log('task_tool_omitted');
          continue;
        }
        if (tool.inputSchema.toJson()['type'] != 'object') {
          throw const FormatException('invalid schema root');
        }
        // Catalog metadata reaches every provider request, so one server must
        // not be able to inflate it without bound.
        final schemaBytes = utf8
            .encode(jsonEncode(tool.inputSchema.toJson()))
            .length;
        if (schemaBytes > _maxToolSchemaBytes) {
          throw const FormatException('tool schema too large');
        }
        final description = boundedText(
          tool.description ?? '${options.name}: ${tool.name}',
          _maxToolDescriptionBytes,
          marker: '\n[MCP description truncated]',
        );
        // Count what the model would actually receive, not the rejected input.
        catalogBytes += schemaBytes + utf8.encode(description.text).length;
        if (catalogBytes > _maxCatalogBytes) {
          throw const FormatException('catalog too large');
        }
        found.add((tool: tool, description: description.text));
      }
      cursor = page.nextCursor;
      if (cursor != null && !seenCursors.add(cursor)) {
        throw const FormatException('repeated cursor');
      }
    } while (cursor != null);
    found.sort((a, b) => a.tool.name.compareTo(b.tool.name));
    tools.addAll(
      found.map((entry) => _McpTool(this, entry.tool, entry.description)),
    );
    client.frozen = true;
    if (client.getProtocolVersion() == '2026-07-28' &&
        client.getServerCapabilities()?.tools?.listChanged == true) {
      final subscription = client.listenSubscriptions(
        const mcp.SubscriptionsListenRequest(
          notifications: mcp.SubscriptionFilter(toolsListChanged: true),
        ),
      );
      _notifications = subscription.notifications.listen((notification) {
        if (notification.method == mcp.Method.notificationsToolsListChanged &&
            !_catalogNotice) {
          _catalogNotice = true;
          log('catalog_changed_restart_required');
        }
      }, onError: (Object _) => log('catalog_subscription_error'));
      unawaited(subscription.done.then<void>((_) {}, onError: (Object _) {}));
      await subscription.acknowledged;
    }
  }

  void _attachStderr(mcp.StdioClientTransport stdio) {
    _stderr = stdio.stderr?.listen((_) {
      if (!_stderrNotice) {
        _stderrNotice = true;
        log('server_stderr_omitted');
      }
    }, onError: (Object _) => log('stderr_error'));
  }

  /// The failure kind that makes this connection unusable, if any.
  String? get unusableReason => _httpState.abandoned
      ? 'connection_abandoned'
      : _closed
      ? 'connection_error'
      : null;

  /// Marks an HTTP connection unusable after a deadline or cancellation left a
  /// request in flight, then closes its remaining streams in the background.
  ///
  /// The SDK cannot abort that request, so every further call would add one
  /// more abandoned request; the server's late response frees the socket.
  bool abandonAfterDeadline() {
    if (options is! McpHttpOptions || _httpState.abandoned) return false;
    _httpState.abandoned = true;
    log('request_abandoned_connection_unusable');
    unawaited(close().catchError((Object _) {}));
    return true;
  }

  Future<void> close() => _closing ??= _close();

  Future<void> _close() async {
    _closed = true;
    try {
      try {
        await client.close().timeout(const Duration(seconds: 10));
      } finally {
        try {
          await transport.close().timeout(const Duration(seconds: 10));
        } finally {
          await _stderr?.cancel();
          await _notifications?.cancel();
        }
      }
    } catch (_) {
      log('cleanup_failed');
      throw McpConnectionException(options.name, 'cleanup_failed');
    }
  }
}

final class _McpTool(
  final _Connection connection,
  final mcp.Tool remote,
  final String description,
) implements Tool {
  this
    : descriptor = ToolDescriptor(
        name: _toolName(connection.options.name, remote.name),
        description: description,
        inputSchema: Map.unmodifiable(remote.inputSchema.toJson()),
      );

  @override
  final ToolDescriptor descriptor;

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    final abort = mcp.BasicAbortController();
    var finished = false;
    var timedOut = false;
    final timer = Timer(connection.options.callTimeout, () {
      timedOut = true;
      abort.abort();
    });
    final subscription = context.cancellation?.whenCancelled.asStream().listen(
      (_) => abort.abort(),
    );
    try {
      if (context.cancellation?.isCancelled ?? false) {
        throw const TurnCancelledException();
      }
      if (connection.unusableReason case final reason?) {
        throw McpConnectionException(connection.options.name, reason);
      }
      final result = await connection.client.callTool(
        mcp.CallToolRequest(name: remote.name, arguments: arguments),
        options: mcp.RequestOptions(
          signal: abort.signal,
          timeout: connection.options.callTimeout,
          maxTotalTimeout: connection.options.callTimeout,
          onprogress: (progress) {
            if (finished || abort.signal.aborted) return;
            final text =
                progress.message ??
                'Progress: ${progress.progress}${progress.total == null ? '' : '/${progress.total}'}';
            final bounded = boundedText(text, 4096);
            context.onOutput?.call(
              ToolOutputSnapshot(
                content: bounded.text,
                totalBytes: bounded.bytes,
                truncated: bounded.truncated,
              ),
            );
          },
        ),
      );
      return convertResult(result, connection.options.name, remote.name);
    } catch (error) {
      final kind = context.cancellation?.isCancelled == true
          ? 'cancelled'
          : timedOut
          ? 'timeout'
          : _failureKind(error);
      final uncertain = {
        'cancelled',
        'timeout',
        'connection_error',
        'session_lost',
      }.contains(kind);
      final abandoned =
          (kind == 'timeout' || kind == 'cancelled') &&
          connection.abandonAfterDeadline();
      final hint = switch (kind) {
        'connection_abandoned' => ' Restart Atlas to reconnect this server.',
        _ when abandoned =>
          ' The connection is now unusable; restart Atlas to reconnect. '
              'The remote outcome may be unknown; the call was not retried.',
        _ when uncertain =>
          ' The remote outcome may be unknown; the call was not retried.',
        _ => '',
      };
      return ToolResult(
        content: 'MCP tool failed: $kind.$hint',
        isError: true,
        metadata: {
          'mcp_server': connection.options.name,
          'mcp_tool': boundedText(remote.name, 512).text,
          'failure_kind': kind,
        },
      );
    } finally {
      finished = true;
      timer.cancel();
      await subscription?.cancel();
      abort.abort();
    }
  }
}

String _toolName(String server, String tool) {
  final prefix = 'mcp_${server}_$tool'.replaceAll(
    RegExp(r'[^A-Za-z0-9_-]'),
    '_',
  );
  final digest = sha256
      .convert(utf8.encode(jsonEncode([server, tool])))
      .toString()
      .substring(0, 16);
  return '${prefix.substring(0, prefix.length > 47 ? 47 : prefix.length)}_$digest';
}

String _failureKind(Object error) {
  if (error is TimeoutException) return 'timeout';
  if (error is FormatException || error is ArgumentError) {
    return 'invalid_catalog_or_result';
  }
  if (error is McpConnectionException) return error.kind;
  if (error is mcp.McpError) {
    // SDK 2.4.2 wraps HTTP status in a fixed message prefix with code zero.
    // Match only that prefix; the server-controlled body never enters diagnostics.
    if (error.code == 401 ||
        error.code == 403 ||
        (error.code == 0 &&
            RegExp(r'^Error POSTing to endpoint \(HTTP (401|403)\):')
                .hasMatch(error.message))) {
      return 'authentication_error';
    }
    if (error.code == mcp.ErrorCode.requestTimeout.value) return 'timeout';
    if (error.code == 0 || error.code == mcp.ErrorCode.connectionClosed.value) {
      return 'connection_error';
    }
    return 'protocol_error';
  }
  return 'connection_error';
}

// A frozen catalog cannot be silently replaced by the SDK's header-mismatch retry.
final class _CatalogClient() extends mcp.McpClient {
  this
    : super(
        const mcp.Implementation(name: 'atlas', version: '0.1.0'),
        options: mcp.McpClientOptions(protocol: mcp.McpProtocol.stable),
      );

  bool frozen = false;

  @override
  Future<mcp.ListToolsResult> listTools({
    mcp.ListToolsRequest? params,
    mcp.RequestOptions? options,
  }) {
    if (frozen) {
      throw const McpConnectionException(
        'catalog',
        'catalog_changed_restart_required',
      );
    }
    return super.listTools(params: params, options: options);
  }
}

// Keep all SDK HTTP capabilities while preventing automatic session replay.
final class _HttpTransport(
  super.url, {
  required final String server,
  required final _HttpState state,
  super.opts,
}) extends mcp.StreamableHttpClientTransport {
  this;

  bool _lost = false;

  @override
  Future<void> send(
    mcp.JsonRpcMessage message, {
    int? relatedRequestId,
    String? resumptionToken,
    void Function(String)? onResumptionToken,
  }) async {
    if (_lost) {
      throw McpConnectionException(server, 'session_lost');
    }
    if (state.abandoned) {
      throw McpConnectionException(server, 'connection_abandoned');
    }
    try {
      await super.send(
        message,
        relatedRequestId: relatedRequestId,
        resumptionToken: resumptionToken,
        onResumptionToken: onResumptionToken,
      );
    } on mcp.StaleSessionError {
      _lost = true;
      throw McpConnectionException(server, 'session_lost');
    }
  }
}

/// HTTP connection state shared by the wrapper and its transport.
final class _HttpState() {
  /// Creates the state for one HTTP connection.
  this;

  /// Whether a deadline left a request in flight on an unusable connection.
  bool abandoned = false;
}

// Drain captured stderr as soon as the SDK has opened its pipes.
final class _StdioTransport(
  super.serverParams,
  final void Function(mcp.StdioClientTransport) started,
) extends mcp.StdioClientTransport {
  this;

  @override
  Future<void> start() async {
    await super.start();
    started(this);
  }
}
