import 'dart:async';
import 'dart:convert';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:stream_channel/stream_channel.dart';

Future<String> createWireSession(Wire wire) async {
  final response = await wire.send({
    'jsonrpc': '2.0',
    'id': 1,
    'method': 'session/new',
    'params': {'cwd': '/tmp/project'},
  });
  return (response['result'] as Map<String, Object?>)['sessionId']! as String;
}

Future<void> runWirePrompt(Wire wire, String sessionId) async {
  final promptFuture = wire.send({
    'jsonrpc': '2.0',
    'id': 2,
    'method': 'session/prompt',
    'params': {
      'sessionId': sessionId,
      'prompt': [
        {'type': 'text', 'text': 'Inspect the files'},
      ],
    },
  });
  await wire.turnNotifications.take(5).toList();
  await promptFuture;
}

/// Awaits the latest `available_commands_update` notification for
/// [sessionId] and returns its command list.
Future<List<Map<String, Object?>>> wireAvailableCommands(
  Wire wire,
  String sessionId,
) async {
  final message = await wire.notifications.where((m) {
    final update = (m['params'] as Map?)?['update'];
    return update is Map &&
        update['sessionUpdate'] == 'available_commands_update' &&
        (m['params'] as Map)['sessionId'] == sessionId;
  }).first;
  final update = (message['params'] as Map)['update'] as Map<String, Object?>;
  return [
    for (final command in update['availableCommands'] as List<Object?>)
      (command as Map).cast<String, Object?>(),
  ];
}

final testModel = ModelRef(
  providerId: ProviderId('test'),
  modelId: ModelId('m'),
);
final testModel2 = ModelRef(
  providerId: ProviderId('test'),
  modelId: ModelId('m2'),
);

/// Builds a test skill with the given [name] and [description].
Skill testSkill(String name, String description) => Skill(
  name: name,
  description: description,
  dir: '/tmp/project/.atlas/skills/$name',
  path: '/tmp/project/.atlas/skills/$name/SKILL.md',
  content: 'Instructions for $name.',
);

/// A model catalog with one reasoning-capable model and one plain model.
final testCatalog = <ModelDescriptor>[
  ModelDescriptor(
    ref: testModel,
    name: 'Model One',
    description: 'Fast model',
    contextWindow: 128000,
    reasoningEfforts: const [
      ReasoningEffortOption(value: 'low', name: 'Low'),
      ReasoningEffortOption(value: 'high', name: 'High'),
    ],
  ),
  ModelDescriptor(ref: testModel2, name: 'Model Two', contextWindow: 64000),
];

/// The default scripted turn: one tool call followed by a final response.
List<ModelResponse> defaultWireResponses() => [
  ModelResponse(
    content: const [TextContent('I will inspect the files.')],
    toolCalls: [
      ToolCall(
        id: ToolCallId('call-1'),
        name: 'read',
        arguments: <String, Object?>{'path': '.'},
      ),
    ],
    stopReason: StopReason.toolUse,
  ),
  const ModelResponse(
    content: [TextContent('Done.')],
    stopReason: StopReason.endTurn,
  ),
];

/// A JSON-RPC wire harness driving an [AcpServer] over an in-memory channel.
final class Wire {
  Wire._(this._requests, this._provider);

  final StreamController<String> _requests;
  final ModelProvider _provider;
  final _pending = <Object?, Completer<Map<String, Object?>>>{};
  final _notifications = StreamController<Map<String, Object?>>.broadcast();
  final _clientRequests = StreamController<Map<String, Object?>>.broadcast();
  var _notificationCount = 0;
  var _clientRequestCount = 0;
  late final Future<void> _serverDone;

  static Future<Wire> open({
    bool blockingProvider = false,
    List<ModelResponse>? responses,
    Duration providerDelay = Duration.zero,
    int? delayedResponseIndex,
    List<ModelDescriptor> models = const [],
    List<Skill> skills = const [],
    int keptRecentTurns = 5,
    ToolRegistry? tools,
  }) async {
    final provider = blockingProvider
        ? BlockingProvider()
        : ScriptedProvider(
            responses ?? defaultWireResponses(),
            delay: providerDelay,
            delayedResponseIndex: delayedResponseIndex,
          );
    final runtime = AgentRuntime(
      store: MemorySessionStore(),
      provider: provider,
      tools: tools ?? MemoryTools(),
      ids: TestIds(),
      defaultModel: testModel,
      maxSteps: 2,
      keptRecentTurns: keptRecentTurns,
      sessionContextBuilder: (cwd) => SessionContext(
        workingDirectory: cwd,
        instructions: const [],
        skills: MemorySkillCatalog([
          for (final skill in skills)
            SkillSummary(
              name: skill.name,
              path: skill.path,
              description: skill.description,
            ),
        ], skills: skills),
      ),
    );
    final server = AcpServer(runtime, models: models);
    final requests = StreamController<String>();
    final outgoing = StreamController<String>();
    final wire = Wire._(requests, provider);
    outgoing.stream.listen((line) {
      final message = jsonDecode(line);
      if (message is Map) {
        final map = message.cast<String, Object?>();
        if (map['method'] != null && map.containsKey('id')) {
          // Agent-to-client requests (e.g. terminal/create) must be
          // answered by the harness before the agent proceeds.
          wire._clientRequestCount++;
          wire._clientRequests.add(map);
        } else if (map.containsKey('id')) {
          wire._pending.remove(map['id'])?.complete(map);
        } else {
          // Lifecycle notifications (e.g. available_commands_update) are
          // excluded from the turn notification count.
          final update = (map['params'] as Map?)?['update'];
          final isTurn =
              !(update is Map &&
                  update['sessionUpdate'] == 'available_commands_update');
          if (isTurn) {
            wire._notificationCount++;
          }
          wire._notifications.add(map);
        }
      }
    });
    wire._serverDone = server.serveChannel(
      StreamChannel<String>(requests.stream, outgoing.sink),
    );
    return wire;
  }

  /// Notifications received so far.
  Stream<Map<String, Object?>> get notifications => _notifications.stream;

  /// Turn notifications, excluding lifecycle notifications such as
  /// `available_commands_update` that arrive between requests.
  Stream<Map<String, Object?>> get turnNotifications =>
      _notifications.stream.where((message) {
        final update = (message['params'] as Map?)?['update'];
        return !(update is Map &&
            update['sessionUpdate'] == 'available_commands_update');
      });

  /// The number of notifications received so far.
  int get notificationCount => _notificationCount;

  /// Agent-to-client requests (e.g. `terminal/create`) awaiting a reply.
  Stream<Map<String, Object?>> get clientRequests => _clientRequests.stream;

  /// The number of agent-to-client requests received so far.
  int get clientRequestCount => _clientRequestCount;

  /// Answers an agent-to-client [request] with [result], or with a JSON-RPC
  /// [error] to exercise the client-failure path.
  void respondToRequest(
    Map<String, Object?> request, {
    Map<String, Object?>? result,
    Map<String, Object?>? error,
  }) {
    _requests.add(
      jsonEncode({
        'jsonrpc': '2.0',
        'id': request['id'],
        'result': ?result,
        'error': ?error,
      }),
    );
  }

  /// The model request of the most recent turn, when one was run.
  ModelRequest? get lastRequest => switch (_provider) {
    ScriptedProvider(:final lastRequest) => lastRequest,
    _ => null,
  };

  /// Sends a request and awaits its response.
  Future<Map<String, Object?>> send(Map<String, Object?> request) {
    final completer = Completer<Map<String, Object?>>();
    _pending[request['id']] = completer;
    _requests.add(jsonEncode(request));
    return completer.future;
  }

  /// Sends a notification without awaiting a response.
  void sendNotification(Map<String, Object?> notification) {
    _requests.add(jsonEncode(notification));
  }

  /// Closes the input (EOF) as a client would when shutting down.
  Future<void> closeInput() async {
    await _requests.close();
  }

  Future<void> close() async {
    await closeInput();
    await _serverDone;
    // Let deferred lifecycle notifications (available_commands_update) flush
    // before the broadcast controller closes.
    await Future<void>.delayed(Duration.zero);
    await _notifications.close();
  }
}

final class ScriptedProvider implements ModelProvider {
  ScriptedProvider(
    this.responses, {
    this.delay = Duration.zero,
    this.delayedResponseIndex,
  });

  final List<ModelResponse> responses;

  /// Artificial latency so a turn is still running when a test closes EOF.
  final Duration delay;

  /// Limits [delay] to one response index when set.
  final int? delayedResponseIndex;
  var _index = 0;

  /// The most recent model request, captured for assertions.
  ModelRequest? lastRequest;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async {
    // Model-aware so tests can observe which model the caller resolved.
    for (final descriptor in testCatalog) {
      if (descriptor.ref == model) {
        return descriptor;
      }
    }
    return ModelDescriptor(ref: model, name: 'test', contextWindow: 128000);
  }

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    lastRequest = request;
    final index = _index++;
    if (index >= responses.length) {
      throw const TurnCancelledException();
    }
    if (delay > Duration.zero &&
        (delayedResponseIndex == null || delayedResponseIndex == index)) {
      await Future.any<void>([
        Future<void>.delayed(delay),
        if (request.cancellation != null) request.cancellation!.whenCancelled,
      ]);
      request.cancellation?.throwIfCancelled();
    }
    final response = responses[index];
    for (final part in response.content) {
      if (part is TextContent) {
        yield TextDeltaEvent(part.text);
      }
    }
    yield ModelCompletedEvent(response);
  }
}

/// A provider that emits one delta and then waits for cancellation.
final class BlockingProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, name: 'test', contextWindow: 128000);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield const TextDeltaEvent('thinking');
    await request.cancellation!.whenCancelled;
    throw const TurnCancelledException();
  }
}

/// An in-memory skill catalog returning the injected summaries and skills.
final class MemorySkillCatalog implements SkillCatalog {
  MemorySkillCatalog(Iterable<SkillSummary> summaries, {this.skills = const []})
    : _summaries = List.unmodifiable(summaries);

  final List<Skill> skills;
  final List<SkillSummary> _summaries;

  @override
  List<SkillSummary> get summaries => _summaries;

  @override
  Skill? lookup(String name) {
    for (final skill in skills) {
      if (skill.name == name) {
        return skill;
      }
    }
    return null;
  }
}

final class MemoryTools implements ToolRegistry {
  @override
  List<ToolDescriptor> get descriptors => const [
    ToolDescriptor(
      name: 'read',
      description: 'Read a file',
      inputSchema: <String, Object?>{},
    ),
  ];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async =>
      const ToolResult(content: 'file list');
}

final class MemorySessionStore implements SessionStore {
  Session? session;
  final turns = <Turn>[];
  final timeline = <TimelineItem>[];
  final checkpoints = <ModelCheckpoint>[];

  @override
  Future<void> createSession(Session value) async => session = value;

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) async {
    final value = session;
    if (value == null || value.id != sessionId) {
      throw SessionNotFoundException(sessionId);
    }
    return SessionSnapshot(
      session: value,
      turns: List.unmodifiable(turns),
      timeline: List.unmodifiable(timeline),
      modelCheckpoints: List.unmodifiable(checkpoints),
    );
  }

  @override
  Future<SessionPage> listSessions(SessionQuery query) async => SessionPage(
    items: [
      if (session != null)
        SessionSummary(
          id: session!.id,
          title: session!.title,
          workingDirectory: session!.workingDirectory,
          additionalDirectories: session!.additionalDirectories,
          updatedAt: session!.updatedAt,
        ),
    ],
  );

  @override
  Future<void> beginTurn(BeginTurn operation) async {
    // Mirror DriftSessionStore: the title is auto-generated from the first
    // user message when the session has none yet.
    var value = operation.session;
    if (value.title.isEmpty) {
      final text = textFromContent(operation.userMessage.content).trim();
      if (text.isNotEmpty) {
        final firstLine = text.split('\n').first.trim();
        value = Session(
          id: value.id,
          title: String.fromCharCodes(firstLine.runes.take(80)),
          workingDirectory: value.workingDirectory,
          additionalDirectories: value.additionalDirectories,
          createdAt: value.createdAt,
          updatedAt: value.updatedAt,
          compaction: value.compaction,
          lastUsage: value.lastUsage,
        );
      }
    }
    session = value;
    turns.add(operation.turn);
    timeline.add(operation.userMessage);
  }

  @override
  Future<void> appendModelStep(
    SessionId sessionId,
    PersistedModelStep operation,
  ) async {
    timeline.add(operation.assistantMessage);
    timeline.addAll(operation.toolCalls);
    if (operation.checkpoint != null) checkpoints.add(operation.checkpoint!);
  }

  @override
  Future<void> appendToolResult(
    SessionId sessionId,
    ToolResultItem item,
  ) async => timeline.add(item);

  @override
  Future<void> finishTurn(SessionId sessionId, Turn turn) async {
    final index = turns.indexWhere((item) => item.id == turn.id);
    if (index < 0) {
      // Mirror DriftSessionStore: the turn row is gone, e.g. because the
      // session was deleted with ON DELETE CASCADE while the turn ran.
      throw SessionNotFoundException(sessionId);
    }
    turns[index] = turn;
  }

  @override
  Future<void> saveCompaction(
    SessionId sessionId,
    CompactionCheckpoint checkpoint,
  ) async {
    final value = session;
    if (value != null) {
      session = Session(
        id: value.id,
        title: value.title,
        workingDirectory: value.workingDirectory,
        additionalDirectories: value.additionalDirectories,
        createdAt: value.createdAt,
        updatedAt: value.updatedAt,
        compaction: checkpoint,
        lastUsage: value.lastUsage,
      );
    }
  }

  @override
  Future<void> deleteSession(SessionId sessionId) async {
    // Mirror DriftSessionStore: unknown sessions throw, and deleting a
    // session cascades to its turns, messages, and checkpoints.
    if (session == null || session!.id != sessionId) {
      throw SessionNotFoundException(sessionId);
    }
    session = null;
    turns.clear();
    timeline.clear();
    checkpoints.clear();
  }

  @override
  Future<void> renameSession(SessionId sessionId, String title) async {
    final value = session;
    if (value == null || value.id != sessionId) {
      throw SessionNotFoundException(sessionId);
    }
    session = Session(
      id: value.id,
      workingDirectory: value.workingDirectory,
      additionalDirectories: value.additionalDirectories,
      createdAt: value.createdAt,
      updatedAt: value.updatedAt,
      title: title,
      compaction: value.compaction,
      lastUsage: value.lastUsage,
    );
  }
}

final class TestIds implements IdGenerator {
  var _session = 0;
  var _turn = 0;
  var _item = 0;

  @override
  SessionId sessionId() => SessionId('session-${++_session}');

  @override
  TurnId turnId() => TurnId('turn-${++_turn}');

  @override
  TimelineItemId timelineItemId() => TimelineItemId('item-${++_item}');
}
