/// Persistence adapter for a connection list.
abstract interface class ConnectionStore<T> {
  /// Loads the saved connections.
  Future<List<T>> load();

  /// Replaces the saved connections.
  Future<void> save(List<T> connections);
}

/// Owns an immutable connection list and serializes read-modify-write updates.
final class ConnectionRepository<T> {
  /// Creates a repository backed by [store].
  ConnectionRepository(ConnectionStore<T> store) : _store = store;

  final ConnectionStore<T> _store;
  List<T>? _cached;
  Future<void> _pending = Future.value();

  /// Loads the shared snapshot, reading storage only on the first access.
  Future<List<T>> load() => _serialize(_load);

  Future<List<T>> _load() async =>
      _cached ??= List<T>.unmodifiable(await _store.load());

  /// Commits [transform] against the latest snapshot before publishing it.
  ///
  /// Failed writes leave the previous snapshot intact and do not block later
  /// updates. The transform receives a mutable copy, never the shared list.
  Future<List<T>> update(void Function(List<T>) transform) =>
      _serialize(() async {
        final items = List<T>.of(await _load());
        transform(items);
        final snapshot = List<T>.unmodifiable(items);
        await _store.save(snapshot);
        return _cached = snapshot;
      });

  Future<List<T>> _serialize(Future<List<T>> Function() operation) {
    final result = _pending.then((_) => operation());
    _pending = result.then<void>((_) {}, onError: (Object _, StackTrace _) {});
    return result;
  }
}
