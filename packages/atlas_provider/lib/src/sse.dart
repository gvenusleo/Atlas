import 'dart:async';
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
///
/// A listen-forwarding controller makes subscription cancellation immediate.
Stream<SseEvent> decodeSse(Stream<List<int>> bytes) {
  final controller = StreamController<SseEvent>();
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

  late final StreamSubscription<String> subscription;
  subscription = utf8.decoder
      .bind(bytes)
      .transform(const LineSplitter())
      .listen(
        (line) {
          if (line.isEmpty) {
            if (data.isNotEmpty) {
              controller.add(SseEvent(name, data.join('\n')));
            }
            reset();
          } else {
            addLine(line);
          }
        },
        onError: (Object error, StackTrace stackTrace) {
          controller.addError(error, stackTrace);
        },
        onDone: () {
          if (data.isNotEmpty) {
            controller.add(SseEvent(name, data.join('\n')));
          }
          controller.close();
        },
      );
  controller.onCancel = () => subscription.cancel();
  return controller.stream;
}
