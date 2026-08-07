import 'dart:math';

import '../domain/ids.dart';

/// Supplies unique identifiers for runtime records.
abstract interface class IdGenerator {
  /// Creates a new session identifier.
  SessionId sessionId();

  /// Creates a new turn identifier.
  TurnId turnId();

  /// Creates a new timeline item identifier.
  TimelineItemId timelineItemId();
}

/// Generates timestamp-prefixed identifiers using the platform secure random source.
final class SecureIdGenerator implements IdGenerator {
  /// Creates a secure identifier generator.
  SecureIdGenerator({DateTime Function()? now, Random? random})
    : _now = now ?? DateTime.now,
      _random = random ?? Random.secure();

  final DateTime Function() _now;
  final Random _random;

  String _next(String prefix) {
    final timestamp = _now().toUtc().microsecondsSinceEpoch;
    final high = _random.nextInt(1 << 32).toRadixString(16).padLeft(8, '0');
    final low = _random.nextInt(1 << 32).toRadixString(16).padLeft(8, '0');
    return '$prefix-$timestamp-$high$low';
  }

  @override
  SessionId sessionId() => SessionId(_next('session'));

  @override
  TurnId turnId() => TurnId(_next('turn'));

  @override
  TimelineItemId timelineItemId() => TimelineItemId(_next('item'));
}
