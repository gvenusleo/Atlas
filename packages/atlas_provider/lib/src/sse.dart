import 'dart:convert';

/// One Server-Sent Events record.
final class SseEvent {
  /// Creates an event with an optional event name and payload.
  const SseEvent(this.name, this.data);

  /// The SSE event name, if supplied by the server.
  final String? name;

  /// The joined `data` fields.
  final String data;
}

/// Decodes an SSE byte stream while preserving UTF-8 and CRLF boundaries.
Stream<SseEvent> decodeSse(Stream<List<int>> bytes) async* {
  String? name;
  final data = <String>[];

  void reset() {
    name = null;
    data.clear();
  }

  void addLine(String line) {
    if (line.isEmpty) {
      return;
    }
    if (line.startsWith(':')) {
      return;
    }
    final separator = line.indexOf(':');
    final field = separator < 0 ? line : line.substring(0, separator);
    var value = separator < 0 ? '' : line.substring(separator + 1);
    if (value.startsWith(' ')) {
      value = value.substring(1);
    }
    if (field == 'event') {
      name = value;
    } else if (field == 'data') {
      data.add(value);
    }
  }

  await for (final line
      in utf8.decoder.bind(bytes).transform(const LineSplitter())) {
    if (line.isEmpty) {
      if (data.isNotEmpty) {
        yield SseEvent(name, data.join('\n'));
      }
      reset();
    } else {
      addLine(line);
    }
  }
  if (data.isNotEmpty) {
    yield SseEvent(name, data.join('\n'));
  }
}
