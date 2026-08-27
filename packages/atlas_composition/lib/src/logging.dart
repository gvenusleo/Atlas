import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

/// Writes redacted structured events as JSON lines to a local file.
final class FileLogSink implements AtlasLogger {
  /// Creates a sink at [directory], creating it on first write.
  FileLogSink(
    this.directory, {
    this.minimumLevel = LogLevel.info,
    this.retainDays = 7,
  });

  /// Directory containing the log file.
  final Directory directory;

  /// Events below this level are ignored.
  final LogLevel minimumLevel;

  /// Number of daily files to retain after a write.
  final int retainDays;

  @override
  void log(LogEvent event) {
    if (event.level.index < minimumLevel.index) return;
    directory.createSync(recursive: true);
    final date = event.occurredAt.toUtc().toIso8601String().substring(0, 10);
    final file = File('${directory.path}/atlas-$date.log');
    file.writeAsStringSync(
      '${jsonEncode({'level': event.level.name, 'code': event.code, 'message': event.message, 'sessionId': event.sessionId?.value, 'turnId': event.turnId?.value, 'fields': event.fields, 'occurredAt': event.occurredAt.toUtc().toIso8601String()})}\n',
      mode: FileMode.append,
      flush: true,
    );
    _prune(date);
  }

  void _prune(String today) {
    final cutoff = DateTime.parse(
      today,
    ).subtract(Duration(days: retainDays - 1));
    for (final entity in directory.listSync()) {
      if (entity is! File) continue;
      final match = RegExp(
        r'atlas-(\d{4}-\d{2}-\d{2})\.log$',
      ).firstMatch(entity.path);
      if (match == null) continue;
      final date = DateTime.tryParse(match.group(1)!);
      if (date != null && date.isBefore(cutoff)) entity.deleteSync();
    }
  }
}
