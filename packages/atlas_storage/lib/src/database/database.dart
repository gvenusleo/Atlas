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
  int get schemaVersion => 1;

  @override
  MigrationStrategy get migration => MigrationStrategy(
    onCreate: (migrator) => migrator.createAll(),
    beforeOpen: (details) async {
      await customStatement('PRAGMA foreign_keys = ON');
    },
  );
}
