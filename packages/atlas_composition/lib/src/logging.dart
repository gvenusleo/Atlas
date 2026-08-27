import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

/// Writes redacted structured events as JSON lines to a local file.
final class FileLogSink implements AtlasLogger {
  /// Creates a sink at [directory], creating it on first write.
  FileLogSink(this.directory);

  /// Directory containing the log file.
  final Directory directory;

  @override
  void log(LogEvent event) {
    directory.createSync(recursive: true);
    final date = event.occurredAt.toUtc().toIso8601String().substring(0, 10);
    final file = File('${directory.path}/atlas-$date.log');
    file.writeAsStringSync(
      '${jsonEncode({'level': event.level.name, 'code': event.code, 'message': event.message, 'sessionId': event.sessionId?.value, 'turnId': event.turnId?.value, 'fields': event.fields, 'occurredAt': event.occurredAt.toUtc().toIso8601String()})}\n',
      mode: FileMode.append,
      flush: true,
    );
  }
}
