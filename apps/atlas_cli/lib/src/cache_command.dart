import 'dart:io';

import 'package:args/args.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:io/io.dart' show ExitCode;

/// Parsed `atlas cache` options.
final class CacheOptions {
  /// Creates cache options.
  const CacheOptions({this.limit = 200});

  /// Validates parsed command options.
  factory CacheOptions.fromResults(ArgResults results) {
    if (results.rest.isNotEmpty) {
      throw const FormatException('cache does not take positional arguments');
    }
    final limit = int.tryParse(results.option('limit')!);
    if (limit == null || limit <= 0) {
      throw const FormatException('--limit must be a positive number');
    }
    return CacheOptions(limit: limit);
  }

  /// Maximum number of recent turns, including every recorded model response.
  final int limit;
}

/// Parses `atlas cache` arguments.
///
/// Throws [FormatException] on unknown flags or malformed values.
CacheOptions parseCacheOptions(List<String> args) {
  final parser = ArgParser();
  addCacheOptions(parser);
  return CacheOptions.fromResults(parser.parse(args));
}

/// Registers the prompt-cache report options.
void addCacheOptions(ArgParser parser) {
  parser.addOption(
    'limit',
    defaultsTo: '200',
    valueHelp: 'turns',
    help: 'Maximum number of recent turns to sample.',
  );
}

/// Prints recorded prompt-cache reuse without consulting provider configuration.
///
/// Token hit rate is the sum of cache reads divided by the sum of complete
/// prompt sizes, using only responses with known, valid accounting. Legacy,
/// unreported and aborted usage remains visible in the coverage counts.
/// [columns] overrides the detected terminal width for embedding and tests.
Future<int> runCacheCommand(
  DriftSessionStore store, {
  CacheOptions options = const CacheOptions(),
  StringSink? out,
  int? columns,
}) async {
  final sink = out ?? stdout;
  final width =
      columns ??
      (sink is Stdout && sink.hasTerminal ? sink.terminalColumns : 80);
  final writer = _ReportWriter(sink, width.clamp(1, 120));
  // Read before printing: a database failure must not look like a valid report.
  final turns = await store.recentTurnUsage(limit: options.limit);
  writer.line('Atlas | Prompt cache');
  writer.line();
  if (turns.isEmpty) {
    writer.line('No turns recorded yet (limit ${options.limit}).');
    return ExitCode.success.code;
  }

  final overall = _Totals();
  final byModel = <(String, String), _Totals>{};
  final bySession = <String, _Totals>{};
  final titles = <String, String>{};
  for (final turn in turns) {
    final session = bySession.putIfAbsent(turn.sessionId, _Totals.new);
    titles[turn.sessionId] = turn.title;
    for (final item in turn.requests) {
      overall.add(item);
      session.add(item);
      byModel
          .putIfAbsent((
            item.model.providerId.value,
            item.model.modelId.value,
          ), _Totals.new)
          .add(item);
    }
  }

  writer.line(
    'Latest ${_count(turns.length, 'turn')} | '
    '${_count(bySession.length, 'session')} | '
    '${_count(overall.requests, 'recorded request')}',
  );
  writer.line(
    'Turn starts (UTC): ${turns.last.startedAt.toUtc().toIso8601String()} '
    'to ${turns.first.startedAt.toUtc().toIso8601String()}',
  );
  writer.line();
  writer.metric(
    'Token hit rate',
    overall.rate,
    '${_number(overall.read)} / ${_number(overall.prompt)} measured input tokens',
  );
  writer.metric(
    'Requests with hits',
    _ratio(overall.hits, overall.measured),
    '${overall.hits} / ${overall.measured} measured requests',
  );
  writer.metric(
    'Cache-data coverage',
    _ratio(overall.measured, overall.requests),
    '${overall.measured} / ${overall.requests} recorded requests',
  );
  writer.line();
  writer.line('Measured input breakdown');
  writer.metric('Cache read', _number(overall.read));
  writer.metric(
    'Cache write',
    overall.measured == 0
        ? 'n/a'
        : '${_number(overall.write)}${overall.unknownWrites > 0 ? ' (known)' : ''}',
  );
  writer.metric(
    'Fresh input',
    overall.measured == 0
        ? 'n/a'
        : '${_number(overall.fresh)}${overall.unknownWrites > 0 ? ' (known)' : ''}',
  );
  if (overall.unknownWrites > 0) {
    writer.metric(
      'Unclassified input',
      _number(overall.unclassified),
      'fresh/write split unknown for ${_count(overall.unknownWrites, 'request')}',
    );
  }
  writer.metric('Total input', _number(overall.prompt));

  writer.line();
  writer.line('By provider / model');
  if (byModel.isEmpty) writer.line('  No recorded responses.');
  for (final entry in byModel.entries) {
    writer.group('${entry.key.$1} / ${entry.key.$2}', entry.value);
  }
  writer.line();
  writer.line('Recent sessions');
  for (final entry in bySession.entries.take(8)) {
    // Full identifiers remain available even when titles or names wrap.
    writer.group('${entry.key} | ${titles[entry.key]}', entry.value);
  }
  if (bySession.length > 8) {
    writer.line('  ${bySession.length - 8} more sessions included in totals.');
  }

  writer.line();
  writer.line('Notes');
  if (overall.unknown > 0) {
    writer.line(
      '  ${_count(overall.unknown, 'request')} excluded: cache fields '
      'or input accounting unknown (including legacy records).',
    );
  }
  if (overall.aborted > 0) {
    writer.line(
      '  ${_count(overall.aborted, 'aborted response')} excluded: '
      'final request usage unavailable; historical copied usage ignored.',
    );
  }
  if (overall.invalid > 0) {
    writer.line(
      '  ${_count(overall.invalid, 'request')} excluded: inconsistent '
      'token counts; no accounting convention was guessed.',
    );
  }
  if (overall.empty > 0) {
    writer.line(
      '  ${_count(overall.empty, 'request')} excluded: zero input tokens.',
    );
  }
  writer.line('  Rates cover measured requests only. Unknown is not a miss.');
  writer.line('  Cache writes are not hits; token reuse is not cost savings.');
  writer.line(
    '  Scope: recorded conversation responses, including compacted history.',
  );
  writer.line(
    '  Not included: summary calls or attempts without a response record.',
  );
  return ExitCode.success.code;
}

class _Totals {
  int requests = 0;
  int measured = 0;
  int hits = 0;
  int prompt = 0;
  int read = 0;
  int write = 0;
  int fresh = 0;
  int unclassified = 0;
  int unknownWrites = 0;
  int unknown = 0;
  int aborted = 0;
  int invalid = 0;
  int empty = 0;

  void add(AssistantMessageItem item) {
    requests++;
    if (item.stopReason == StopReason.aborted) {
      aborted++;
      return;
    }
    final usage = item.usage;
    final input = usage.promptTokens;
    if (input == null || !usage.cacheReadReported) {
      unknown++;
      return;
    }
    final cached = usage.cacheReadInputTokens;
    final written = usage.cacheWriteInputTokens;
    if (input < 0 ||
        cached < 0 ||
        cached > input ||
        (usage.cacheWriteReported &&
            (written < 0 || cached + written > input))) {
      invalid++;
      return;
    }
    if (input == 0) {
      empty++;
      return;
    }
    measured++;
    if (cached > 0) hits++;
    prompt += input;
    read += cached;
    if (usage.cacheWriteReported) {
      write += written;
      fresh += input - cached - written;
    } else {
      unknownWrites++;
      unclassified += input - cached;
    }
  }

  String get rate => _ratio(read, prompt);
}

// Plain text works unchanged with NO_COLOR, redirected stdout and TERM=dumb.
// Non-ASCII runes are conservatively budgeted at two cells to avoid splitting
// wide titles across the right edge. Control sequences never reach the sink.
class _ReportWriter {
  _ReportWriter(this.sink, this.columns);
  final StringSink sink;
  final int columns;

  void line([String text = '']) {
    final clean = text
        .replaceAll(RegExp(r'\x1b\[[0-?]*[ -/]*[@-~]'), '')
        .replaceAll(
          RegExp(r'[\x00-\x1f\x7f-\x9f\u2028\u2029\u202a-\u202e\u2066-\u2069]'),
          ' ',
        );
    var runes = clean.runes.toList();
    while (runes.isNotEmpty) {
      var end = 0;
      var width = 0;
      var lastSpace = -1;
      while (end < runes.length) {
        final size = runes[end] < 128 || columns == 1 ? 1 : 2;
        if (width + size > columns) break;
        if (runes[end] == 32 && width > 4) lastSpace = end;
        width += size;
        end++;
      }
      if (end < runes.length && lastSpace > 0) end = lastSpace;
      sink.writeln(
        String.fromCharCodes(
          runes
              .take(end)
              .map((rune) => columns == 1 && rune >= 128 ? 63 : rune),
        ).trimRight(),
      );
      runes = runes.skip(end).skipWhile((rune) => rune == 32).toList();
    }
    if (clean.isEmpty) sink.writeln();
  }

  void metric(String label, String value, [String detail = '']) {
    final primary = '  ${label.padRight(21)} ${value.padLeft(7)}';
    if (columns >= 72 && '$primary  $detail'.length <= columns) {
      line('$primary${detail.isEmpty ? '' : '  $detail'}');
    } else {
      line('  $label: $value');
      if (detail.isNotEmpty) line('    $detail');
    }
  }

  void group(String label, _Totals totals) {
    line('  $label');
    line(
      '    Token hit ${totals.rate} | measured '
      '${totals.measured}/${totals.requests} requests',
    );
    line('    Input ${_number(totals.prompt)} | read ${_number(totals.read)}');
  }
}

String _ratio(int numerator, int denominator) => denominator == 0
    ? 'n/a'
    : '${(numerator / denominator * 100).toStringAsFixed(1)}%';

String _number(int value) => value.toString().replaceAllMapped(
  RegExp(r'(\d)(?=(\d{3})+(?!\d))'),
  (match) => '${match[1]},',
);

String _count(int value, String noun) => '$value $noun${value == 1 ? '' : 's'}';
