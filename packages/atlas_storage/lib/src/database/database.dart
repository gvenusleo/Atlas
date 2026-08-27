import 'dart:io';

import 'package:drift/drift.dart';
import 'package:drift/native.dart';

import 'tables.dart';

part 'database.g.dart';

/// The private Drift database used by [DriftSessionStore].
@DriftDatabase(tables: [Sessions, Turns, Messages])
final class AtlasDatabase extends _$AtlasDatabase {
  /// Creates a database with an injected Drift executor.
  AtlasDatabase(super.executor);

  /// Opens a native database file on a background isolate.
  factory AtlasDatabase.openFile(File file) =>
      AtlasDatabase(NativeDatabase.createInBackground(file));

  @override
  int get schemaVersion => 2;

  @override
  MigrationStrategy get migration => MigrationStrategy(
    onCreate: (migrator) => migrator.createAll(),
    onUpgrade: (migrator, from, to) async {
      if (from < 2) {
        await migrator.createIndex(turnsSessionStarted);
      }
    },
    beforeOpen: (details) async {
      await customStatement('PRAGMA foreign_keys = ON');
      // WAL permits concurrent readers while SQLite serializes writers.
      await customStatement('PRAGMA journal_mode = WAL');
      await customStatement('PRAGMA busy_timeout = 5000');
    },
  );
}
