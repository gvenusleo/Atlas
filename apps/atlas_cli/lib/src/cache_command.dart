import 'dart:io';

import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';

/// Parsed `atlas cache` options.
final class CacheOptions {
  /// Creates cache options.
  const CacheOptions({this.limit = 200});

  /// Maximum number of recent turns to sample.
  ///
  /// Every model request inside a sampled turn is counted, so the reported
  /// request count can exceed this limit.
  final int limit;
}

/// Parses `atlas cache` arguments.
///
/// Throws [FormatException] on unknown flags or malformed values.
CacheOptions parseCacheOptions(List<String> args) {
  var limit = 200;
  for (var i = 0; i < args.length; i++) {
    switch (args[i]) {
      case '--limit':
        if (i + 1 >= args.length) {
          throw const FormatException('--limit requires a turn count');
        }
        final value = int.tryParse(args[++i]);
        if (value == null || value <= 0) {
          throw const FormatException('--limit must be a positive number');
        }
        limit = value;
      default:
        throw FormatException('unknown option: ${args[i]}');
    }
  }
  return CacheOptions(limit: limit);
}

/// Prints a prompt-cache report for [store] and returns the process exit code.
///
/// The report follows the provider convention: every model request that
/// reports usage counts once, and the hit rate is cache-read tokens over the
/// request's whole prompt. Anthropic reports cached tokens on top of
/// `input_tokens` while OpenAI-compatible endpoints report them inside it, so
/// each request is normalized before it is added up.
///
/// Usage is read from every assistant item in the sampled turns, not from the
/// turn rows: a turn row only keeps the usage of its last model response, so
/// summing turns would count just the request that is already fully cached.
Future<int> runCacheCommand(
  DriftSessionStore store, {
  required AtlasConfig config,
  required List<String> args,
  StringSink? out,
}) async {
  final sink = out ?? stdout;
  final CacheOptions options;
  try {
    options = parseCacheOptions(args);
  } on FormatException catch (error) {
    sink.writeln('atlas cache: ${error.message}');
    sink.writeln('usage: atlas cache [--limit turns]');
    return 64;
  }

  final turns = await store.recentTurnUsage(limit: options.limit);
  sink.writeln('Atlas prompt cache report');
  sink.writeln();
  if (turns.isEmpty) {
    sink.writeln('No turns recorded yet (limit ${options.limit}).');
    return 0;
  }

  final anthropicProviders = <String>{
    for (final provider in config.providers)
      if (provider is ConfiguredAnthropic) provider.id.value,
  };
  final openAiProviders = <String>{
    for (final provider in config.providers)
      if (provider is ConfiguredOpenAI) provider.id.value,
  };

  final sampled = <String, _SampledSession>{};
  for (final turn in turns) {
    sampled
        .putIfAbsent(turn.sessionId, () => _SampledSession(title: turn.title))
        .providerByTurn[turn.turnId] = turn
        .providerId;
  }

  final overall = _Totals();
  final byProvider = <String, _Totals>{};
  final bySession = <String, _Totals>{};
  final notes = <String>{};
  var requests = 0;
  var requestsWithUsage = 0;
  var requestsWithRead = 0;
  var sessionsRead = 0;
  final unreadSessions = <String>[];
  for (final entry in sampled.entries) {
    final SessionSnapshot snapshot;
    try {
      snapshot = await store.loadSession(SessionId(entry.key));
    } on Object {
      // A live database can refuse a read or lose a session mid-run. The catch
      // stays generic on purpose: persistence errors surface as
      // implementation-specific types, so the failure is counted and reported
      // instead of being matched by type.
      unreadSessions.add(_shortSessionId(entry.key));
      continue;
    }
    sessionsRead++;
    for (final item in snapshot.timeline) {
      if (item is! AssistantMessageItem) {
        continue;
      }
      if (!entry.value.providerByTurn.containsKey(item.turnId.value)) {
        continue;
      }
      final providerId = entry.value.providerByTurn[item.turnId.value];
      final anthropic = _usesAnthropicAccounting(
        item.usage,
        providerId,
        anthropicProviders,
        openAiProviders,
        notes,
      );
      requests++;
      if (_reported(item.usage)) {
        requestsWithUsage++;
        if (item.usage.cacheReadInputTokens > 0) {
          requestsWithRead++;
        }
      }
      overall.add(item.usage, anthropic: anthropic);
      byProvider
          .putIfAbsent(providerId ?? '(unknown)', _Totals.new)
          .add(item.usage, anthropic: anthropic);
      bySession
          .putIfAbsent(entry.key, () => _Totals(title: entry.value.title))
          .add(item.usage, anthropic: anthropic);
    }
  }

  sink.writeln(
    'Sampled ${_count(requests, 'model request')} from '
    '${_count(turns.length, 'turn')} in ${_count(sessionsRead, 'session')} '
    '(newest first, limit ${options.limit} turns).',
  );
  sink.writeln(
    'Accounting: per model request; cached tokens are normalized out of '
    '`input_tokens` for OpenAI-compatible providers and added on top for '
    'Anthropic.',
  );
  sink.writeln();
  sink.writeln('Overall');
  _writeTotals(sink, overall);
  sink.writeln();
  sink.writeln('By provider');
  for (final entry in byProvider.entries) {
    final totals = entry.value;
    sink.writeln(
      '  ${entry.key.padRight(16)} '
      '${_count(totals.requests, 'request').padRight(15)} '
      'hit ${_rate(totals).padLeft(6)}  '
      'read ${_thousands(totals.cacheReadTokens).padLeft(12)}  '
      'write ${_thousands(totals.cacheWriteTokens).padLeft(9)}  '
      'fresh ${_thousands(totals.freshTokens).padLeft(12)}',
    );
  }
  sink.writeln();
  sink.writeln('Recent sessions');
  final listed = bySession.entries.take(8).toList();
  for (final entry in listed) {
    final totals = entry.value;
    sink.writeln(
      '  ${_shortSessionId(entry.key).padRight(10)} '
      '${_count(totals.requests, 'request').padRight(15)} '
      'hit ${_rate(totals).padLeft(6)}  '
      'read ${_thousands(totals.cacheReadTokens).padLeft(12)}  '
      'fresh ${_thousands(totals.freshTokens).padLeft(12)}  ${totals.title}',
    );
  }
  if (bySession.length > listed.length) {
    sink.writeln('  ... and ${bySession.length - listed.length} more sessions');
  }
  sink.writeln();
  sink.writeln('Assessment');
  for (final line in _assessment(
    overall,
    requestsWithUsage,
    requestsWithRead,
  )) {
    sink.writeln('  $line');
  }
  if (requests - requestsWithUsage > 0) {
    notes.add(
      '${_count(requests - requestsWithUsage, 'model request')} reported no '
      'token usage (cancelled or failed steps); they stay out of the '
      'request-level counts.',
    );
  }
  if (unreadSessions.isNotEmpty) {
    notes.add(
      '${_count(unreadSessions.length, 'session')} could not be read '
      '(${unreadSessions.join(', ')}); their requests are missing from the '
      'totals.',
    );
  }
  if (notes.isNotEmpty) {
    sink.writeln();
    sink.writeln('Notes');
    for (final note in notes) {
      sink.writeln('  $note');
    }
  }
  return 0;
}

/// One sampled session and the turns taken from it.
class _SampledSession {
  _SampledSession({required this.title});

  final String title;
  final Map<String, String?> providerByTurn = {};
}

/// Token totals for one group of model requests.
class _Totals {
  _Totals({this.title = ''});

  int requests = 0;
  int promptTokens = 0;
  int freshTokens = 0;
  int cacheReadTokens = 0;
  int cacheWriteTokens = 0;
  final String title;

  void add(TokenUsage usage, {required bool anthropic}) {
    requests++;
    cacheReadTokens += usage.cacheReadInputTokens;
    cacheWriteTokens += usage.cacheWriteInputTokens;
    if (anthropic) {
      // Anthropic bills cached tokens on top of `input_tokens`.
      freshTokens += usage.inputTokens;
      promptTokens +=
          usage.inputTokens +
          usage.cacheReadInputTokens +
          usage.cacheWriteInputTokens;
    } else {
      // OpenAI-compatible usage already counts cached tokens in `input_tokens`.
      final cached = usage.cacheReadInputTokens + usage.cacheWriteInputTokens;
      freshTokens += (usage.inputTokens - cached).clamp(0, usage.inputTokens);
      promptTokens += usage.inputTokens;
    }
  }

  /// Cache reads as a share of the group's normalized prompt tokens.
  ///
  /// Both branches keep the numerator inside the denominator, so the value can
  /// never exceed 1.
  double get hitRate => promptTokens == 0 ? 0 : cacheReadTokens / promptTokens;
}

/// Whether [usage] needs Anthropic accounting, given the configured providers.
///
/// A cache read can never exceed a prompt total that already contains the
/// cached tokens, so that shape wins over the configured provider type: Claude
/// behind an OpenAI-compatible endpoint reports `input_tokens` without the
/// cached tokens. Mixed reports append an entry to [notes].
bool _usesAnthropicAccounting(
  TokenUsage usage,
  String? providerId,
  Set<String> anthropicProviders,
  Set<String> openAiProviders,
  Set<String> notes,
) {
  if (usage.cacheReadInputTokens > usage.inputTokens) {
    if (providerId != null && openAiProviders.contains(providerId)) {
      notes.add(
        '$providerId reports cache reads outside input_tokens; using '
        'Anthropic-style accounting for it.',
      );
    }
    return true;
  }
  if (providerId != null) {
    if (anthropicProviders.contains(providerId)) {
      return true;
    }
    if (openAiProviders.contains(providerId)) {
      if (usage.cacheWriteInputTokens > 0) {
        notes.add(
          '$providerId reports cache writes while configured as '
          'OpenAI-compatible; confirm whether its input_tokens includes '
          'cached tokens.',
        );
      }
      return false;
    }
  }
  return usage.cacheWriteInputTokens > 0;
}

void _writeTotals(StringSink sink, _Totals totals) {
  sink.writeln('  prompt tokens  ${_thousands(totals.promptTokens)}');
  sink.writeln(
    '  cache read     ${_thousands(totals.cacheReadTokens).padRight(12)} '
    '${totals.promptTokens == 0 ? 'n/a' : '${_percent(totals.hitRate)} hit rate'}',
  );
  sink.writeln('  cache write    ${_thousands(totals.cacheWriteTokens)}');
  sink.writeln('  fresh input    ${_thousands(totals.freshTokens)}');
}

/// Hit rate for one line of the table, or `n/a` when nothing was reported.
String _rate(_Totals totals) =>
    totals.promptTokens == 0 ? 'n/a' : _percent(totals.hitRate);

/// Session identifier without its shared prefix, for compact table rows.
String _shortSessionId(String sessionId) {
  final bare = sessionId.startsWith('session-')
      ? sessionId.substring('session-'.length)
      : sessionId;
  return bare.length > 8 ? bare.substring(0, 8) : bare;
}

/// Whether a model request reported any token usage at all.
///
/// Cancelled and failed steps still persist an assistant item, but with an
/// empty usage record; they are not cache misses.
bool _reported(TokenUsage usage) =>
    usage.inputTokens > 0 ||
    usage.outputTokens > 0 ||
    usage.cacheReadInputTokens > 0 ||
    usage.cacheWriteInputTokens > 0;

List<String> _assessment(
  _Totals overall,
  int requestsWithUsage,
  int requestsWithRead,
) {
  if (overall.promptTokens == 0) {
    return const ['No prompt tokens were recorded for the sampled requests.'];
  }
  final lines = <String>[
    '${_percent(overall.hitRate)} of prompt tokens were served from cache.',
    'Cache reads appeared in $requestsWithRead of $requestsWithUsage model '
        'requests that reported usage.',
  ];
  if (overall.cacheReadTokens == 0 && overall.cacheWriteTokens == 0) {
    lines.add(
      'No cache activity: the endpoint may not report cache fields, or every '
      'request missed.',
    );
  } else if (overall.cacheReadTokens == 0) {
    lines.add(
      'Cache writes are never read back: the prompt prefix changes between '
      'requests, or the cache entry expired before it could be reused.',
    );
  } else if (overall.hitRate >= 0.6) {
    lines.add('Cache reuse looks healthy.');
  } else if (overall.hitRate >= 0.2) {
    lines.add(
      'Cache reuse is partial: compaction, model switches, or skill injection '
      'rewrite the prefix and invalidate what came before.',
    );
  } else {
    lines.add(
      'Cache reuse is low: for Anthropic, confirm requests carry cache '
      'breakpoints; OpenAI-compatible endpoints cache prefixes server-side and '
      'need a stable, long-enough prefix.',
    );
  }
  if (requestsWithUsage > 0 &&
      requestsWithRead < requestsWithUsage &&
      requestsWithRead > 0) {
    lines.add(
      'Requests without a cache read are the ones to inspect first: they are '
      'where the prefix broke or the cache entry was cold.',
    );
  }
  return lines;
}

String _thousands(int value) {
  final digits = value.abs().toString();
  final buffer = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) {
      buffer.write(',');
    }
    buffer.write(digits[i]);
  }
  return '${value < 0 ? '-' : ''}$buffer';
}

String _percent(double ratio) => '${(ratio * 100).toStringAsFixed(1)}%';

String _count(int value, String noun) => '$value $noun${value == 1 ? '' : 's'}';
