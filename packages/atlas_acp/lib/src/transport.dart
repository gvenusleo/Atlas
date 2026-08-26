import 'dart:async';
import 'dart:convert';

import 'package:acpd/acpd.dart';
import 'package:stream_channel/stream_channel.dart';

/// Builds the NDJSON string channel used by ACP: one JSON-RPC message per
/// line, read from [input] and written to [output].
///
/// Kept for tests and integrations that exchange raw NDJSON lines; channel
/// transports decode each line into an acpd [TransportFrame].
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

/// Adapts a [StreamChannel] of encoded transport frames into an acpd
/// [Transport].
///
/// The channel carries NDJSON lines: each incoming line is decoded into one
/// [TransportFrame] and each outgoing frame is serialized to a line. Used by
/// the in-process and test transports where a raw stdio channel is not
/// available.
final class ChannelTransport implements Transport {
  /// Creates a transport over [channel].
  ChannelTransport(
    this._channel, {
    this.onSend,
    void Function(String line)? onReceive,
  }) {
    _subscription = _channel.stream.listen(
      (line) {
        onReceive?.call(line);
        _incoming.add(TransportFrame.decode(line));
      },
      onError: _incoming.addError,
      onDone: _incoming.close,
    );
  }

  final StreamChannel<String> _channel;

  /// Called with each outgoing line before it is written.
  final void Function(String line)? onSend;
  final _incoming = StreamController<TransportFrame>.broadcast();
  late final StreamSubscription<String> _subscription;
  bool _closed = false;

  @override
  Stream<TransportFrame> get incoming => _incoming.stream;

  @override
  void send(TransportFrame frame) {
    if (_closed) {
      throw StateError('ChannelTransport is closed');
    }
    final line = frame.toWire();
    onSend?.call(line);
    _channel.sink.add(line);
  }

  @override
  Future<void> close() async {
    if (_closed) {
      return;
    }
    _closed = true;
    await _subscription.cancel();
    await _incoming.close();
    await _channel.sink.close();
  }
}

/// Defers the `done` event of an underlying [Transport] while tracked tasks
/// are still running.
///
/// acpd's [Connection] closes and cancels pending requests as soon as the
/// transport stream ends, which would drop responses still being computed
/// when the peer closes its input (EOF). Wrapping the incoming stream keeps
/// the connection alive until every in-flight request handler completes.
final class InFlightTransport implements Transport {
  /// Creates a wrapper over [inner].
  InFlightTransport(this._inner);

  final Transport _inner;
  final _controller = StreamController<TransportFrame>.broadcast();
  StreamSubscription<TransportFrame>? _sub;
  int _pending = 0;
  bool _doneSeen = false;
  bool _doneScheduled = false;

  /// Runs [action] while keeping the connection open.
  Future<T> run<T>(Future<T> Function() action) {
    _pending++;
    return Future<T>.sync(action).whenComplete(() {
      _pending--;
      _maybeDone();
    });
  }

  @override
  Stream<TransportFrame> get incoming {
    _sub ??= _inner.incoming.listen(
      _controller.add,
      onError: _controller.addError,
      onDone: () {
        _doneSeen = true;
        _maybeDone();
      },
    );
    return _controller.stream;
  }

  @override
  void send(TransportFrame frame) => _inner.send(frame);

  @override
  Future<void> close() async {
    await _sub?.cancel();
    await _controller.close();
    await _inner.close();
  }

  void _maybeDone() {
    if (!_doneSeen || _pending != 0 || _doneScheduled) {
      return;
    }
    _doneScheduled = true;
    // Defer past the current microtask so a finishing handler's response is
    // written before the connection observes the closed input stream.
    scheduleMicrotask(() {
      if (!_controller.isClosed) {
        _controller.close();
      }
    });
  }
}

/// A two-sided in-memory transport pair backed by broadcast channels.
///
/// Both sides exchange [TransportFrame]s without any serialization, so an
/// agent and a client can run in the same process with full protocol
/// semantics. This is the transport used when Flutter connects to the
/// in-process Atlas agent.
final class MemoryTransportPair {
  /// Creates the pair.
  MemoryTransportPair() {
    left = _MemoryTransport('client', _rightToLeft.sink, _leftToRight.stream);
    right = _MemoryTransport('agent', _leftToRight.sink, _rightToLeft.stream);
  }

  /// The client-side transport.
  late final Transport left;

  /// The agent-side transport.
  late final Transport right;

  final _leftToRight = StreamController<TransportFrame>.broadcast();
  final _rightToLeft = StreamController<TransportFrame>.broadcast();

  /// Closes both sides.
  Future<void> close() async {
    await left.close();
    await right.close();
  }
}

class _MemoryTransport implements Transport {
  _MemoryTransport(this.name, this._sink, this._source);

  final String name;
  final StreamSink<TransportFrame> _sink;
  final Stream<TransportFrame> _source;
  final _controller = StreamController<TransportFrame>.broadcast();
  StreamSubscription<TransportFrame>? _sub;
  bool _closed = false;

  @override
  Stream<TransportFrame> get incoming {
    _sub ??= _source.listen(
      _controller.add,
      onError: _controller.addError,
      onDone: _controller.close,
    );
    return _controller.stream;
  }

  @override
  void send(TransportFrame frame) {
    if (_closed) {
      throw StateError('Transport "$name" is closed');
    }
    _sink.add(frame);
  }

  @override
  Future<void> close() async {
    if (_closed) {
      return;
    }
    _closed = true;
    await _sub?.cancel();
    if (!_controller.isClosed) {
      await _controller.close();
    }
    // Closing the outbound sink ends the peer's incoming stream so the
    // paired connection observes EOF instead of hanging on `closed`.
    try {
      await _sink.close();
    } on StateError {
      // The peer already closed this direction.
    }
  }
}
