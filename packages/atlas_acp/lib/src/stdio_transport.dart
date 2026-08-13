import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:stream_channel/stream_channel.dart';

/// Builds the NDJSON stdio channel used by ACP: one JSON-RPC message per
/// line, read from [input] and written to [output].
///
/// ACP requires that stdout carries only protocol messages; logging must go
/// to stderr. The returned channel reads UTF-8 lines from [input] and writes
/// each emitted message followed by a newline to [output].
StreamChannel<String> ndjsonChannel(
  Stream<List<int>> input,
  StreamSink<String> output,
) => StreamChannel<String>(
  input
      .transform(utf8.decoder)
      .transform(const LineSplitter())
      .where((line) => line.isNotEmpty),
  output,
);

/// A [StreamSink] that writes each message as one line to [stdout].
///
/// json_rpc_2 encodes responses as JSON strings without literal newlines, so
/// appending a newline preserves the one-message-per-line framing.
final class StdoutLineSink implements StreamSink<String> {
  /// Creates a line sink over [stdout].
  StdoutLineSink(this.stdout);

  /// The destination stream.
  final IOSink stdout;

  @override
  void add(String data) => stdout.write('$data\n');

  @override
  Future<void> addStream(Stream<String> stream) async {
    await for (final data in stream) {
      add(data);
    }
  }

  @override
  Future<void> close() => stdout.close();

  @override
  Future<void> get done => stdout.done;

  @override
  void addError(Object error, [StackTrace? stackTrace]) {}
}
