// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'database.dart';

// ignore_for_file: type=lint
class $SessionsTable extends Sessions
    with TableInfo<$SessionsTable, SessionRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $SessionsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _titleMeta = const VerificationMeta('title');
  @override
  late final GeneratedColumn<String> title = GeneratedColumn<String>(
    'title',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
    defaultValue: const Constant(''),
  );
  static const VerificationMeta _workingDirectoryMeta = const VerificationMeta(
    'workingDirectory',
  );
  @override
  late final GeneratedColumn<String> workingDirectory = GeneratedColumn<String>(
    'working_directory',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _additionalDirectoriesJsonMeta =
      const VerificationMeta('additionalDirectoriesJson');
  @override
  late final GeneratedColumn<String> additionalDirectoriesJson =
      GeneratedColumn<String>(
        'additional_directories_json',
        aliasedName,
        false,
        type: DriftSqlType.string,
        requiredDuringInsert: false,
        defaultValue: const Constant('[]'),
      );
  static const VerificationMeta _lastInputTokensMeta = const VerificationMeta(
    'lastInputTokens',
  );
  @override
  late final GeneratedColumn<int> lastInputTokens = GeneratedColumn<int>(
    'last_input_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _lastOutputTokensMeta = const VerificationMeta(
    'lastOutputTokens',
  );
  @override
  late final GeneratedColumn<int> lastOutputTokens = GeneratedColumn<int>(
    'last_output_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _lastTotalTokensMeta = const VerificationMeta(
    'lastTotalTokens',
  );
  @override
  late final GeneratedColumn<int> lastTotalTokens = GeneratedColumn<int>(
    'last_total_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _lastCacheReadTokensMeta =
      const VerificationMeta('lastCacheReadTokens');
  @override
  late final GeneratedColumn<int> lastCacheReadTokens = GeneratedColumn<int>(
    'last_cache_read_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _lastCacheWriteTokensMeta =
      const VerificationMeta('lastCacheWriteTokens');
  @override
  late final GeneratedColumn<int> lastCacheWriteTokens = GeneratedColumn<int>(
    'last_cache_write_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _createdAtMeta = const VerificationMeta(
    'createdAt',
  );
  @override
  late final GeneratedColumn<DateTime> createdAt = GeneratedColumn<DateTime>(
    'created_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _updatedAtMeta = const VerificationMeta(
    'updatedAt',
  );
  @override
  late final GeneratedColumn<DateTime> updatedAt = GeneratedColumn<DateTime>(
    'updated_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    title,
    workingDirectory,
    additionalDirectoriesJson,
    lastInputTokens,
    lastOutputTokens,
    lastTotalTokens,
    lastCacheReadTokens,
    lastCacheWriteTokens,
    createdAt,
    updatedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'sessions';
  @override
  VerificationContext validateIntegrity(
    Insertable<SessionRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('title')) {
      context.handle(
        _titleMeta,
        title.isAcceptableOrUnknown(data['title']!, _titleMeta),
      );
    }
    if (data.containsKey('working_directory')) {
      context.handle(
        _workingDirectoryMeta,
        workingDirectory.isAcceptableOrUnknown(
          data['working_directory']!,
          _workingDirectoryMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_workingDirectoryMeta);
    }
    if (data.containsKey('additional_directories_json')) {
      context.handle(
        _additionalDirectoriesJsonMeta,
        additionalDirectoriesJson.isAcceptableOrUnknown(
          data['additional_directories_json']!,
          _additionalDirectoriesJsonMeta,
        ),
      );
    }
    if (data.containsKey('last_input_tokens')) {
      context.handle(
        _lastInputTokensMeta,
        lastInputTokens.isAcceptableOrUnknown(
          data['last_input_tokens']!,
          _lastInputTokensMeta,
        ),
      );
    }
    if (data.containsKey('last_output_tokens')) {
      context.handle(
        _lastOutputTokensMeta,
        lastOutputTokens.isAcceptableOrUnknown(
          data['last_output_tokens']!,
          _lastOutputTokensMeta,
        ),
      );
    }
    if (data.containsKey('last_total_tokens')) {
      context.handle(
        _lastTotalTokensMeta,
        lastTotalTokens.isAcceptableOrUnknown(
          data['last_total_tokens']!,
          _lastTotalTokensMeta,
        ),
      );
    }
    if (data.containsKey('last_cache_read_tokens')) {
      context.handle(
        _lastCacheReadTokensMeta,
        lastCacheReadTokens.isAcceptableOrUnknown(
          data['last_cache_read_tokens']!,
          _lastCacheReadTokensMeta,
        ),
      );
    }
    if (data.containsKey('last_cache_write_tokens')) {
      context.handle(
        _lastCacheWriteTokensMeta,
        lastCacheWriteTokens.isAcceptableOrUnknown(
          data['last_cache_write_tokens']!,
          _lastCacheWriteTokensMeta,
        ),
      );
    }
    if (data.containsKey('created_at')) {
      context.handle(
        _createdAtMeta,
        createdAt.isAcceptableOrUnknown(data['created_at']!, _createdAtMeta),
      );
    } else if (isInserting) {
      context.missing(_createdAtMeta);
    }
    if (data.containsKey('updated_at')) {
      context.handle(
        _updatedAtMeta,
        updatedAt.isAcceptableOrUnknown(data['updated_at']!, _updatedAtMeta),
      );
    } else if (isInserting) {
      context.missing(_updatedAtMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  SessionRow map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return SessionRow(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      title: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}title'],
      )!,
      workingDirectory: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}working_directory'],
      )!,
      additionalDirectoriesJson: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}additional_directories_json'],
      )!,
      lastInputTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}last_input_tokens'],
      )!,
      lastOutputTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}last_output_tokens'],
      )!,
      lastTotalTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}last_total_tokens'],
      )!,
      lastCacheReadTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}last_cache_read_tokens'],
      )!,
      lastCacheWriteTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}last_cache_write_tokens'],
      )!,
      createdAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}created_at'],
      )!,
      updatedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}updated_at'],
      )!,
    );
  }

  @override
  $SessionsTable createAlias(String alias) {
    return $SessionsTable(attachedDatabase, alias);
  }
}

class SessionRow extends DataClass implements Insertable<SessionRow> {
  /// Serialized session identifier.
  final String id;

  /// User-facing session title.
  final String title;

  /// Primary tool working directory.
  final String workingDirectory;

  /// JSON-encoded additional tool roots.
  final String additionalDirectoriesJson;

  /// Input tokens from the latest model response.
  final int lastInputTokens;

  /// Output tokens from the latest model response.
  final int lastOutputTokens;

  /// Total tokens from the latest model response.
  final int lastTotalTokens;

  /// Cached input tokens read by the latest model response.
  final int lastCacheReadTokens;

  /// Cached input tokens written by the latest model response.
  final int lastCacheWriteTokens;

  /// UTC creation time.
  final DateTime createdAt;

  /// UTC last-update time.
  final DateTime updatedAt;
  const SessionRow({
    required this.id,
    required this.title,
    required this.workingDirectory,
    required this.additionalDirectoriesJson,
    required this.lastInputTokens,
    required this.lastOutputTokens,
    required this.lastTotalTokens,
    required this.lastCacheReadTokens,
    required this.lastCacheWriteTokens,
    required this.createdAt,
    required this.updatedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    map['title'] = Variable<String>(title);
    map['working_directory'] = Variable<String>(workingDirectory);
    map['additional_directories_json'] = Variable<String>(
      additionalDirectoriesJson,
    );
    map['last_input_tokens'] = Variable<int>(lastInputTokens);
    map['last_output_tokens'] = Variable<int>(lastOutputTokens);
    map['last_total_tokens'] = Variable<int>(lastTotalTokens);
    map['last_cache_read_tokens'] = Variable<int>(lastCacheReadTokens);
    map['last_cache_write_tokens'] = Variable<int>(lastCacheWriteTokens);
    map['created_at'] = Variable<DateTime>(createdAt);
    map['updated_at'] = Variable<DateTime>(updatedAt);
    return map;
  }

  SessionsCompanion toCompanion(bool nullToAbsent) {
    return SessionsCompanion(
      id: Value(id),
      title: Value(title),
      workingDirectory: Value(workingDirectory),
      additionalDirectoriesJson: Value(additionalDirectoriesJson),
      lastInputTokens: Value(lastInputTokens),
      lastOutputTokens: Value(lastOutputTokens),
      lastTotalTokens: Value(lastTotalTokens),
      lastCacheReadTokens: Value(lastCacheReadTokens),
      lastCacheWriteTokens: Value(lastCacheWriteTokens),
      createdAt: Value(createdAt),
      updatedAt: Value(updatedAt),
    );
  }

  factory SessionRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return SessionRow(
      id: serializer.fromJson<String>(json['id']),
      title: serializer.fromJson<String>(json['title']),
      workingDirectory: serializer.fromJson<String>(json['workingDirectory']),
      additionalDirectoriesJson: serializer.fromJson<String>(
        json['additionalDirectoriesJson'],
      ),
      lastInputTokens: serializer.fromJson<int>(json['lastInputTokens']),
      lastOutputTokens: serializer.fromJson<int>(json['lastOutputTokens']),
      lastTotalTokens: serializer.fromJson<int>(json['lastTotalTokens']),
      lastCacheReadTokens: serializer.fromJson<int>(
        json['lastCacheReadTokens'],
      ),
      lastCacheWriteTokens: serializer.fromJson<int>(
        json['lastCacheWriteTokens'],
      ),
      createdAt: serializer.fromJson<DateTime>(json['createdAt']),
      updatedAt: serializer.fromJson<DateTime>(json['updatedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'title': serializer.toJson<String>(title),
      'workingDirectory': serializer.toJson<String>(workingDirectory),
      'additionalDirectoriesJson': serializer.toJson<String>(
        additionalDirectoriesJson,
      ),
      'lastInputTokens': serializer.toJson<int>(lastInputTokens),
      'lastOutputTokens': serializer.toJson<int>(lastOutputTokens),
      'lastTotalTokens': serializer.toJson<int>(lastTotalTokens),
      'lastCacheReadTokens': serializer.toJson<int>(lastCacheReadTokens),
      'lastCacheWriteTokens': serializer.toJson<int>(lastCacheWriteTokens),
      'createdAt': serializer.toJson<DateTime>(createdAt),
      'updatedAt': serializer.toJson<DateTime>(updatedAt),
    };
  }

  SessionRow copyWith({
    String? id,
    String? title,
    String? workingDirectory,
    String? additionalDirectoriesJson,
    int? lastInputTokens,
    int? lastOutputTokens,
    int? lastTotalTokens,
    int? lastCacheReadTokens,
    int? lastCacheWriteTokens,
    DateTime? createdAt,
    DateTime? updatedAt,
  }) => SessionRow(
    id: id ?? this.id,
    title: title ?? this.title,
    workingDirectory: workingDirectory ?? this.workingDirectory,
    additionalDirectoriesJson:
        additionalDirectoriesJson ?? this.additionalDirectoriesJson,
    lastInputTokens: lastInputTokens ?? this.lastInputTokens,
    lastOutputTokens: lastOutputTokens ?? this.lastOutputTokens,
    lastTotalTokens: lastTotalTokens ?? this.lastTotalTokens,
    lastCacheReadTokens: lastCacheReadTokens ?? this.lastCacheReadTokens,
    lastCacheWriteTokens: lastCacheWriteTokens ?? this.lastCacheWriteTokens,
    createdAt: createdAt ?? this.createdAt,
    updatedAt: updatedAt ?? this.updatedAt,
  );
  SessionRow copyWithCompanion(SessionsCompanion data) {
    return SessionRow(
      id: data.id.present ? data.id.value : this.id,
      title: data.title.present ? data.title.value : this.title,
      workingDirectory: data.workingDirectory.present
          ? data.workingDirectory.value
          : this.workingDirectory,
      additionalDirectoriesJson: data.additionalDirectoriesJson.present
          ? data.additionalDirectoriesJson.value
          : this.additionalDirectoriesJson,
      lastInputTokens: data.lastInputTokens.present
          ? data.lastInputTokens.value
          : this.lastInputTokens,
      lastOutputTokens: data.lastOutputTokens.present
          ? data.lastOutputTokens.value
          : this.lastOutputTokens,
      lastTotalTokens: data.lastTotalTokens.present
          ? data.lastTotalTokens.value
          : this.lastTotalTokens,
      lastCacheReadTokens: data.lastCacheReadTokens.present
          ? data.lastCacheReadTokens.value
          : this.lastCacheReadTokens,
      lastCacheWriteTokens: data.lastCacheWriteTokens.present
          ? data.lastCacheWriteTokens.value
          : this.lastCacheWriteTokens,
      createdAt: data.createdAt.present ? data.createdAt.value : this.createdAt,
      updatedAt: data.updatedAt.present ? data.updatedAt.value : this.updatedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('SessionRow(')
          ..write('id: $id, ')
          ..write('title: $title, ')
          ..write('workingDirectory: $workingDirectory, ')
          ..write('additionalDirectoriesJson: $additionalDirectoriesJson, ')
          ..write('lastInputTokens: $lastInputTokens, ')
          ..write('lastOutputTokens: $lastOutputTokens, ')
          ..write('lastTotalTokens: $lastTotalTokens, ')
          ..write('lastCacheReadTokens: $lastCacheReadTokens, ')
          ..write('lastCacheWriteTokens: $lastCacheWriteTokens, ')
          ..write('createdAt: $createdAt, ')
          ..write('updatedAt: $updatedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    id,
    title,
    workingDirectory,
    additionalDirectoriesJson,
    lastInputTokens,
    lastOutputTokens,
    lastTotalTokens,
    lastCacheReadTokens,
    lastCacheWriteTokens,
    createdAt,
    updatedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is SessionRow &&
          other.id == this.id &&
          other.title == this.title &&
          other.workingDirectory == this.workingDirectory &&
          other.additionalDirectoriesJson == this.additionalDirectoriesJson &&
          other.lastInputTokens == this.lastInputTokens &&
          other.lastOutputTokens == this.lastOutputTokens &&
          other.lastTotalTokens == this.lastTotalTokens &&
          other.lastCacheReadTokens == this.lastCacheReadTokens &&
          other.lastCacheWriteTokens == this.lastCacheWriteTokens &&
          other.createdAt == this.createdAt &&
          other.updatedAt == this.updatedAt);
}

class SessionsCompanion extends UpdateCompanion<SessionRow> {
  final Value<String> id;
  final Value<String> title;
  final Value<String> workingDirectory;
  final Value<String> additionalDirectoriesJson;
  final Value<int> lastInputTokens;
  final Value<int> lastOutputTokens;
  final Value<int> lastTotalTokens;
  final Value<int> lastCacheReadTokens;
  final Value<int> lastCacheWriteTokens;
  final Value<DateTime> createdAt;
  final Value<DateTime> updatedAt;
  final Value<int> rowid;
  const SessionsCompanion({
    this.id = const Value.absent(),
    this.title = const Value.absent(),
    this.workingDirectory = const Value.absent(),
    this.additionalDirectoriesJson = const Value.absent(),
    this.lastInputTokens = const Value.absent(),
    this.lastOutputTokens = const Value.absent(),
    this.lastTotalTokens = const Value.absent(),
    this.lastCacheReadTokens = const Value.absent(),
    this.lastCacheWriteTokens = const Value.absent(),
    this.createdAt = const Value.absent(),
    this.updatedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  SessionsCompanion.insert({
    required String id,
    this.title = const Value.absent(),
    required String workingDirectory,
    this.additionalDirectoriesJson = const Value.absent(),
    this.lastInputTokens = const Value.absent(),
    this.lastOutputTokens = const Value.absent(),
    this.lastTotalTokens = const Value.absent(),
    this.lastCacheReadTokens = const Value.absent(),
    this.lastCacheWriteTokens = const Value.absent(),
    required DateTime createdAt,
    required DateTime updatedAt,
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       workingDirectory = Value(workingDirectory),
       createdAt = Value(createdAt),
       updatedAt = Value(updatedAt);
  static Insertable<SessionRow> custom({
    Expression<String>? id,
    Expression<String>? title,
    Expression<String>? workingDirectory,
    Expression<String>? additionalDirectoriesJson,
    Expression<int>? lastInputTokens,
    Expression<int>? lastOutputTokens,
    Expression<int>? lastTotalTokens,
    Expression<int>? lastCacheReadTokens,
    Expression<int>? lastCacheWriteTokens,
    Expression<DateTime>? createdAt,
    Expression<DateTime>? updatedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (title != null) 'title': title,
      if (workingDirectory != null) 'working_directory': workingDirectory,
      if (additionalDirectoriesJson != null)
        'additional_directories_json': additionalDirectoriesJson,
      if (lastInputTokens != null) 'last_input_tokens': lastInputTokens,
      if (lastOutputTokens != null) 'last_output_tokens': lastOutputTokens,
      if (lastTotalTokens != null) 'last_total_tokens': lastTotalTokens,
      if (lastCacheReadTokens != null)
        'last_cache_read_tokens': lastCacheReadTokens,
      if (lastCacheWriteTokens != null)
        'last_cache_write_tokens': lastCacheWriteTokens,
      if (createdAt != null) 'created_at': createdAt,
      if (updatedAt != null) 'updated_at': updatedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  SessionsCompanion copyWith({
    Value<String>? id,
    Value<String>? title,
    Value<String>? workingDirectory,
    Value<String>? additionalDirectoriesJson,
    Value<int>? lastInputTokens,
    Value<int>? lastOutputTokens,
    Value<int>? lastTotalTokens,
    Value<int>? lastCacheReadTokens,
    Value<int>? lastCacheWriteTokens,
    Value<DateTime>? createdAt,
    Value<DateTime>? updatedAt,
    Value<int>? rowid,
  }) {
    return SessionsCompanion(
      id: id ?? this.id,
      title: title ?? this.title,
      workingDirectory: workingDirectory ?? this.workingDirectory,
      additionalDirectoriesJson:
          additionalDirectoriesJson ?? this.additionalDirectoriesJson,
      lastInputTokens: lastInputTokens ?? this.lastInputTokens,
      lastOutputTokens: lastOutputTokens ?? this.lastOutputTokens,
      lastTotalTokens: lastTotalTokens ?? this.lastTotalTokens,
      lastCacheReadTokens: lastCacheReadTokens ?? this.lastCacheReadTokens,
      lastCacheWriteTokens: lastCacheWriteTokens ?? this.lastCacheWriteTokens,
      createdAt: createdAt ?? this.createdAt,
      updatedAt: updatedAt ?? this.updatedAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (title.present) {
      map['title'] = Variable<String>(title.value);
    }
    if (workingDirectory.present) {
      map['working_directory'] = Variable<String>(workingDirectory.value);
    }
    if (additionalDirectoriesJson.present) {
      map['additional_directories_json'] = Variable<String>(
        additionalDirectoriesJson.value,
      );
    }
    if (lastInputTokens.present) {
      map['last_input_tokens'] = Variable<int>(lastInputTokens.value);
    }
    if (lastOutputTokens.present) {
      map['last_output_tokens'] = Variable<int>(lastOutputTokens.value);
    }
    if (lastTotalTokens.present) {
      map['last_total_tokens'] = Variable<int>(lastTotalTokens.value);
    }
    if (lastCacheReadTokens.present) {
      map['last_cache_read_tokens'] = Variable<int>(lastCacheReadTokens.value);
    }
    if (lastCacheWriteTokens.present) {
      map['last_cache_write_tokens'] = Variable<int>(
        lastCacheWriteTokens.value,
      );
    }
    if (createdAt.present) {
      map['created_at'] = Variable<DateTime>(createdAt.value);
    }
    if (updatedAt.present) {
      map['updated_at'] = Variable<DateTime>(updatedAt.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('SessionsCompanion(')
          ..write('id: $id, ')
          ..write('title: $title, ')
          ..write('workingDirectory: $workingDirectory, ')
          ..write('additionalDirectoriesJson: $additionalDirectoriesJson, ')
          ..write('lastInputTokens: $lastInputTokens, ')
          ..write('lastOutputTokens: $lastOutputTokens, ')
          ..write('lastTotalTokens: $lastTotalTokens, ')
          ..write('lastCacheReadTokens: $lastCacheReadTokens, ')
          ..write('lastCacheWriteTokens: $lastCacheWriteTokens, ')
          ..write('createdAt: $createdAt, ')
          ..write('updatedAt: $updatedAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $TurnsTable extends Turns with TableInfo<$TurnsTable, TurnRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $TurnsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _sessionIdMeta = const VerificationMeta(
    'sessionId',
  );
  @override
  late final GeneratedColumn<String> sessionId = GeneratedColumn<String>(
    'session_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'REFERENCES sessions (id) ON DELETE CASCADE',
    ),
  );
  static const VerificationMeta _statusMeta = const VerificationMeta('status');
  @override
  late final GeneratedColumn<String> status = GeneratedColumn<String>(
    'status',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _startedAtMeta = const VerificationMeta(
    'startedAt',
  );
  @override
  late final GeneratedColumn<DateTime> startedAt = GeneratedColumn<DateTime>(
    'started_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _completedAtMeta = const VerificationMeta(
    'completedAt',
  );
  @override
  late final GeneratedColumn<DateTime> completedAt = GeneratedColumn<DateTime>(
    'completed_at',
    aliasedName,
    true,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _providerIdMeta = const VerificationMeta(
    'providerId',
  );
  @override
  late final GeneratedColumn<String> providerId = GeneratedColumn<String>(
    'provider_id',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _modelIdMeta = const VerificationMeta(
    'modelId',
  );
  @override
  late final GeneratedColumn<String> modelId = GeneratedColumn<String>(
    'model_id',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _reasoningEffortMeta = const VerificationMeta(
    'reasoningEffort',
  );
  @override
  late final GeneratedColumn<String> reasoningEffort = GeneratedColumn<String>(
    'reasoning_effort',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _inputTokensMeta = const VerificationMeta(
    'inputTokens',
  );
  @override
  late final GeneratedColumn<int> inputTokens = GeneratedColumn<int>(
    'input_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _outputTokensMeta = const VerificationMeta(
    'outputTokens',
  );
  @override
  late final GeneratedColumn<int> outputTokens = GeneratedColumn<int>(
    'output_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _totalTokensMeta = const VerificationMeta(
    'totalTokens',
  );
  @override
  late final GeneratedColumn<int> totalTokens = GeneratedColumn<int>(
    'total_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _cacheReadTokensMeta = const VerificationMeta(
    'cacheReadTokens',
  );
  @override
  late final GeneratedColumn<int> cacheReadTokens = GeneratedColumn<int>(
    'cache_read_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _cacheWriteTokensMeta = const VerificationMeta(
    'cacheWriteTokens',
  );
  @override
  late final GeneratedColumn<int> cacheWriteTokens = GeneratedColumn<int>(
    'cache_write_tokens',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _failureCodeMeta = const VerificationMeta(
    'failureCode',
  );
  @override
  late final GeneratedColumn<String> failureCode = GeneratedColumn<String>(
    'failure_code',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _failureMessageMeta = const VerificationMeta(
    'failureMessage',
  );
  @override
  late final GeneratedColumn<String> failureMessage = GeneratedColumn<String>(
    'failure_message',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _cancelReasonMeta = const VerificationMeta(
    'cancelReason',
  );
  @override
  late final GeneratedColumn<String> cancelReason = GeneratedColumn<String>(
    'cancel_reason',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    sessionId,
    status,
    startedAt,
    completedAt,
    providerId,
    modelId,
    reasoningEffort,
    inputTokens,
    outputTokens,
    totalTokens,
    cacheReadTokens,
    cacheWriteTokens,
    failureCode,
    failureMessage,
    cancelReason,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'turns';
  @override
  VerificationContext validateIntegrity(
    Insertable<TurnRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('session_id')) {
      context.handle(
        _sessionIdMeta,
        sessionId.isAcceptableOrUnknown(data['session_id']!, _sessionIdMeta),
      );
    } else if (isInserting) {
      context.missing(_sessionIdMeta);
    }
    if (data.containsKey('status')) {
      context.handle(
        _statusMeta,
        status.isAcceptableOrUnknown(data['status']!, _statusMeta),
      );
    } else if (isInserting) {
      context.missing(_statusMeta);
    }
    if (data.containsKey('started_at')) {
      context.handle(
        _startedAtMeta,
        startedAt.isAcceptableOrUnknown(data['started_at']!, _startedAtMeta),
      );
    } else if (isInserting) {
      context.missing(_startedAtMeta);
    }
    if (data.containsKey('completed_at')) {
      context.handle(
        _completedAtMeta,
        completedAt.isAcceptableOrUnknown(
          data['completed_at']!,
          _completedAtMeta,
        ),
      );
    }
    if (data.containsKey('provider_id')) {
      context.handle(
        _providerIdMeta,
        providerId.isAcceptableOrUnknown(data['provider_id']!, _providerIdMeta),
      );
    }
    if (data.containsKey('model_id')) {
      context.handle(
        _modelIdMeta,
        modelId.isAcceptableOrUnknown(data['model_id']!, _modelIdMeta),
      );
    }
    if (data.containsKey('reasoning_effort')) {
      context.handle(
        _reasoningEffortMeta,
        reasoningEffort.isAcceptableOrUnknown(
          data['reasoning_effort']!,
          _reasoningEffortMeta,
        ),
      );
    }
    if (data.containsKey('input_tokens')) {
      context.handle(
        _inputTokensMeta,
        inputTokens.isAcceptableOrUnknown(
          data['input_tokens']!,
          _inputTokensMeta,
        ),
      );
    }
    if (data.containsKey('output_tokens')) {
      context.handle(
        _outputTokensMeta,
        outputTokens.isAcceptableOrUnknown(
          data['output_tokens']!,
          _outputTokensMeta,
        ),
      );
    }
    if (data.containsKey('total_tokens')) {
      context.handle(
        _totalTokensMeta,
        totalTokens.isAcceptableOrUnknown(
          data['total_tokens']!,
          _totalTokensMeta,
        ),
      );
    }
    if (data.containsKey('cache_read_tokens')) {
      context.handle(
        _cacheReadTokensMeta,
        cacheReadTokens.isAcceptableOrUnknown(
          data['cache_read_tokens']!,
          _cacheReadTokensMeta,
        ),
      );
    }
    if (data.containsKey('cache_write_tokens')) {
      context.handle(
        _cacheWriteTokensMeta,
        cacheWriteTokens.isAcceptableOrUnknown(
          data['cache_write_tokens']!,
          _cacheWriteTokensMeta,
        ),
      );
    }
    if (data.containsKey('failure_code')) {
      context.handle(
        _failureCodeMeta,
        failureCode.isAcceptableOrUnknown(
          data['failure_code']!,
          _failureCodeMeta,
        ),
      );
    }
    if (data.containsKey('failure_message')) {
      context.handle(
        _failureMessageMeta,
        failureMessage.isAcceptableOrUnknown(
          data['failure_message']!,
          _failureMessageMeta,
        ),
      );
    }
    if (data.containsKey('cancel_reason')) {
      context.handle(
        _cancelReasonMeta,
        cancelReason.isAcceptableOrUnknown(
          data['cancel_reason']!,
          _cancelReasonMeta,
        ),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  TurnRow map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return TurnRow(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      sessionId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}session_id'],
      )!,
      status: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}status'],
      )!,
      startedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}started_at'],
      )!,
      completedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}completed_at'],
      ),
      providerId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}provider_id'],
      ),
      modelId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}model_id'],
      ),
      reasoningEffort: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}reasoning_effort'],
      ),
      inputTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}input_tokens'],
      )!,
      outputTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}output_tokens'],
      )!,
      totalTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}total_tokens'],
      )!,
      cacheReadTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}cache_read_tokens'],
      )!,
      cacheWriteTokens: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}cache_write_tokens'],
      )!,
      failureCode: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}failure_code'],
      ),
      failureMessage: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}failure_message'],
      ),
      cancelReason: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}cancel_reason'],
      ),
    );
  }

  @override
  $TurnsTable createAlias(String alias) {
    return $TurnsTable(attachedDatabase, alias);
  }
}

class TurnRow extends DataClass implements Insertable<TurnRow> {
  /// Serialized turn identifier.
  final String id;

  /// Owning session identifier.
  final String sessionId;

  /// Serialized runtime turn status.
  final String status;

  /// UTC turn start time.
  final DateTime startedAt;

  /// UTC terminal time.
  final DateTime? completedAt;

  /// Selected provider identifier.
  final String? providerId;

  /// Selected model identifier.
  final String? modelId;

  /// Provider-local reasoning effort.
  final String? reasoningEffort;

  /// Input token usage.
  final int inputTokens;

  /// Output token usage.
  final int outputTokens;

  /// Total token usage.
  final int totalTokens;

  /// Cached input tokens read.
  final int cacheReadTokens;

  /// Cached input tokens written.
  final int cacheWriteTokens;

  /// Stable runtime failure code.
  final String? failureCode;

  /// User-visible failure message.
  final String? failureMessage;

  /// Cancellation reason.
  final String? cancelReason;
  const TurnRow({
    required this.id,
    required this.sessionId,
    required this.status,
    required this.startedAt,
    this.completedAt,
    this.providerId,
    this.modelId,
    this.reasoningEffort,
    required this.inputTokens,
    required this.outputTokens,
    required this.totalTokens,
    required this.cacheReadTokens,
    required this.cacheWriteTokens,
    this.failureCode,
    this.failureMessage,
    this.cancelReason,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    map['session_id'] = Variable<String>(sessionId);
    map['status'] = Variable<String>(status);
    map['started_at'] = Variable<DateTime>(startedAt);
    if (!nullToAbsent || completedAt != null) {
      map['completed_at'] = Variable<DateTime>(completedAt);
    }
    if (!nullToAbsent || providerId != null) {
      map['provider_id'] = Variable<String>(providerId);
    }
    if (!nullToAbsent || modelId != null) {
      map['model_id'] = Variable<String>(modelId);
    }
    if (!nullToAbsent || reasoningEffort != null) {
      map['reasoning_effort'] = Variable<String>(reasoningEffort);
    }
    map['input_tokens'] = Variable<int>(inputTokens);
    map['output_tokens'] = Variable<int>(outputTokens);
    map['total_tokens'] = Variable<int>(totalTokens);
    map['cache_read_tokens'] = Variable<int>(cacheReadTokens);
    map['cache_write_tokens'] = Variable<int>(cacheWriteTokens);
    if (!nullToAbsent || failureCode != null) {
      map['failure_code'] = Variable<String>(failureCode);
    }
    if (!nullToAbsent || failureMessage != null) {
      map['failure_message'] = Variable<String>(failureMessage);
    }
    if (!nullToAbsent || cancelReason != null) {
      map['cancel_reason'] = Variable<String>(cancelReason);
    }
    return map;
  }

  TurnsCompanion toCompanion(bool nullToAbsent) {
    return TurnsCompanion(
      id: Value(id),
      sessionId: Value(sessionId),
      status: Value(status),
      startedAt: Value(startedAt),
      completedAt: completedAt == null && nullToAbsent
          ? const Value.absent()
          : Value(completedAt),
      providerId: providerId == null && nullToAbsent
          ? const Value.absent()
          : Value(providerId),
      modelId: modelId == null && nullToAbsent
          ? const Value.absent()
          : Value(modelId),
      reasoningEffort: reasoningEffort == null && nullToAbsent
          ? const Value.absent()
          : Value(reasoningEffort),
      inputTokens: Value(inputTokens),
      outputTokens: Value(outputTokens),
      totalTokens: Value(totalTokens),
      cacheReadTokens: Value(cacheReadTokens),
      cacheWriteTokens: Value(cacheWriteTokens),
      failureCode: failureCode == null && nullToAbsent
          ? const Value.absent()
          : Value(failureCode),
      failureMessage: failureMessage == null && nullToAbsent
          ? const Value.absent()
          : Value(failureMessage),
      cancelReason: cancelReason == null && nullToAbsent
          ? const Value.absent()
          : Value(cancelReason),
    );
  }

  factory TurnRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return TurnRow(
      id: serializer.fromJson<String>(json['id']),
      sessionId: serializer.fromJson<String>(json['sessionId']),
      status: serializer.fromJson<String>(json['status']),
      startedAt: serializer.fromJson<DateTime>(json['startedAt']),
      completedAt: serializer.fromJson<DateTime?>(json['completedAt']),
      providerId: serializer.fromJson<String?>(json['providerId']),
      modelId: serializer.fromJson<String?>(json['modelId']),
      reasoningEffort: serializer.fromJson<String?>(json['reasoningEffort']),
      inputTokens: serializer.fromJson<int>(json['inputTokens']),
      outputTokens: serializer.fromJson<int>(json['outputTokens']),
      totalTokens: serializer.fromJson<int>(json['totalTokens']),
      cacheReadTokens: serializer.fromJson<int>(json['cacheReadTokens']),
      cacheWriteTokens: serializer.fromJson<int>(json['cacheWriteTokens']),
      failureCode: serializer.fromJson<String?>(json['failureCode']),
      failureMessage: serializer.fromJson<String?>(json['failureMessage']),
      cancelReason: serializer.fromJson<String?>(json['cancelReason']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'sessionId': serializer.toJson<String>(sessionId),
      'status': serializer.toJson<String>(status),
      'startedAt': serializer.toJson<DateTime>(startedAt),
      'completedAt': serializer.toJson<DateTime?>(completedAt),
      'providerId': serializer.toJson<String?>(providerId),
      'modelId': serializer.toJson<String?>(modelId),
      'reasoningEffort': serializer.toJson<String?>(reasoningEffort),
      'inputTokens': serializer.toJson<int>(inputTokens),
      'outputTokens': serializer.toJson<int>(outputTokens),
      'totalTokens': serializer.toJson<int>(totalTokens),
      'cacheReadTokens': serializer.toJson<int>(cacheReadTokens),
      'cacheWriteTokens': serializer.toJson<int>(cacheWriteTokens),
      'failureCode': serializer.toJson<String?>(failureCode),
      'failureMessage': serializer.toJson<String?>(failureMessage),
      'cancelReason': serializer.toJson<String?>(cancelReason),
    };
  }

  TurnRow copyWith({
    String? id,
    String? sessionId,
    String? status,
    DateTime? startedAt,
    Value<DateTime?> completedAt = const Value.absent(),
    Value<String?> providerId = const Value.absent(),
    Value<String?> modelId = const Value.absent(),
    Value<String?> reasoningEffort = const Value.absent(),
    int? inputTokens,
    int? outputTokens,
    int? totalTokens,
    int? cacheReadTokens,
    int? cacheWriteTokens,
    Value<String?> failureCode = const Value.absent(),
    Value<String?> failureMessage = const Value.absent(),
    Value<String?> cancelReason = const Value.absent(),
  }) => TurnRow(
    id: id ?? this.id,
    sessionId: sessionId ?? this.sessionId,
    status: status ?? this.status,
    startedAt: startedAt ?? this.startedAt,
    completedAt: completedAt.present ? completedAt.value : this.completedAt,
    providerId: providerId.present ? providerId.value : this.providerId,
    modelId: modelId.present ? modelId.value : this.modelId,
    reasoningEffort: reasoningEffort.present
        ? reasoningEffort.value
        : this.reasoningEffort,
    inputTokens: inputTokens ?? this.inputTokens,
    outputTokens: outputTokens ?? this.outputTokens,
    totalTokens: totalTokens ?? this.totalTokens,
    cacheReadTokens: cacheReadTokens ?? this.cacheReadTokens,
    cacheWriteTokens: cacheWriteTokens ?? this.cacheWriteTokens,
    failureCode: failureCode.present ? failureCode.value : this.failureCode,
    failureMessage: failureMessage.present
        ? failureMessage.value
        : this.failureMessage,
    cancelReason: cancelReason.present ? cancelReason.value : this.cancelReason,
  );
  TurnRow copyWithCompanion(TurnsCompanion data) {
    return TurnRow(
      id: data.id.present ? data.id.value : this.id,
      sessionId: data.sessionId.present ? data.sessionId.value : this.sessionId,
      status: data.status.present ? data.status.value : this.status,
      startedAt: data.startedAt.present ? data.startedAt.value : this.startedAt,
      completedAt: data.completedAt.present
          ? data.completedAt.value
          : this.completedAt,
      providerId: data.providerId.present
          ? data.providerId.value
          : this.providerId,
      modelId: data.modelId.present ? data.modelId.value : this.modelId,
      reasoningEffort: data.reasoningEffort.present
          ? data.reasoningEffort.value
          : this.reasoningEffort,
      inputTokens: data.inputTokens.present
          ? data.inputTokens.value
          : this.inputTokens,
      outputTokens: data.outputTokens.present
          ? data.outputTokens.value
          : this.outputTokens,
      totalTokens: data.totalTokens.present
          ? data.totalTokens.value
          : this.totalTokens,
      cacheReadTokens: data.cacheReadTokens.present
          ? data.cacheReadTokens.value
          : this.cacheReadTokens,
      cacheWriteTokens: data.cacheWriteTokens.present
          ? data.cacheWriteTokens.value
          : this.cacheWriteTokens,
      failureCode: data.failureCode.present
          ? data.failureCode.value
          : this.failureCode,
      failureMessage: data.failureMessage.present
          ? data.failureMessage.value
          : this.failureMessage,
      cancelReason: data.cancelReason.present
          ? data.cancelReason.value
          : this.cancelReason,
    );
  }

  @override
  String toString() {
    return (StringBuffer('TurnRow(')
          ..write('id: $id, ')
          ..write('sessionId: $sessionId, ')
          ..write('status: $status, ')
          ..write('startedAt: $startedAt, ')
          ..write('completedAt: $completedAt, ')
          ..write('providerId: $providerId, ')
          ..write('modelId: $modelId, ')
          ..write('reasoningEffort: $reasoningEffort, ')
          ..write('inputTokens: $inputTokens, ')
          ..write('outputTokens: $outputTokens, ')
          ..write('totalTokens: $totalTokens, ')
          ..write('cacheReadTokens: $cacheReadTokens, ')
          ..write('cacheWriteTokens: $cacheWriteTokens, ')
          ..write('failureCode: $failureCode, ')
          ..write('failureMessage: $failureMessage, ')
          ..write('cancelReason: $cancelReason')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    id,
    sessionId,
    status,
    startedAt,
    completedAt,
    providerId,
    modelId,
    reasoningEffort,
    inputTokens,
    outputTokens,
    totalTokens,
    cacheReadTokens,
    cacheWriteTokens,
    failureCode,
    failureMessage,
    cancelReason,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is TurnRow &&
          other.id == this.id &&
          other.sessionId == this.sessionId &&
          other.status == this.status &&
          other.startedAt == this.startedAt &&
          other.completedAt == this.completedAt &&
          other.providerId == this.providerId &&
          other.modelId == this.modelId &&
          other.reasoningEffort == this.reasoningEffort &&
          other.inputTokens == this.inputTokens &&
          other.outputTokens == this.outputTokens &&
          other.totalTokens == this.totalTokens &&
          other.cacheReadTokens == this.cacheReadTokens &&
          other.cacheWriteTokens == this.cacheWriteTokens &&
          other.failureCode == this.failureCode &&
          other.failureMessage == this.failureMessage &&
          other.cancelReason == this.cancelReason);
}

class TurnsCompanion extends UpdateCompanion<TurnRow> {
  final Value<String> id;
  final Value<String> sessionId;
  final Value<String> status;
  final Value<DateTime> startedAt;
  final Value<DateTime?> completedAt;
  final Value<String?> providerId;
  final Value<String?> modelId;
  final Value<String?> reasoningEffort;
  final Value<int> inputTokens;
  final Value<int> outputTokens;
  final Value<int> totalTokens;
  final Value<int> cacheReadTokens;
  final Value<int> cacheWriteTokens;
  final Value<String?> failureCode;
  final Value<String?> failureMessage;
  final Value<String?> cancelReason;
  final Value<int> rowid;
  const TurnsCompanion({
    this.id = const Value.absent(),
    this.sessionId = const Value.absent(),
    this.status = const Value.absent(),
    this.startedAt = const Value.absent(),
    this.completedAt = const Value.absent(),
    this.providerId = const Value.absent(),
    this.modelId = const Value.absent(),
    this.reasoningEffort = const Value.absent(),
    this.inputTokens = const Value.absent(),
    this.outputTokens = const Value.absent(),
    this.totalTokens = const Value.absent(),
    this.cacheReadTokens = const Value.absent(),
    this.cacheWriteTokens = const Value.absent(),
    this.failureCode = const Value.absent(),
    this.failureMessage = const Value.absent(),
    this.cancelReason = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  TurnsCompanion.insert({
    required String id,
    required String sessionId,
    required String status,
    required DateTime startedAt,
    this.completedAt = const Value.absent(),
    this.providerId = const Value.absent(),
    this.modelId = const Value.absent(),
    this.reasoningEffort = const Value.absent(),
    this.inputTokens = const Value.absent(),
    this.outputTokens = const Value.absent(),
    this.totalTokens = const Value.absent(),
    this.cacheReadTokens = const Value.absent(),
    this.cacheWriteTokens = const Value.absent(),
    this.failureCode = const Value.absent(),
    this.failureMessage = const Value.absent(),
    this.cancelReason = const Value.absent(),
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       sessionId = Value(sessionId),
       status = Value(status),
       startedAt = Value(startedAt);
  static Insertable<TurnRow> custom({
    Expression<String>? id,
    Expression<String>? sessionId,
    Expression<String>? status,
    Expression<DateTime>? startedAt,
    Expression<DateTime>? completedAt,
    Expression<String>? providerId,
    Expression<String>? modelId,
    Expression<String>? reasoningEffort,
    Expression<int>? inputTokens,
    Expression<int>? outputTokens,
    Expression<int>? totalTokens,
    Expression<int>? cacheReadTokens,
    Expression<int>? cacheWriteTokens,
    Expression<String>? failureCode,
    Expression<String>? failureMessage,
    Expression<String>? cancelReason,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (sessionId != null) 'session_id': sessionId,
      if (status != null) 'status': status,
      if (startedAt != null) 'started_at': startedAt,
      if (completedAt != null) 'completed_at': completedAt,
      if (providerId != null) 'provider_id': providerId,
      if (modelId != null) 'model_id': modelId,
      if (reasoningEffort != null) 'reasoning_effort': reasoningEffort,
      if (inputTokens != null) 'input_tokens': inputTokens,
      if (outputTokens != null) 'output_tokens': outputTokens,
      if (totalTokens != null) 'total_tokens': totalTokens,
      if (cacheReadTokens != null) 'cache_read_tokens': cacheReadTokens,
      if (cacheWriteTokens != null) 'cache_write_tokens': cacheWriteTokens,
      if (failureCode != null) 'failure_code': failureCode,
      if (failureMessage != null) 'failure_message': failureMessage,
      if (cancelReason != null) 'cancel_reason': cancelReason,
      if (rowid != null) 'rowid': rowid,
    });
  }

  TurnsCompanion copyWith({
    Value<String>? id,
    Value<String>? sessionId,
    Value<String>? status,
    Value<DateTime>? startedAt,
    Value<DateTime?>? completedAt,
    Value<String?>? providerId,
    Value<String?>? modelId,
    Value<String?>? reasoningEffort,
    Value<int>? inputTokens,
    Value<int>? outputTokens,
    Value<int>? totalTokens,
    Value<int>? cacheReadTokens,
    Value<int>? cacheWriteTokens,
    Value<String?>? failureCode,
    Value<String?>? failureMessage,
    Value<String?>? cancelReason,
    Value<int>? rowid,
  }) {
    return TurnsCompanion(
      id: id ?? this.id,
      sessionId: sessionId ?? this.sessionId,
      status: status ?? this.status,
      startedAt: startedAt ?? this.startedAt,
      completedAt: completedAt ?? this.completedAt,
      providerId: providerId ?? this.providerId,
      modelId: modelId ?? this.modelId,
      reasoningEffort: reasoningEffort ?? this.reasoningEffort,
      inputTokens: inputTokens ?? this.inputTokens,
      outputTokens: outputTokens ?? this.outputTokens,
      totalTokens: totalTokens ?? this.totalTokens,
      cacheReadTokens: cacheReadTokens ?? this.cacheReadTokens,
      cacheWriteTokens: cacheWriteTokens ?? this.cacheWriteTokens,
      failureCode: failureCode ?? this.failureCode,
      failureMessage: failureMessage ?? this.failureMessage,
      cancelReason: cancelReason ?? this.cancelReason,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (sessionId.present) {
      map['session_id'] = Variable<String>(sessionId.value);
    }
    if (status.present) {
      map['status'] = Variable<String>(status.value);
    }
    if (startedAt.present) {
      map['started_at'] = Variable<DateTime>(startedAt.value);
    }
    if (completedAt.present) {
      map['completed_at'] = Variable<DateTime>(completedAt.value);
    }
    if (providerId.present) {
      map['provider_id'] = Variable<String>(providerId.value);
    }
    if (modelId.present) {
      map['model_id'] = Variable<String>(modelId.value);
    }
    if (reasoningEffort.present) {
      map['reasoning_effort'] = Variable<String>(reasoningEffort.value);
    }
    if (inputTokens.present) {
      map['input_tokens'] = Variable<int>(inputTokens.value);
    }
    if (outputTokens.present) {
      map['output_tokens'] = Variable<int>(outputTokens.value);
    }
    if (totalTokens.present) {
      map['total_tokens'] = Variable<int>(totalTokens.value);
    }
    if (cacheReadTokens.present) {
      map['cache_read_tokens'] = Variable<int>(cacheReadTokens.value);
    }
    if (cacheWriteTokens.present) {
      map['cache_write_tokens'] = Variable<int>(cacheWriteTokens.value);
    }
    if (failureCode.present) {
      map['failure_code'] = Variable<String>(failureCode.value);
    }
    if (failureMessage.present) {
      map['failure_message'] = Variable<String>(failureMessage.value);
    }
    if (cancelReason.present) {
      map['cancel_reason'] = Variable<String>(cancelReason.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('TurnsCompanion(')
          ..write('id: $id, ')
          ..write('sessionId: $sessionId, ')
          ..write('status: $status, ')
          ..write('startedAt: $startedAt, ')
          ..write('completedAt: $completedAt, ')
          ..write('providerId: $providerId, ')
          ..write('modelId: $modelId, ')
          ..write('reasoningEffort: $reasoningEffort, ')
          ..write('inputTokens: $inputTokens, ')
          ..write('outputTokens: $outputTokens, ')
          ..write('totalTokens: $totalTokens, ')
          ..write('cacheReadTokens: $cacheReadTokens, ')
          ..write('cacheWriteTokens: $cacheWriteTokens, ')
          ..write('failureCode: $failureCode, ')
          ..write('failureMessage: $failureMessage, ')
          ..write('cancelReason: $cancelReason, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $TimelineItemsTable extends TimelineItems
    with TableInfo<$TimelineItemsTable, TimelineItemRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $TimelineItemsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _sessionIdMeta = const VerificationMeta(
    'sessionId',
  );
  @override
  late final GeneratedColumn<String> sessionId = GeneratedColumn<String>(
    'session_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'REFERENCES sessions (id) ON DELETE CASCADE',
    ),
  );
  static const VerificationMeta _turnIdMeta = const VerificationMeta('turnId');
  @override
  late final GeneratedColumn<String> turnId = GeneratedColumn<String>(
    'turn_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'REFERENCES turns (id) ON DELETE CASCADE',
    ),
  );
  static const VerificationMeta _sequenceMeta = const VerificationMeta(
    'sequence',
  );
  @override
  late final GeneratedColumn<int> sequence = GeneratedColumn<int>(
    'sequence',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _kindMeta = const VerificationMeta('kind');
  @override
  late final GeneratedColumn<String> kind = GeneratedColumn<String>(
    'kind',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _payloadVersionMeta = const VerificationMeta(
    'payloadVersion',
  );
  @override
  late final GeneratedColumn<int> payloadVersion = GeneratedColumn<int>(
    'payload_version',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(1),
  );
  static const VerificationMeta _payloadJsonMeta = const VerificationMeta(
    'payloadJson',
  );
  @override
  late final GeneratedColumn<String> payloadJson = GeneratedColumn<String>(
    'payload_json',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _occurredAtMeta = const VerificationMeta(
    'occurredAt',
  );
  @override
  late final GeneratedColumn<DateTime> occurredAt = GeneratedColumn<DateTime>(
    'occurred_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    sessionId,
    turnId,
    sequence,
    kind,
    payloadVersion,
    payloadJson,
    occurredAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'timeline_items';
  @override
  VerificationContext validateIntegrity(
    Insertable<TimelineItemRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('session_id')) {
      context.handle(
        _sessionIdMeta,
        sessionId.isAcceptableOrUnknown(data['session_id']!, _sessionIdMeta),
      );
    } else if (isInserting) {
      context.missing(_sessionIdMeta);
    }
    if (data.containsKey('turn_id')) {
      context.handle(
        _turnIdMeta,
        turnId.isAcceptableOrUnknown(data['turn_id']!, _turnIdMeta),
      );
    } else if (isInserting) {
      context.missing(_turnIdMeta);
    }
    if (data.containsKey('sequence')) {
      context.handle(
        _sequenceMeta,
        sequence.isAcceptableOrUnknown(data['sequence']!, _sequenceMeta),
      );
    } else if (isInserting) {
      context.missing(_sequenceMeta);
    }
    if (data.containsKey('kind')) {
      context.handle(
        _kindMeta,
        kind.isAcceptableOrUnknown(data['kind']!, _kindMeta),
      );
    } else if (isInserting) {
      context.missing(_kindMeta);
    }
    if (data.containsKey('payload_version')) {
      context.handle(
        _payloadVersionMeta,
        payloadVersion.isAcceptableOrUnknown(
          data['payload_version']!,
          _payloadVersionMeta,
        ),
      );
    }
    if (data.containsKey('payload_json')) {
      context.handle(
        _payloadJsonMeta,
        payloadJson.isAcceptableOrUnknown(
          data['payload_json']!,
          _payloadJsonMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_payloadJsonMeta);
    }
    if (data.containsKey('occurred_at')) {
      context.handle(
        _occurredAtMeta,
        occurredAt.isAcceptableOrUnknown(data['occurred_at']!, _occurredAtMeta),
      );
    } else if (isInserting) {
      context.missing(_occurredAtMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  List<Set<GeneratedColumn>> get uniqueKeys => [
    {sessionId, sequence},
  ];
  @override
  TimelineItemRow map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return TimelineItemRow(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      sessionId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}session_id'],
      )!,
      turnId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}turn_id'],
      )!,
      sequence: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}sequence'],
      )!,
      kind: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}kind'],
      )!,
      payloadVersion: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}payload_version'],
      )!,
      payloadJson: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}payload_json'],
      )!,
      occurredAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}occurred_at'],
      )!,
    );
  }

  @override
  $TimelineItemsTable createAlias(String alias) {
    return $TimelineItemsTable(attachedDatabase, alias);
  }
}

class TimelineItemRow extends DataClass implements Insertable<TimelineItemRow> {
  /// Serialized timeline item identifier.
  final String id;

  /// Owning session identifier.
  final String sessionId;

  /// Owning turn identifier.
  final String turnId;

  /// Strict session-local order.
  final int sequence;

  /// Stable timeline item discriminant.
  final String kind;

  /// Version of the JSON payload schema.
  final int payloadVersion;

  /// Versioned JSON payload.
  final String payloadJson;

  /// UTC append time.
  final DateTime occurredAt;
  const TimelineItemRow({
    required this.id,
    required this.sessionId,
    required this.turnId,
    required this.sequence,
    required this.kind,
    required this.payloadVersion,
    required this.payloadJson,
    required this.occurredAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    map['session_id'] = Variable<String>(sessionId);
    map['turn_id'] = Variable<String>(turnId);
    map['sequence'] = Variable<int>(sequence);
    map['kind'] = Variable<String>(kind);
    map['payload_version'] = Variable<int>(payloadVersion);
    map['payload_json'] = Variable<String>(payloadJson);
    map['occurred_at'] = Variable<DateTime>(occurredAt);
    return map;
  }

  TimelineItemsCompanion toCompanion(bool nullToAbsent) {
    return TimelineItemsCompanion(
      id: Value(id),
      sessionId: Value(sessionId),
      turnId: Value(turnId),
      sequence: Value(sequence),
      kind: Value(kind),
      payloadVersion: Value(payloadVersion),
      payloadJson: Value(payloadJson),
      occurredAt: Value(occurredAt),
    );
  }

  factory TimelineItemRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return TimelineItemRow(
      id: serializer.fromJson<String>(json['id']),
      sessionId: serializer.fromJson<String>(json['sessionId']),
      turnId: serializer.fromJson<String>(json['turnId']),
      sequence: serializer.fromJson<int>(json['sequence']),
      kind: serializer.fromJson<String>(json['kind']),
      payloadVersion: serializer.fromJson<int>(json['payloadVersion']),
      payloadJson: serializer.fromJson<String>(json['payloadJson']),
      occurredAt: serializer.fromJson<DateTime>(json['occurredAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'sessionId': serializer.toJson<String>(sessionId),
      'turnId': serializer.toJson<String>(turnId),
      'sequence': serializer.toJson<int>(sequence),
      'kind': serializer.toJson<String>(kind),
      'payloadVersion': serializer.toJson<int>(payloadVersion),
      'payloadJson': serializer.toJson<String>(payloadJson),
      'occurredAt': serializer.toJson<DateTime>(occurredAt),
    };
  }

  TimelineItemRow copyWith({
    String? id,
    String? sessionId,
    String? turnId,
    int? sequence,
    String? kind,
    int? payloadVersion,
    String? payloadJson,
    DateTime? occurredAt,
  }) => TimelineItemRow(
    id: id ?? this.id,
    sessionId: sessionId ?? this.sessionId,
    turnId: turnId ?? this.turnId,
    sequence: sequence ?? this.sequence,
    kind: kind ?? this.kind,
    payloadVersion: payloadVersion ?? this.payloadVersion,
    payloadJson: payloadJson ?? this.payloadJson,
    occurredAt: occurredAt ?? this.occurredAt,
  );
  TimelineItemRow copyWithCompanion(TimelineItemsCompanion data) {
    return TimelineItemRow(
      id: data.id.present ? data.id.value : this.id,
      sessionId: data.sessionId.present ? data.sessionId.value : this.sessionId,
      turnId: data.turnId.present ? data.turnId.value : this.turnId,
      sequence: data.sequence.present ? data.sequence.value : this.sequence,
      kind: data.kind.present ? data.kind.value : this.kind,
      payloadVersion: data.payloadVersion.present
          ? data.payloadVersion.value
          : this.payloadVersion,
      payloadJson: data.payloadJson.present
          ? data.payloadJson.value
          : this.payloadJson,
      occurredAt: data.occurredAt.present
          ? data.occurredAt.value
          : this.occurredAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('TimelineItemRow(')
          ..write('id: $id, ')
          ..write('sessionId: $sessionId, ')
          ..write('turnId: $turnId, ')
          ..write('sequence: $sequence, ')
          ..write('kind: $kind, ')
          ..write('payloadVersion: $payloadVersion, ')
          ..write('payloadJson: $payloadJson, ')
          ..write('occurredAt: $occurredAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    id,
    sessionId,
    turnId,
    sequence,
    kind,
    payloadVersion,
    payloadJson,
    occurredAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is TimelineItemRow &&
          other.id == this.id &&
          other.sessionId == this.sessionId &&
          other.turnId == this.turnId &&
          other.sequence == this.sequence &&
          other.kind == this.kind &&
          other.payloadVersion == this.payloadVersion &&
          other.payloadJson == this.payloadJson &&
          other.occurredAt == this.occurredAt);
}

class TimelineItemsCompanion extends UpdateCompanion<TimelineItemRow> {
  final Value<String> id;
  final Value<String> sessionId;
  final Value<String> turnId;
  final Value<int> sequence;
  final Value<String> kind;
  final Value<int> payloadVersion;
  final Value<String> payloadJson;
  final Value<DateTime> occurredAt;
  final Value<int> rowid;
  const TimelineItemsCompanion({
    this.id = const Value.absent(),
    this.sessionId = const Value.absent(),
    this.turnId = const Value.absent(),
    this.sequence = const Value.absent(),
    this.kind = const Value.absent(),
    this.payloadVersion = const Value.absent(),
    this.payloadJson = const Value.absent(),
    this.occurredAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  TimelineItemsCompanion.insert({
    required String id,
    required String sessionId,
    required String turnId,
    required int sequence,
    required String kind,
    this.payloadVersion = const Value.absent(),
    required String payloadJson,
    required DateTime occurredAt,
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       sessionId = Value(sessionId),
       turnId = Value(turnId),
       sequence = Value(sequence),
       kind = Value(kind),
       payloadJson = Value(payloadJson),
       occurredAt = Value(occurredAt);
  static Insertable<TimelineItemRow> custom({
    Expression<String>? id,
    Expression<String>? sessionId,
    Expression<String>? turnId,
    Expression<int>? sequence,
    Expression<String>? kind,
    Expression<int>? payloadVersion,
    Expression<String>? payloadJson,
    Expression<DateTime>? occurredAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (sessionId != null) 'session_id': sessionId,
      if (turnId != null) 'turn_id': turnId,
      if (sequence != null) 'sequence': sequence,
      if (kind != null) 'kind': kind,
      if (payloadVersion != null) 'payload_version': payloadVersion,
      if (payloadJson != null) 'payload_json': payloadJson,
      if (occurredAt != null) 'occurred_at': occurredAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  TimelineItemsCompanion copyWith({
    Value<String>? id,
    Value<String>? sessionId,
    Value<String>? turnId,
    Value<int>? sequence,
    Value<String>? kind,
    Value<int>? payloadVersion,
    Value<String>? payloadJson,
    Value<DateTime>? occurredAt,
    Value<int>? rowid,
  }) {
    return TimelineItemsCompanion(
      id: id ?? this.id,
      sessionId: sessionId ?? this.sessionId,
      turnId: turnId ?? this.turnId,
      sequence: sequence ?? this.sequence,
      kind: kind ?? this.kind,
      payloadVersion: payloadVersion ?? this.payloadVersion,
      payloadJson: payloadJson ?? this.payloadJson,
      occurredAt: occurredAt ?? this.occurredAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (sessionId.present) {
      map['session_id'] = Variable<String>(sessionId.value);
    }
    if (turnId.present) {
      map['turn_id'] = Variable<String>(turnId.value);
    }
    if (sequence.present) {
      map['sequence'] = Variable<int>(sequence.value);
    }
    if (kind.present) {
      map['kind'] = Variable<String>(kind.value);
    }
    if (payloadVersion.present) {
      map['payload_version'] = Variable<int>(payloadVersion.value);
    }
    if (payloadJson.present) {
      map['payload_json'] = Variable<String>(payloadJson.value);
    }
    if (occurredAt.present) {
      map['occurred_at'] = Variable<DateTime>(occurredAt.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('TimelineItemsCompanion(')
          ..write('id: $id, ')
          ..write('sessionId: $sessionId, ')
          ..write('turnId: $turnId, ')
          ..write('sequence: $sequence, ')
          ..write('kind: $kind, ')
          ..write('payloadVersion: $payloadVersion, ')
          ..write('payloadJson: $payloadJson, ')
          ..write('occurredAt: $occurredAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $ModelCheckpointsTable extends ModelCheckpoints
    with TableInfo<$ModelCheckpointsTable, ModelCheckpointRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $ModelCheckpointsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _timelineItemIdMeta = const VerificationMeta(
    'timelineItemId',
  );
  @override
  late final GeneratedColumn<String> timelineItemId = GeneratedColumn<String>(
    'timeline_item_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'REFERENCES timeline_items (id) ON DELETE CASCADE',
    ),
  );
  static const VerificationMeta _providerIdMeta = const VerificationMeta(
    'providerId',
  );
  @override
  late final GeneratedColumn<String> providerId = GeneratedColumn<String>(
    'provider_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _reasoningSummaryMeta = const VerificationMeta(
    'reasoningSummary',
  );
  @override
  late final GeneratedColumn<String> reasoningSummary = GeneratedColumn<String>(
    'reasoning_summary',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
    defaultValue: const Constant(''),
  );
  static const VerificationMeta _payloadJsonMeta = const VerificationMeta(
    'payloadJson',
  );
  @override
  late final GeneratedColumn<String> payloadJson = GeneratedColumn<String>(
    'payload_json',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
    defaultValue: const Constant('{}'),
  );
  static const VerificationMeta _createdAtMeta = const VerificationMeta(
    'createdAt',
  );
  @override
  late final GeneratedColumn<DateTime> createdAt = GeneratedColumn<DateTime>(
    'created_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [
    timelineItemId,
    providerId,
    reasoningSummary,
    payloadJson,
    createdAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'model_checkpoints';
  @override
  VerificationContext validateIntegrity(
    Insertable<ModelCheckpointRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('timeline_item_id')) {
      context.handle(
        _timelineItemIdMeta,
        timelineItemId.isAcceptableOrUnknown(
          data['timeline_item_id']!,
          _timelineItemIdMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_timelineItemIdMeta);
    }
    if (data.containsKey('provider_id')) {
      context.handle(
        _providerIdMeta,
        providerId.isAcceptableOrUnknown(data['provider_id']!, _providerIdMeta),
      );
    } else if (isInserting) {
      context.missing(_providerIdMeta);
    }
    if (data.containsKey('reasoning_summary')) {
      context.handle(
        _reasoningSummaryMeta,
        reasoningSummary.isAcceptableOrUnknown(
          data['reasoning_summary']!,
          _reasoningSummaryMeta,
        ),
      );
    }
    if (data.containsKey('payload_json')) {
      context.handle(
        _payloadJsonMeta,
        payloadJson.isAcceptableOrUnknown(
          data['payload_json']!,
          _payloadJsonMeta,
        ),
      );
    }
    if (data.containsKey('created_at')) {
      context.handle(
        _createdAtMeta,
        createdAt.isAcceptableOrUnknown(data['created_at']!, _createdAtMeta),
      );
    } else if (isInserting) {
      context.missing(_createdAtMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {timelineItemId};
  @override
  ModelCheckpointRow map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return ModelCheckpointRow(
      timelineItemId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}timeline_item_id'],
      )!,
      providerId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}provider_id'],
      )!,
      reasoningSummary: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}reasoning_summary'],
      )!,
      payloadJson: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}payload_json'],
      )!,
      createdAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}created_at'],
      )!,
    );
  }

  @override
  $ModelCheckpointsTable createAlias(String alias) {
    return $ModelCheckpointsTable(attachedDatabase, alias);
  }
}

class ModelCheckpointRow extends DataClass
    implements Insertable<ModelCheckpointRow> {
  /// Assistant timeline item that owns this continuation.
  final String timelineItemId;

  /// Provider that can interpret the payload.
  final String providerId;

  /// Provider-produced reasoning summary.
  final String reasoningSummary;

  /// Provider-owned continuation payload.
  final String payloadJson;

  /// UTC checkpoint creation time.
  final DateTime createdAt;
  const ModelCheckpointRow({
    required this.timelineItemId,
    required this.providerId,
    required this.reasoningSummary,
    required this.payloadJson,
    required this.createdAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['timeline_item_id'] = Variable<String>(timelineItemId);
    map['provider_id'] = Variable<String>(providerId);
    map['reasoning_summary'] = Variable<String>(reasoningSummary);
    map['payload_json'] = Variable<String>(payloadJson);
    map['created_at'] = Variable<DateTime>(createdAt);
    return map;
  }

  ModelCheckpointsCompanion toCompanion(bool nullToAbsent) {
    return ModelCheckpointsCompanion(
      timelineItemId: Value(timelineItemId),
      providerId: Value(providerId),
      reasoningSummary: Value(reasoningSummary),
      payloadJson: Value(payloadJson),
      createdAt: Value(createdAt),
    );
  }

  factory ModelCheckpointRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return ModelCheckpointRow(
      timelineItemId: serializer.fromJson<String>(json['timelineItemId']),
      providerId: serializer.fromJson<String>(json['providerId']),
      reasoningSummary: serializer.fromJson<String>(json['reasoningSummary']),
      payloadJson: serializer.fromJson<String>(json['payloadJson']),
      createdAt: serializer.fromJson<DateTime>(json['createdAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'timelineItemId': serializer.toJson<String>(timelineItemId),
      'providerId': serializer.toJson<String>(providerId),
      'reasoningSummary': serializer.toJson<String>(reasoningSummary),
      'payloadJson': serializer.toJson<String>(payloadJson),
      'createdAt': serializer.toJson<DateTime>(createdAt),
    };
  }

  ModelCheckpointRow copyWith({
    String? timelineItemId,
    String? providerId,
    String? reasoningSummary,
    String? payloadJson,
    DateTime? createdAt,
  }) => ModelCheckpointRow(
    timelineItemId: timelineItemId ?? this.timelineItemId,
    providerId: providerId ?? this.providerId,
    reasoningSummary: reasoningSummary ?? this.reasoningSummary,
    payloadJson: payloadJson ?? this.payloadJson,
    createdAt: createdAt ?? this.createdAt,
  );
  ModelCheckpointRow copyWithCompanion(ModelCheckpointsCompanion data) {
    return ModelCheckpointRow(
      timelineItemId: data.timelineItemId.present
          ? data.timelineItemId.value
          : this.timelineItemId,
      providerId: data.providerId.present
          ? data.providerId.value
          : this.providerId,
      reasoningSummary: data.reasoningSummary.present
          ? data.reasoningSummary.value
          : this.reasoningSummary,
      payloadJson: data.payloadJson.present
          ? data.payloadJson.value
          : this.payloadJson,
      createdAt: data.createdAt.present ? data.createdAt.value : this.createdAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('ModelCheckpointRow(')
          ..write('timelineItemId: $timelineItemId, ')
          ..write('providerId: $providerId, ')
          ..write('reasoningSummary: $reasoningSummary, ')
          ..write('payloadJson: $payloadJson, ')
          ..write('createdAt: $createdAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    timelineItemId,
    providerId,
    reasoningSummary,
    payloadJson,
    createdAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is ModelCheckpointRow &&
          other.timelineItemId == this.timelineItemId &&
          other.providerId == this.providerId &&
          other.reasoningSummary == this.reasoningSummary &&
          other.payloadJson == this.payloadJson &&
          other.createdAt == this.createdAt);
}

class ModelCheckpointsCompanion extends UpdateCompanion<ModelCheckpointRow> {
  final Value<String> timelineItemId;
  final Value<String> providerId;
  final Value<String> reasoningSummary;
  final Value<String> payloadJson;
  final Value<DateTime> createdAt;
  final Value<int> rowid;
  const ModelCheckpointsCompanion({
    this.timelineItemId = const Value.absent(),
    this.providerId = const Value.absent(),
    this.reasoningSummary = const Value.absent(),
    this.payloadJson = const Value.absent(),
    this.createdAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  ModelCheckpointsCompanion.insert({
    required String timelineItemId,
    required String providerId,
    this.reasoningSummary = const Value.absent(),
    this.payloadJson = const Value.absent(),
    required DateTime createdAt,
    this.rowid = const Value.absent(),
  }) : timelineItemId = Value(timelineItemId),
       providerId = Value(providerId),
       createdAt = Value(createdAt);
  static Insertable<ModelCheckpointRow> custom({
    Expression<String>? timelineItemId,
    Expression<String>? providerId,
    Expression<String>? reasoningSummary,
    Expression<String>? payloadJson,
    Expression<DateTime>? createdAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (timelineItemId != null) 'timeline_item_id': timelineItemId,
      if (providerId != null) 'provider_id': providerId,
      if (reasoningSummary != null) 'reasoning_summary': reasoningSummary,
      if (payloadJson != null) 'payload_json': payloadJson,
      if (createdAt != null) 'created_at': createdAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  ModelCheckpointsCompanion copyWith({
    Value<String>? timelineItemId,
    Value<String>? providerId,
    Value<String>? reasoningSummary,
    Value<String>? payloadJson,
    Value<DateTime>? createdAt,
    Value<int>? rowid,
  }) {
    return ModelCheckpointsCompanion(
      timelineItemId: timelineItemId ?? this.timelineItemId,
      providerId: providerId ?? this.providerId,
      reasoningSummary: reasoningSummary ?? this.reasoningSummary,
      payloadJson: payloadJson ?? this.payloadJson,
      createdAt: createdAt ?? this.createdAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (timelineItemId.present) {
      map['timeline_item_id'] = Variable<String>(timelineItemId.value);
    }
    if (providerId.present) {
      map['provider_id'] = Variable<String>(providerId.value);
    }
    if (reasoningSummary.present) {
      map['reasoning_summary'] = Variable<String>(reasoningSummary.value);
    }
    if (payloadJson.present) {
      map['payload_json'] = Variable<String>(payloadJson.value);
    }
    if (createdAt.present) {
      map['created_at'] = Variable<DateTime>(createdAt.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('ModelCheckpointsCompanion(')
          ..write('timelineItemId: $timelineItemId, ')
          ..write('providerId: $providerId, ')
          ..write('reasoningSummary: $reasoningSummary, ')
          ..write('payloadJson: $payloadJson, ')
          ..write('createdAt: $createdAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

class $CompactionCheckpointsTable extends CompactionCheckpoints
    with TableInfo<$CompactionCheckpointsTable, CompactionCheckpointRow> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $CompactionCheckpointsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _sessionIdMeta = const VerificationMeta(
    'sessionId',
  );
  @override
  late final GeneratedColumn<String> sessionId = GeneratedColumn<String>(
    'session_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'REFERENCES sessions (id) ON DELETE CASCADE',
    ),
  );
  static const VerificationMeta _compactedThroughSequenceMeta =
      const VerificationMeta('compactedThroughSequence');
  @override
  late final GeneratedColumn<int> compactedThroughSequence =
      GeneratedColumn<int>(
        'compacted_through_sequence',
        aliasedName,
        false,
        type: DriftSqlType.int,
        requiredDuringInsert: true,
      );
  static const VerificationMeta _summaryMeta = const VerificationMeta(
    'summary',
  );
  @override
  late final GeneratedColumn<String> summary = GeneratedColumn<String>(
    'summary',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _inputTokensBeforeMeta = const VerificationMeta(
    'inputTokensBefore',
  );
  @override
  late final GeneratedColumn<int> inputTokensBefore = GeneratedColumn<int>(
    'input_tokens_before',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _inputTokensAfterMeta = const VerificationMeta(
    'inputTokensAfter',
  );
  @override
  late final GeneratedColumn<int> inputTokensAfter = GeneratedColumn<int>(
    'input_tokens_after',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _createdAtMeta = const VerificationMeta(
    'createdAt',
  );
  @override
  late final GeneratedColumn<DateTime> createdAt = GeneratedColumn<DateTime>(
    'created_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: true,
  );
  @override
  List<GeneratedColumn> get $columns => [
    sessionId,
    compactedThroughSequence,
    summary,
    inputTokensBefore,
    inputTokensAfter,
    createdAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'compaction_checkpoints';
  @override
  VerificationContext validateIntegrity(
    Insertable<CompactionCheckpointRow> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('session_id')) {
      context.handle(
        _sessionIdMeta,
        sessionId.isAcceptableOrUnknown(data['session_id']!, _sessionIdMeta),
      );
    } else if (isInserting) {
      context.missing(_sessionIdMeta);
    }
    if (data.containsKey('compacted_through_sequence')) {
      context.handle(
        _compactedThroughSequenceMeta,
        compactedThroughSequence.isAcceptableOrUnknown(
          data['compacted_through_sequence']!,
          _compactedThroughSequenceMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_compactedThroughSequenceMeta);
    }
    if (data.containsKey('summary')) {
      context.handle(
        _summaryMeta,
        summary.isAcceptableOrUnknown(data['summary']!, _summaryMeta),
      );
    } else if (isInserting) {
      context.missing(_summaryMeta);
    }
    if (data.containsKey('input_tokens_before')) {
      context.handle(
        _inputTokensBeforeMeta,
        inputTokensBefore.isAcceptableOrUnknown(
          data['input_tokens_before']!,
          _inputTokensBeforeMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_inputTokensBeforeMeta);
    }
    if (data.containsKey('input_tokens_after')) {
      context.handle(
        _inputTokensAfterMeta,
        inputTokensAfter.isAcceptableOrUnknown(
          data['input_tokens_after']!,
          _inputTokensAfterMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_inputTokensAfterMeta);
    }
    if (data.containsKey('created_at')) {
      context.handle(
        _createdAtMeta,
        createdAt.isAcceptableOrUnknown(data['created_at']!, _createdAtMeta),
      );
    } else if (isInserting) {
      context.missing(_createdAtMeta);
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {sessionId};
  @override
  CompactionCheckpointRow map(
    Map<String, dynamic> data, {
    String? tablePrefix,
  }) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return CompactionCheckpointRow(
      sessionId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}session_id'],
      )!,
      compactedThroughSequence: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}compacted_through_sequence'],
      )!,
      summary: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}summary'],
      )!,
      inputTokensBefore: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}input_tokens_before'],
      )!,
      inputTokensAfter: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}input_tokens_after'],
      )!,
      createdAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}created_at'],
      )!,
    );
  }

  @override
  $CompactionCheckpointsTable createAlias(String alias) {
    return $CompactionCheckpointsTable(attachedDatabase, alias);
  }
}

class CompactionCheckpointRow extends DataClass
    implements Insertable<CompactionCheckpointRow> {
  /// Owning session identifier.
  final String sessionId;

  /// Last timeline sequence represented by the summary.
  final int compactedThroughSequence;

  /// Compact model context summary.
  final String summary;

  /// Input token count before compaction.
  final int inputTokensBefore;

  /// Input token count after compaction.
  final int inputTokensAfter;

  /// UTC checkpoint creation time.
  final DateTime createdAt;
  const CompactionCheckpointRow({
    required this.sessionId,
    required this.compactedThroughSequence,
    required this.summary,
    required this.inputTokensBefore,
    required this.inputTokensAfter,
    required this.createdAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['session_id'] = Variable<String>(sessionId);
    map['compacted_through_sequence'] = Variable<int>(compactedThroughSequence);
    map['summary'] = Variable<String>(summary);
    map['input_tokens_before'] = Variable<int>(inputTokensBefore);
    map['input_tokens_after'] = Variable<int>(inputTokensAfter);
    map['created_at'] = Variable<DateTime>(createdAt);
    return map;
  }

  CompactionCheckpointsCompanion toCompanion(bool nullToAbsent) {
    return CompactionCheckpointsCompanion(
      sessionId: Value(sessionId),
      compactedThroughSequence: Value(compactedThroughSequence),
      summary: Value(summary),
      inputTokensBefore: Value(inputTokensBefore),
      inputTokensAfter: Value(inputTokensAfter),
      createdAt: Value(createdAt),
    );
  }

  factory CompactionCheckpointRow.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return CompactionCheckpointRow(
      sessionId: serializer.fromJson<String>(json['sessionId']),
      compactedThroughSequence: serializer.fromJson<int>(
        json['compactedThroughSequence'],
      ),
      summary: serializer.fromJson<String>(json['summary']),
      inputTokensBefore: serializer.fromJson<int>(json['inputTokensBefore']),
      inputTokensAfter: serializer.fromJson<int>(json['inputTokensAfter']),
      createdAt: serializer.fromJson<DateTime>(json['createdAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'sessionId': serializer.toJson<String>(sessionId),
      'compactedThroughSequence': serializer.toJson<int>(
        compactedThroughSequence,
      ),
      'summary': serializer.toJson<String>(summary),
      'inputTokensBefore': serializer.toJson<int>(inputTokensBefore),
      'inputTokensAfter': serializer.toJson<int>(inputTokensAfter),
      'createdAt': serializer.toJson<DateTime>(createdAt),
    };
  }

  CompactionCheckpointRow copyWith({
    String? sessionId,
    int? compactedThroughSequence,
    String? summary,
    int? inputTokensBefore,
    int? inputTokensAfter,
    DateTime? createdAt,
  }) => CompactionCheckpointRow(
    sessionId: sessionId ?? this.sessionId,
    compactedThroughSequence:
        compactedThroughSequence ?? this.compactedThroughSequence,
    summary: summary ?? this.summary,
    inputTokensBefore: inputTokensBefore ?? this.inputTokensBefore,
    inputTokensAfter: inputTokensAfter ?? this.inputTokensAfter,
    createdAt: createdAt ?? this.createdAt,
  );
  CompactionCheckpointRow copyWithCompanion(
    CompactionCheckpointsCompanion data,
  ) {
    return CompactionCheckpointRow(
      sessionId: data.sessionId.present ? data.sessionId.value : this.sessionId,
      compactedThroughSequence: data.compactedThroughSequence.present
          ? data.compactedThroughSequence.value
          : this.compactedThroughSequence,
      summary: data.summary.present ? data.summary.value : this.summary,
      inputTokensBefore: data.inputTokensBefore.present
          ? data.inputTokensBefore.value
          : this.inputTokensBefore,
      inputTokensAfter: data.inputTokensAfter.present
          ? data.inputTokensAfter.value
          : this.inputTokensAfter,
      createdAt: data.createdAt.present ? data.createdAt.value : this.createdAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('CompactionCheckpointRow(')
          ..write('sessionId: $sessionId, ')
          ..write('compactedThroughSequence: $compactedThroughSequence, ')
          ..write('summary: $summary, ')
          ..write('inputTokensBefore: $inputTokensBefore, ')
          ..write('inputTokensAfter: $inputTokensAfter, ')
          ..write('createdAt: $createdAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    sessionId,
    compactedThroughSequence,
    summary,
    inputTokensBefore,
    inputTokensAfter,
    createdAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is CompactionCheckpointRow &&
          other.sessionId == this.sessionId &&
          other.compactedThroughSequence == this.compactedThroughSequence &&
          other.summary == this.summary &&
          other.inputTokensBefore == this.inputTokensBefore &&
          other.inputTokensAfter == this.inputTokensAfter &&
          other.createdAt == this.createdAt);
}

class CompactionCheckpointsCompanion
    extends UpdateCompanion<CompactionCheckpointRow> {
  final Value<String> sessionId;
  final Value<int> compactedThroughSequence;
  final Value<String> summary;
  final Value<int> inputTokensBefore;
  final Value<int> inputTokensAfter;
  final Value<DateTime> createdAt;
  final Value<int> rowid;
  const CompactionCheckpointsCompanion({
    this.sessionId = const Value.absent(),
    this.compactedThroughSequence = const Value.absent(),
    this.summary = const Value.absent(),
    this.inputTokensBefore = const Value.absent(),
    this.inputTokensAfter = const Value.absent(),
    this.createdAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  CompactionCheckpointsCompanion.insert({
    required String sessionId,
    required int compactedThroughSequence,
    required String summary,
    required int inputTokensBefore,
    required int inputTokensAfter,
    required DateTime createdAt,
    this.rowid = const Value.absent(),
  }) : sessionId = Value(sessionId),
       compactedThroughSequence = Value(compactedThroughSequence),
       summary = Value(summary),
       inputTokensBefore = Value(inputTokensBefore),
       inputTokensAfter = Value(inputTokensAfter),
       createdAt = Value(createdAt);
  static Insertable<CompactionCheckpointRow> custom({
    Expression<String>? sessionId,
    Expression<int>? compactedThroughSequence,
    Expression<String>? summary,
    Expression<int>? inputTokensBefore,
    Expression<int>? inputTokensAfter,
    Expression<DateTime>? createdAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (sessionId != null) 'session_id': sessionId,
      if (compactedThroughSequence != null)
        'compacted_through_sequence': compactedThroughSequence,
      if (summary != null) 'summary': summary,
      if (inputTokensBefore != null) 'input_tokens_before': inputTokensBefore,
      if (inputTokensAfter != null) 'input_tokens_after': inputTokensAfter,
      if (createdAt != null) 'created_at': createdAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  CompactionCheckpointsCompanion copyWith({
    Value<String>? sessionId,
    Value<int>? compactedThroughSequence,
    Value<String>? summary,
    Value<int>? inputTokensBefore,
    Value<int>? inputTokensAfter,
    Value<DateTime>? createdAt,
    Value<int>? rowid,
  }) {
    return CompactionCheckpointsCompanion(
      sessionId: sessionId ?? this.sessionId,
      compactedThroughSequence:
          compactedThroughSequence ?? this.compactedThroughSequence,
      summary: summary ?? this.summary,
      inputTokensBefore: inputTokensBefore ?? this.inputTokensBefore,
      inputTokensAfter: inputTokensAfter ?? this.inputTokensAfter,
      createdAt: createdAt ?? this.createdAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (sessionId.present) {
      map['session_id'] = Variable<String>(sessionId.value);
    }
    if (compactedThroughSequence.present) {
      map['compacted_through_sequence'] = Variable<int>(
        compactedThroughSequence.value,
      );
    }
    if (summary.present) {
      map['summary'] = Variable<String>(summary.value);
    }
    if (inputTokensBefore.present) {
      map['input_tokens_before'] = Variable<int>(inputTokensBefore.value);
    }
    if (inputTokensAfter.present) {
      map['input_tokens_after'] = Variable<int>(inputTokensAfter.value);
    }
    if (createdAt.present) {
      map['created_at'] = Variable<DateTime>(createdAt.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('CompactionCheckpointsCompanion(')
          ..write('sessionId: $sessionId, ')
          ..write('compactedThroughSequence: $compactedThroughSequence, ')
          ..write('summary: $summary, ')
          ..write('inputTokensBefore: $inputTokensBefore, ')
          ..write('inputTokensAfter: $inputTokensAfter, ')
          ..write('createdAt: $createdAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

abstract class _$AtlasDatabase extends GeneratedDatabase {
  _$AtlasDatabase(QueryExecutor e) : super(e);
  $AtlasDatabaseManager get managers => $AtlasDatabaseManager(this);
  late final $SessionsTable sessions = $SessionsTable(this);
  late final $TurnsTable turns = $TurnsTable(this);
  late final $TimelineItemsTable timelineItems = $TimelineItemsTable(this);
  late final $ModelCheckpointsTable modelCheckpoints = $ModelCheckpointsTable(
    this,
  );
  late final $CompactionCheckpointsTable compactionCheckpoints =
      $CompactionCheckpointsTable(this);
  @override
  Iterable<TableInfo<Table, Object?>> get allTables =>
      allSchemaEntities.whereType<TableInfo<Table, Object?>>();
  @override
  List<DatabaseSchemaEntity> get allSchemaEntities => [
    sessions,
    turns,
    timelineItems,
    modelCheckpoints,
    compactionCheckpoints,
  ];
  @override
  StreamQueryUpdateRules get streamUpdateRules => const StreamQueryUpdateRules([
    WritePropagation(
      on: TableUpdateQuery.onTableName(
        'sessions',
        limitUpdateKind: UpdateKind.delete,
      ),
      result: [TableUpdate('turns', kind: UpdateKind.delete)],
    ),
    WritePropagation(
      on: TableUpdateQuery.onTableName(
        'sessions',
        limitUpdateKind: UpdateKind.delete,
      ),
      result: [TableUpdate('timeline_items', kind: UpdateKind.delete)],
    ),
    WritePropagation(
      on: TableUpdateQuery.onTableName(
        'turns',
        limitUpdateKind: UpdateKind.delete,
      ),
      result: [TableUpdate('timeline_items', kind: UpdateKind.delete)],
    ),
    WritePropagation(
      on: TableUpdateQuery.onTableName(
        'timeline_items',
        limitUpdateKind: UpdateKind.delete,
      ),
      result: [TableUpdate('model_checkpoints', kind: UpdateKind.delete)],
    ),
    WritePropagation(
      on: TableUpdateQuery.onTableName(
        'sessions',
        limitUpdateKind: UpdateKind.delete,
      ),
      result: [TableUpdate('compaction_checkpoints', kind: UpdateKind.delete)],
    ),
  ]);
}

typedef $$SessionsTableCreateCompanionBuilder =
    SessionsCompanion Function({
      required String id,
      Value<String> title,
      required String workingDirectory,
      Value<String> additionalDirectoriesJson,
      Value<int> lastInputTokens,
      Value<int> lastOutputTokens,
      Value<int> lastTotalTokens,
      Value<int> lastCacheReadTokens,
      Value<int> lastCacheWriteTokens,
      required DateTime createdAt,
      required DateTime updatedAt,
      Value<int> rowid,
    });
typedef $$SessionsTableUpdateCompanionBuilder =
    SessionsCompanion Function({
      Value<String> id,
      Value<String> title,
      Value<String> workingDirectory,
      Value<String> additionalDirectoriesJson,
      Value<int> lastInputTokens,
      Value<int> lastOutputTokens,
      Value<int> lastTotalTokens,
      Value<int> lastCacheReadTokens,
      Value<int> lastCacheWriteTokens,
      Value<DateTime> createdAt,
      Value<DateTime> updatedAt,
      Value<int> rowid,
    });

final class $$SessionsTableReferences
    extends BaseReferences<_$AtlasDatabase, $SessionsTable, SessionRow> {
  $$SessionsTableReferences(super.$_db, super.$_table, super.$_typedResult);

  static MultiTypedResultKey<$TurnsTable, List<TurnRow>> _turnsRefsTable(
    _$AtlasDatabase db,
  ) => MultiTypedResultKey.fromTable(
    db.turns,
    aliasName: 'sessions__id__turns__session_id',
  );

  $$TurnsTableProcessedTableManager get turnsRefs {
    final manager = $$TurnsTableTableManager(
      $_db,
      $_db.turns,
    ).filter((f) => f.sessionId.id.sqlEquals($_itemColumn<String>('id')!));

    final cache = $_typedResult.readTableOrNull(_turnsRefsTable($_db));
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: cache),
    );
  }

  static MultiTypedResultKey<$TimelineItemsTable, List<TimelineItemRow>>
  _timelineItemsRefsTable(_$AtlasDatabase db) => MultiTypedResultKey.fromTable(
    db.timelineItems,
    aliasName: 'sessions__id__timeline_items__session_id',
  );

  $$TimelineItemsTableProcessedTableManager get timelineItemsRefs {
    final manager = $$TimelineItemsTableTableManager(
      $_db,
      $_db.timelineItems,
    ).filter((f) => f.sessionId.id.sqlEquals($_itemColumn<String>('id')!));

    final cache = $_typedResult.readTableOrNull(_timelineItemsRefsTable($_db));
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: cache),
    );
  }

  static MultiTypedResultKey<
    $CompactionCheckpointsTable,
    List<CompactionCheckpointRow>
  >
  _compactionCheckpointsRefsTable(_$AtlasDatabase db) =>
      MultiTypedResultKey.fromTable(
        db.compactionCheckpoints,
        aliasName: 'sessions__id__compaction_checkpoints__session_id',
      );

  $$CompactionCheckpointsTableProcessedTableManager
  get compactionCheckpointsRefs {
    final manager = $$CompactionCheckpointsTableTableManager(
      $_db,
      $_db.compactionCheckpoints,
    ).filter((f) => f.sessionId.id.sqlEquals($_itemColumn<String>('id')!));

    final cache = $_typedResult.readTableOrNull(
      _compactionCheckpointsRefsTable($_db),
    );
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: cache),
    );
  }
}

class $$SessionsTableFilterComposer
    extends Composer<_$AtlasDatabase, $SessionsTable> {
  $$SessionsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get workingDirectory => $composableBuilder(
    column: $table.workingDirectory,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get additionalDirectoriesJson => $composableBuilder(
    column: $table.additionalDirectoriesJson,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get lastInputTokens => $composableBuilder(
    column: $table.lastInputTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get lastOutputTokens => $composableBuilder(
    column: $table.lastOutputTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get lastTotalTokens => $composableBuilder(
    column: $table.lastTotalTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get lastCacheReadTokens => $composableBuilder(
    column: $table.lastCacheReadTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get lastCacheWriteTokens => $composableBuilder(
    column: $table.lastCacheWriteTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnFilters(column),
  );

  Expression<bool> turnsRefs(
    Expression<bool> Function($$TurnsTableFilterComposer f) f,
  ) {
    final $$TurnsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.id,
      referencedTable: $db.turns,
      getReferencedColumn: (t) => t.sessionId,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TurnsTableFilterComposer(
            $db: $db,
            $table: $db.turns,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return f(composer);
  }

  Expression<bool> timelineItemsRefs(
    Expression<bool> Function($$TimelineItemsTableFilterComposer f) f,
  ) {
    final $$TimelineItemsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.id,
      referencedTable: $db.timelineItems,
      getReferencedColumn: (t) => t.sessionId,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TimelineItemsTableFilterComposer(
            $db: $db,
            $table: $db.timelineItems,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return f(composer);
  }

  Expression<bool> compactionCheckpointsRefs(
    Expression<bool> Function($$CompactionCheckpointsTableFilterComposer f) f,
  ) {
    final $$CompactionCheckpointsTableFilterComposer composer =
        $composerBuilder(
          composer: this,
          getCurrentColumn: (t) => t.id,
          referencedTable: $db.compactionCheckpoints,
          getReferencedColumn: (t) => t.sessionId,
          builder:
              (
                joinBuilder, {
                $addJoinBuilderToRootComposer,
                $removeJoinBuilderFromRootComposer,
              }) => $$CompactionCheckpointsTableFilterComposer(
                $db: $db,
                $table: $db.compactionCheckpoints,
                $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
                joinBuilder: joinBuilder,
                $removeJoinBuilderFromRootComposer:
                    $removeJoinBuilderFromRootComposer,
              ),
        );
    return f(composer);
  }
}

class $$SessionsTableOrderingComposer
    extends Composer<_$AtlasDatabase, $SessionsTable> {
  $$SessionsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get workingDirectory => $composableBuilder(
    column: $table.workingDirectory,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get additionalDirectoriesJson => $composableBuilder(
    column: $table.additionalDirectoriesJson,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get lastInputTokens => $composableBuilder(
    column: $table.lastInputTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get lastOutputTokens => $composableBuilder(
    column: $table.lastOutputTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get lastTotalTokens => $composableBuilder(
    column: $table.lastTotalTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get lastCacheReadTokens => $composableBuilder(
    column: $table.lastCacheReadTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get lastCacheWriteTokens => $composableBuilder(
    column: $table.lastCacheWriteTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$SessionsTableAnnotationComposer
    extends Composer<_$AtlasDatabase, $SessionsTable> {
  $$SessionsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get title =>
      $composableBuilder(column: $table.title, builder: (column) => column);

  GeneratedColumn<String> get workingDirectory => $composableBuilder(
    column: $table.workingDirectory,
    builder: (column) => column,
  );

  GeneratedColumn<String> get additionalDirectoriesJson => $composableBuilder(
    column: $table.additionalDirectoriesJson,
    builder: (column) => column,
  );

  GeneratedColumn<int> get lastInputTokens => $composableBuilder(
    column: $table.lastInputTokens,
    builder: (column) => column,
  );

  GeneratedColumn<int> get lastOutputTokens => $composableBuilder(
    column: $table.lastOutputTokens,
    builder: (column) => column,
  );

  GeneratedColumn<int> get lastTotalTokens => $composableBuilder(
    column: $table.lastTotalTokens,
    builder: (column) => column,
  );

  GeneratedColumn<int> get lastCacheReadTokens => $composableBuilder(
    column: $table.lastCacheReadTokens,
    builder: (column) => column,
  );

  GeneratedColumn<int> get lastCacheWriteTokens => $composableBuilder(
    column: $table.lastCacheWriteTokens,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get createdAt =>
      $composableBuilder(column: $table.createdAt, builder: (column) => column);

  GeneratedColumn<DateTime> get updatedAt =>
      $composableBuilder(column: $table.updatedAt, builder: (column) => column);

  Expression<T> turnsRefs<T extends Object>(
    Expression<T> Function($$TurnsTableAnnotationComposer a) f,
  ) {
    final $$TurnsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.id,
      referencedTable: $db.turns,
      getReferencedColumn: (t) => t.sessionId,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TurnsTableAnnotationComposer(
            $db: $db,
            $table: $db.turns,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return f(composer);
  }

  Expression<T> timelineItemsRefs<T extends Object>(
    Expression<T> Function($$TimelineItemsTableAnnotationComposer a) f,
  ) {
    final $$TimelineItemsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.id,
      referencedTable: $db.timelineItems,
      getReferencedColumn: (t) => t.sessionId,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TimelineItemsTableAnnotationComposer(
            $db: $db,
            $table: $db.timelineItems,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return f(composer);
  }

  Expression<T> compactionCheckpointsRefs<T extends Object>(
    Expression<T> Function($$CompactionCheckpointsTableAnnotationComposer a) f,
  ) {
    final $$CompactionCheckpointsTableAnnotationComposer composer =
        $composerBuilder(
          composer: this,
          getCurrentColumn: (t) => t.id,
          referencedTable: $db.compactionCheckpoints,
          getReferencedColumn: (t) => t.sessionId,
          builder:
              (
                joinBuilder, {
                $addJoinBuilderToRootComposer,
                $removeJoinBuilderFromRootComposer,
              }) => $$CompactionCheckpointsTableAnnotationComposer(
                $db: $db,
                $table: $db.compactionCheckpoints,
                $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
                joinBuilder: joinBuilder,
                $removeJoinBuilderFromRootComposer:
                    $removeJoinBuilderFromRootComposer,
              ),
        );
    return f(composer);
  }
}

class $$SessionsTableTableManager
    extends
        RootTableManager<
          _$AtlasDatabase,
          $SessionsTable,
          SessionRow,
          $$SessionsTableFilterComposer,
          $$SessionsTableOrderingComposer,
          $$SessionsTableAnnotationComposer,
          $$SessionsTableCreateCompanionBuilder,
          $$SessionsTableUpdateCompanionBuilder,
          (SessionRow, $$SessionsTableReferences),
          SessionRow,
          PrefetchHooks Function({
            bool turnsRefs,
            bool timelineItemsRefs,
            bool compactionCheckpointsRefs,
          })
        > {
  $$SessionsTableTableManager(_$AtlasDatabase db, $SessionsTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$SessionsTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$SessionsTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$SessionsTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String> title = const Value.absent(),
                Value<String> workingDirectory = const Value.absent(),
                Value<String> additionalDirectoriesJson = const Value.absent(),
                Value<int> lastInputTokens = const Value.absent(),
                Value<int> lastOutputTokens = const Value.absent(),
                Value<int> lastTotalTokens = const Value.absent(),
                Value<int> lastCacheReadTokens = const Value.absent(),
                Value<int> lastCacheWriteTokens = const Value.absent(),
                Value<DateTime> createdAt = const Value.absent(),
                Value<DateTime> updatedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => SessionsCompanion(
                id: id,
                title: title,
                workingDirectory: workingDirectory,
                additionalDirectoriesJson: additionalDirectoriesJson,
                lastInputTokens: lastInputTokens,
                lastOutputTokens: lastOutputTokens,
                lastTotalTokens: lastTotalTokens,
                lastCacheReadTokens: lastCacheReadTokens,
                lastCacheWriteTokens: lastCacheWriteTokens,
                createdAt: createdAt,
                updatedAt: updatedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                Value<String> title = const Value.absent(),
                required String workingDirectory,
                Value<String> additionalDirectoriesJson = const Value.absent(),
                Value<int> lastInputTokens = const Value.absent(),
                Value<int> lastOutputTokens = const Value.absent(),
                Value<int> lastTotalTokens = const Value.absent(),
                Value<int> lastCacheReadTokens = const Value.absent(),
                Value<int> lastCacheWriteTokens = const Value.absent(),
                required DateTime createdAt,
                required DateTime updatedAt,
                Value<int> rowid = const Value.absent(),
              }) => SessionsCompanion.insert(
                id: id,
                title: title,
                workingDirectory: workingDirectory,
                additionalDirectoriesJson: additionalDirectoriesJson,
                lastInputTokens: lastInputTokens,
                lastOutputTokens: lastOutputTokens,
                lastTotalTokens: lastTotalTokens,
                lastCacheReadTokens: lastCacheReadTokens,
                lastCacheWriteTokens: lastCacheWriteTokens,
                createdAt: createdAt,
                updatedAt: updatedAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable(table),
                  $$SessionsTableReferences(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback:
              ({
                turnsRefs = false,
                timelineItemsRefs = false,
                compactionCheckpointsRefs = false,
              }) {
                return PrefetchHooks(
                  db: db,
                  explicitlyWatchedTables: [
                    if (turnsRefs) db.turns,
                    if (timelineItemsRefs) db.timelineItems,
                    if (compactionCheckpointsRefs) db.compactionCheckpoints,
                  ],
                  addJoins: null,
                  getPrefetchedDataCallback: (items) async {
                    return [
                      if (turnsRefs)
                        await $_getPrefetchedData<
                          SessionRow,
                          $SessionsTable,
                          TurnRow
                        >(
                          currentTable: table,
                          referencedTable: $$SessionsTableReferences
                              ._turnsRefsTable(db),
                          managerFromTypedResult: (p0) =>
                              $$SessionsTableReferences(
                                db,
                                table,
                                p0,
                              ).turnsRefs,
                          referencedItemsForCurrentItem:
                              (item, referencedItems) => referencedItems.where(
                                (e) => e.sessionId == item.id,
                              ),
                          typedResults: items,
                        ),
                      if (timelineItemsRefs)
                        await $_getPrefetchedData<
                          SessionRow,
                          $SessionsTable,
                          TimelineItemRow
                        >(
                          currentTable: table,
                          referencedTable: $$SessionsTableReferences
                              ._timelineItemsRefsTable(db),
                          managerFromTypedResult: (p0) =>
                              $$SessionsTableReferences(
                                db,
                                table,
                                p0,
                              ).timelineItemsRefs,
                          referencedItemsForCurrentItem:
                              (item, referencedItems) => referencedItems.where(
                                (e) => e.sessionId == item.id,
                              ),
                          typedResults: items,
                        ),
                      if (compactionCheckpointsRefs)
                        await $_getPrefetchedData<
                          SessionRow,
                          $SessionsTable,
                          CompactionCheckpointRow
                        >(
                          currentTable: table,
                          referencedTable: $$SessionsTableReferences
                              ._compactionCheckpointsRefsTable(db),
                          managerFromTypedResult: (p0) =>
                              $$SessionsTableReferences(
                                db,
                                table,
                                p0,
                              ).compactionCheckpointsRefs,
                          referencedItemsForCurrentItem:
                              (item, referencedItems) => referencedItems.where(
                                (e) => e.sessionId == item.id,
                              ),
                          typedResults: items,
                        ),
                    ];
                  },
                );
              },
        ),
      );
}

typedef $$SessionsTableProcessedTableManager =
    ProcessedTableManager<
      _$AtlasDatabase,
      $SessionsTable,
      SessionRow,
      $$SessionsTableFilterComposer,
      $$SessionsTableOrderingComposer,
      $$SessionsTableAnnotationComposer,
      $$SessionsTableCreateCompanionBuilder,
      $$SessionsTableUpdateCompanionBuilder,
      (SessionRow, $$SessionsTableReferences),
      SessionRow,
      PrefetchHooks Function({
        bool turnsRefs,
        bool timelineItemsRefs,
        bool compactionCheckpointsRefs,
      })
    >;
typedef $$TurnsTableCreateCompanionBuilder =
    TurnsCompanion Function({
      required String id,
      required String sessionId,
      required String status,
      required DateTime startedAt,
      Value<DateTime?> completedAt,
      Value<String?> providerId,
      Value<String?> modelId,
      Value<String?> reasoningEffort,
      Value<int> inputTokens,
      Value<int> outputTokens,
      Value<int> totalTokens,
      Value<int> cacheReadTokens,
      Value<int> cacheWriteTokens,
      Value<String?> failureCode,
      Value<String?> failureMessage,
      Value<String?> cancelReason,
      Value<int> rowid,
    });
typedef $$TurnsTableUpdateCompanionBuilder =
    TurnsCompanion Function({
      Value<String> id,
      Value<String> sessionId,
      Value<String> status,
      Value<DateTime> startedAt,
      Value<DateTime?> completedAt,
      Value<String?> providerId,
      Value<String?> modelId,
      Value<String?> reasoningEffort,
      Value<int> inputTokens,
      Value<int> outputTokens,
      Value<int> totalTokens,
      Value<int> cacheReadTokens,
      Value<int> cacheWriteTokens,
      Value<String?> failureCode,
      Value<String?> failureMessage,
      Value<String?> cancelReason,
      Value<int> rowid,
    });

final class $$TurnsTableReferences
    extends BaseReferences<_$AtlasDatabase, $TurnsTable, TurnRow> {
  $$TurnsTableReferences(super.$_db, super.$_table, super.$_typedResult);

  static $SessionsTable _sessionIdTable(_$AtlasDatabase db) =>
      db.sessions.createAlias('turns__session_id__sessions__id');

  $$SessionsTableProcessedTableManager get sessionId {
    final $_column = $_itemColumn<String>('session_id')!;

    final manager = $$SessionsTableTableManager(
      $_db,
      $_db.sessions,
    ).filter((f) => f.id.sqlEquals($_column));
    final item = $_typedResult.readTableOrNull(_sessionIdTable($_db));
    if (item == null) return manager;
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: [item]),
    );
  }

  static MultiTypedResultKey<$TimelineItemsTable, List<TimelineItemRow>>
  _timelineItemsRefsTable(_$AtlasDatabase db) => MultiTypedResultKey.fromTable(
    db.timelineItems,
    aliasName: 'turns__id__timeline_items__turn_id',
  );

  $$TimelineItemsTableProcessedTableManager get timelineItemsRefs {
    final manager = $$TimelineItemsTableTableManager(
      $_db,
      $_db.timelineItems,
    ).filter((f) => f.turnId.id.sqlEquals($_itemColumn<String>('id')!));

    final cache = $_typedResult.readTableOrNull(_timelineItemsRefsTable($_db));
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: cache),
    );
  }
}

class $$TurnsTableFilterComposer
    extends Composer<_$AtlasDatabase, $TurnsTable> {
  $$TurnsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get status => $composableBuilder(
    column: $table.status,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get startedAt => $composableBuilder(
    column: $table.startedAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get completedAt => $composableBuilder(
    column: $table.completedAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get modelId => $composableBuilder(
    column: $table.modelId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get reasoningEffort => $composableBuilder(
    column: $table.reasoningEffort,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get inputTokens => $composableBuilder(
    column: $table.inputTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get outputTokens => $composableBuilder(
    column: $table.outputTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get totalTokens => $composableBuilder(
    column: $table.totalTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get cacheReadTokens => $composableBuilder(
    column: $table.cacheReadTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get cacheWriteTokens => $composableBuilder(
    column: $table.cacheWriteTokens,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get failureCode => $composableBuilder(
    column: $table.failureCode,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get failureMessage => $composableBuilder(
    column: $table.failureMessage,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get cancelReason => $composableBuilder(
    column: $table.cancelReason,
    builder: (column) => ColumnFilters(column),
  );

  $$SessionsTableFilterComposer get sessionId {
    final $$SessionsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableFilterComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }

  Expression<bool> timelineItemsRefs(
    Expression<bool> Function($$TimelineItemsTableFilterComposer f) f,
  ) {
    final $$TimelineItemsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.id,
      referencedTable: $db.timelineItems,
      getReferencedColumn: (t) => t.turnId,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TimelineItemsTableFilterComposer(
            $db: $db,
            $table: $db.timelineItems,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return f(composer);
  }
}

class $$TurnsTableOrderingComposer
    extends Composer<_$AtlasDatabase, $TurnsTable> {
  $$TurnsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get status => $composableBuilder(
    column: $table.status,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get startedAt => $composableBuilder(
    column: $table.startedAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get completedAt => $composableBuilder(
    column: $table.completedAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get modelId => $composableBuilder(
    column: $table.modelId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get reasoningEffort => $composableBuilder(
    column: $table.reasoningEffort,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get inputTokens => $composableBuilder(
    column: $table.inputTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get outputTokens => $composableBuilder(
    column: $table.outputTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get totalTokens => $composableBuilder(
    column: $table.totalTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get cacheReadTokens => $composableBuilder(
    column: $table.cacheReadTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get cacheWriteTokens => $composableBuilder(
    column: $table.cacheWriteTokens,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get failureCode => $composableBuilder(
    column: $table.failureCode,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get failureMessage => $composableBuilder(
    column: $table.failureMessage,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get cancelReason => $composableBuilder(
    column: $table.cancelReason,
    builder: (column) => ColumnOrderings(column),
  );

  $$SessionsTableOrderingComposer get sessionId {
    final $$SessionsTableOrderingComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableOrderingComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }
}

class $$TurnsTableAnnotationComposer
    extends Composer<_$AtlasDatabase, $TurnsTable> {
  $$TurnsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get status =>
      $composableBuilder(column: $table.status, builder: (column) => column);

  GeneratedColumn<DateTime> get startedAt =>
      $composableBuilder(column: $table.startedAt, builder: (column) => column);

  GeneratedColumn<DateTime> get completedAt => $composableBuilder(
    column: $table.completedAt,
    builder: (column) => column,
  );

  GeneratedColumn<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get modelId =>
      $composableBuilder(column: $table.modelId, builder: (column) => column);

  GeneratedColumn<String> get reasoningEffort => $composableBuilder(
    column: $table.reasoningEffort,
    builder: (column) => column,
  );

  GeneratedColumn<int> get inputTokens => $composableBuilder(
    column: $table.inputTokens,
    builder: (column) => column,
  );

  GeneratedColumn<int> get outputTokens => $composableBuilder(
    column: $table.outputTokens,
    builder: (column) => column,
  );

  GeneratedColumn<int> get totalTokens => $composableBuilder(
    column: $table.totalTokens,
    builder: (column) => column,
  );

  GeneratedColumn<int> get cacheReadTokens => $composableBuilder(
    column: $table.cacheReadTokens,
    builder: (column) => column,
  );

  GeneratedColumn<int> get cacheWriteTokens => $composableBuilder(
    column: $table.cacheWriteTokens,
    builder: (column) => column,
  );

  GeneratedColumn<String> get failureCode => $composableBuilder(
    column: $table.failureCode,
    builder: (column) => column,
  );

  GeneratedColumn<String> get failureMessage => $composableBuilder(
    column: $table.failureMessage,
    builder: (column) => column,
  );

  GeneratedColumn<String> get cancelReason => $composableBuilder(
    column: $table.cancelReason,
    builder: (column) => column,
  );

  $$SessionsTableAnnotationComposer get sessionId {
    final $$SessionsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableAnnotationComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }

  Expression<T> timelineItemsRefs<T extends Object>(
    Expression<T> Function($$TimelineItemsTableAnnotationComposer a) f,
  ) {
    final $$TimelineItemsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.id,
      referencedTable: $db.timelineItems,
      getReferencedColumn: (t) => t.turnId,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TimelineItemsTableAnnotationComposer(
            $db: $db,
            $table: $db.timelineItems,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return f(composer);
  }
}

class $$TurnsTableTableManager
    extends
        RootTableManager<
          _$AtlasDatabase,
          $TurnsTable,
          TurnRow,
          $$TurnsTableFilterComposer,
          $$TurnsTableOrderingComposer,
          $$TurnsTableAnnotationComposer,
          $$TurnsTableCreateCompanionBuilder,
          $$TurnsTableUpdateCompanionBuilder,
          (TurnRow, $$TurnsTableReferences),
          TurnRow,
          PrefetchHooks Function({bool sessionId, bool timelineItemsRefs})
        > {
  $$TurnsTableTableManager(_$AtlasDatabase db, $TurnsTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$TurnsTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$TurnsTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$TurnsTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String> sessionId = const Value.absent(),
                Value<String> status = const Value.absent(),
                Value<DateTime> startedAt = const Value.absent(),
                Value<DateTime?> completedAt = const Value.absent(),
                Value<String?> providerId = const Value.absent(),
                Value<String?> modelId = const Value.absent(),
                Value<String?> reasoningEffort = const Value.absent(),
                Value<int> inputTokens = const Value.absent(),
                Value<int> outputTokens = const Value.absent(),
                Value<int> totalTokens = const Value.absent(),
                Value<int> cacheReadTokens = const Value.absent(),
                Value<int> cacheWriteTokens = const Value.absent(),
                Value<String?> failureCode = const Value.absent(),
                Value<String?> failureMessage = const Value.absent(),
                Value<String?> cancelReason = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => TurnsCompanion(
                id: id,
                sessionId: sessionId,
                status: status,
                startedAt: startedAt,
                completedAt: completedAt,
                providerId: providerId,
                modelId: modelId,
                reasoningEffort: reasoningEffort,
                inputTokens: inputTokens,
                outputTokens: outputTokens,
                totalTokens: totalTokens,
                cacheReadTokens: cacheReadTokens,
                cacheWriteTokens: cacheWriteTokens,
                failureCode: failureCode,
                failureMessage: failureMessage,
                cancelReason: cancelReason,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                required String sessionId,
                required String status,
                required DateTime startedAt,
                Value<DateTime?> completedAt = const Value.absent(),
                Value<String?> providerId = const Value.absent(),
                Value<String?> modelId = const Value.absent(),
                Value<String?> reasoningEffort = const Value.absent(),
                Value<int> inputTokens = const Value.absent(),
                Value<int> outputTokens = const Value.absent(),
                Value<int> totalTokens = const Value.absent(),
                Value<int> cacheReadTokens = const Value.absent(),
                Value<int> cacheWriteTokens = const Value.absent(),
                Value<String?> failureCode = const Value.absent(),
                Value<String?> failureMessage = const Value.absent(),
                Value<String?> cancelReason = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => TurnsCompanion.insert(
                id: id,
                sessionId: sessionId,
                status: status,
                startedAt: startedAt,
                completedAt: completedAt,
                providerId: providerId,
                modelId: modelId,
                reasoningEffort: reasoningEffort,
                inputTokens: inputTokens,
                outputTokens: outputTokens,
                totalTokens: totalTokens,
                cacheReadTokens: cacheReadTokens,
                cacheWriteTokens: cacheWriteTokens,
                failureCode: failureCode,
                failureMessage: failureMessage,
                cancelReason: cancelReason,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) =>
                    (e.readTable(table), $$TurnsTableReferences(db, table, e)),
              )
              .toList(),
          prefetchHooksCallback:
              ({sessionId = false, timelineItemsRefs = false}) {
                return PrefetchHooks(
                  db: db,
                  explicitlyWatchedTables: [
                    if (timelineItemsRefs) db.timelineItems,
                  ],
                  addJoins:
                      <
                        T extends TableManagerState<
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic
                        >
                      >(state) {
                        if (sessionId) {
                          state =
                              state.withJoin(
                                    currentTable: table,
                                    currentColumn: table.sessionId,
                                    referencedTable: $$TurnsTableReferences
                                        ._sessionIdTable(db),
                                    referencedColumn: $$TurnsTableReferences
                                        ._sessionIdTable(db)
                                        .id,
                                  )
                                  as T;
                        }

                        return state;
                      },
                  getPrefetchedDataCallback: (items) async {
                    return [
                      if (timelineItemsRefs)
                        await $_getPrefetchedData<
                          TurnRow,
                          $TurnsTable,
                          TimelineItemRow
                        >(
                          currentTable: table,
                          referencedTable: $$TurnsTableReferences
                              ._timelineItemsRefsTable(db),
                          managerFromTypedResult: (p0) =>
                              $$TurnsTableReferences(
                                db,
                                table,
                                p0,
                              ).timelineItemsRefs,
                          referencedItemsForCurrentItem:
                              (item, referencedItems) => referencedItems.where(
                                (e) => e.turnId == item.id,
                              ),
                          typedResults: items,
                        ),
                    ];
                  },
                );
              },
        ),
      );
}

typedef $$TurnsTableProcessedTableManager =
    ProcessedTableManager<
      _$AtlasDatabase,
      $TurnsTable,
      TurnRow,
      $$TurnsTableFilterComposer,
      $$TurnsTableOrderingComposer,
      $$TurnsTableAnnotationComposer,
      $$TurnsTableCreateCompanionBuilder,
      $$TurnsTableUpdateCompanionBuilder,
      (TurnRow, $$TurnsTableReferences),
      TurnRow,
      PrefetchHooks Function({bool sessionId, bool timelineItemsRefs})
    >;
typedef $$TimelineItemsTableCreateCompanionBuilder =
    TimelineItemsCompanion Function({
      required String id,
      required String sessionId,
      required String turnId,
      required int sequence,
      required String kind,
      Value<int> payloadVersion,
      required String payloadJson,
      required DateTime occurredAt,
      Value<int> rowid,
    });
typedef $$TimelineItemsTableUpdateCompanionBuilder =
    TimelineItemsCompanion Function({
      Value<String> id,
      Value<String> sessionId,
      Value<String> turnId,
      Value<int> sequence,
      Value<String> kind,
      Value<int> payloadVersion,
      Value<String> payloadJson,
      Value<DateTime> occurredAt,
      Value<int> rowid,
    });

final class $$TimelineItemsTableReferences
    extends
        BaseReferences<_$AtlasDatabase, $TimelineItemsTable, TimelineItemRow> {
  $$TimelineItemsTableReferences(
    super.$_db,
    super.$_table,
    super.$_typedResult,
  );

  static $SessionsTable _sessionIdTable(_$AtlasDatabase db) =>
      db.sessions.createAlias('timeline_items__session_id__sessions__id');

  $$SessionsTableProcessedTableManager get sessionId {
    final $_column = $_itemColumn<String>('session_id')!;

    final manager = $$SessionsTableTableManager(
      $_db,
      $_db.sessions,
    ).filter((f) => f.id.sqlEquals($_column));
    final item = $_typedResult.readTableOrNull(_sessionIdTable($_db));
    if (item == null) return manager;
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: [item]),
    );
  }

  static $TurnsTable _turnIdTable(_$AtlasDatabase db) =>
      db.turns.createAlias('timeline_items__turn_id__turns__id');

  $$TurnsTableProcessedTableManager get turnId {
    final $_column = $_itemColumn<String>('turn_id')!;

    final manager = $$TurnsTableTableManager(
      $_db,
      $_db.turns,
    ).filter((f) => f.id.sqlEquals($_column));
    final item = $_typedResult.readTableOrNull(_turnIdTable($_db));
    if (item == null) return manager;
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: [item]),
    );
  }

  static MultiTypedResultKey<$ModelCheckpointsTable, List<ModelCheckpointRow>>
  _modelCheckpointsRefsTable(_$AtlasDatabase db) =>
      MultiTypedResultKey.fromTable(
        db.modelCheckpoints,
        aliasName: 'timeline_items__id__model_checkpoints__timeline_item_id',
      );

  $$ModelCheckpointsTableProcessedTableManager get modelCheckpointsRefs {
    final manager = $$ModelCheckpointsTableTableManager(
      $_db,
      $_db.modelCheckpoints,
    ).filter((f) => f.timelineItemId.id.sqlEquals($_itemColumn<String>('id')!));

    final cache = $_typedResult.readTableOrNull(
      _modelCheckpointsRefsTable($_db),
    );
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: cache),
    );
  }
}

class $$TimelineItemsTableFilterComposer
    extends Composer<_$AtlasDatabase, $TimelineItemsTable> {
  $$TimelineItemsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get sequence => $composableBuilder(
    column: $table.sequence,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get kind => $composableBuilder(
    column: $table.kind,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get payloadVersion => $composableBuilder(
    column: $table.payloadVersion,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get payloadJson => $composableBuilder(
    column: $table.payloadJson,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get occurredAt => $composableBuilder(
    column: $table.occurredAt,
    builder: (column) => ColumnFilters(column),
  );

  $$SessionsTableFilterComposer get sessionId {
    final $$SessionsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableFilterComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }

  $$TurnsTableFilterComposer get turnId {
    final $$TurnsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.turnId,
      referencedTable: $db.turns,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TurnsTableFilterComposer(
            $db: $db,
            $table: $db.turns,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }

  Expression<bool> modelCheckpointsRefs(
    Expression<bool> Function($$ModelCheckpointsTableFilterComposer f) f,
  ) {
    final $$ModelCheckpointsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.id,
      referencedTable: $db.modelCheckpoints,
      getReferencedColumn: (t) => t.timelineItemId,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$ModelCheckpointsTableFilterComposer(
            $db: $db,
            $table: $db.modelCheckpoints,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return f(composer);
  }
}

class $$TimelineItemsTableOrderingComposer
    extends Composer<_$AtlasDatabase, $TimelineItemsTable> {
  $$TimelineItemsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get sequence => $composableBuilder(
    column: $table.sequence,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get kind => $composableBuilder(
    column: $table.kind,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get payloadVersion => $composableBuilder(
    column: $table.payloadVersion,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get payloadJson => $composableBuilder(
    column: $table.payloadJson,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get occurredAt => $composableBuilder(
    column: $table.occurredAt,
    builder: (column) => ColumnOrderings(column),
  );

  $$SessionsTableOrderingComposer get sessionId {
    final $$SessionsTableOrderingComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableOrderingComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }

  $$TurnsTableOrderingComposer get turnId {
    final $$TurnsTableOrderingComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.turnId,
      referencedTable: $db.turns,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TurnsTableOrderingComposer(
            $db: $db,
            $table: $db.turns,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }
}

class $$TimelineItemsTableAnnotationComposer
    extends Composer<_$AtlasDatabase, $TimelineItemsTable> {
  $$TimelineItemsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<int> get sequence =>
      $composableBuilder(column: $table.sequence, builder: (column) => column);

  GeneratedColumn<String> get kind =>
      $composableBuilder(column: $table.kind, builder: (column) => column);

  GeneratedColumn<int> get payloadVersion => $composableBuilder(
    column: $table.payloadVersion,
    builder: (column) => column,
  );

  GeneratedColumn<String> get payloadJson => $composableBuilder(
    column: $table.payloadJson,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get occurredAt => $composableBuilder(
    column: $table.occurredAt,
    builder: (column) => column,
  );

  $$SessionsTableAnnotationComposer get sessionId {
    final $$SessionsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableAnnotationComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }

  $$TurnsTableAnnotationComposer get turnId {
    final $$TurnsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.turnId,
      referencedTable: $db.turns,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TurnsTableAnnotationComposer(
            $db: $db,
            $table: $db.turns,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }

  Expression<T> modelCheckpointsRefs<T extends Object>(
    Expression<T> Function($$ModelCheckpointsTableAnnotationComposer a) f,
  ) {
    final $$ModelCheckpointsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.id,
      referencedTable: $db.modelCheckpoints,
      getReferencedColumn: (t) => t.timelineItemId,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$ModelCheckpointsTableAnnotationComposer(
            $db: $db,
            $table: $db.modelCheckpoints,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return f(composer);
  }
}

class $$TimelineItemsTableTableManager
    extends
        RootTableManager<
          _$AtlasDatabase,
          $TimelineItemsTable,
          TimelineItemRow,
          $$TimelineItemsTableFilterComposer,
          $$TimelineItemsTableOrderingComposer,
          $$TimelineItemsTableAnnotationComposer,
          $$TimelineItemsTableCreateCompanionBuilder,
          $$TimelineItemsTableUpdateCompanionBuilder,
          (TimelineItemRow, $$TimelineItemsTableReferences),
          TimelineItemRow,
          PrefetchHooks Function({
            bool sessionId,
            bool turnId,
            bool modelCheckpointsRefs,
          })
        > {
  $$TimelineItemsTableTableManager(
    _$AtlasDatabase db,
    $TimelineItemsTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$TimelineItemsTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$TimelineItemsTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$TimelineItemsTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String> sessionId = const Value.absent(),
                Value<String> turnId = const Value.absent(),
                Value<int> sequence = const Value.absent(),
                Value<String> kind = const Value.absent(),
                Value<int> payloadVersion = const Value.absent(),
                Value<String> payloadJson = const Value.absent(),
                Value<DateTime> occurredAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => TimelineItemsCompanion(
                id: id,
                sessionId: sessionId,
                turnId: turnId,
                sequence: sequence,
                kind: kind,
                payloadVersion: payloadVersion,
                payloadJson: payloadJson,
                occurredAt: occurredAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                required String sessionId,
                required String turnId,
                required int sequence,
                required String kind,
                Value<int> payloadVersion = const Value.absent(),
                required String payloadJson,
                required DateTime occurredAt,
                Value<int> rowid = const Value.absent(),
              }) => TimelineItemsCompanion.insert(
                id: id,
                sessionId: sessionId,
                turnId: turnId,
                sequence: sequence,
                kind: kind,
                payloadVersion: payloadVersion,
                payloadJson: payloadJson,
                occurredAt: occurredAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable(table),
                  $$TimelineItemsTableReferences(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback:
              ({
                sessionId = false,
                turnId = false,
                modelCheckpointsRefs = false,
              }) {
                return PrefetchHooks(
                  db: db,
                  explicitlyWatchedTables: [
                    if (modelCheckpointsRefs) db.modelCheckpoints,
                  ],
                  addJoins:
                      <
                        T extends TableManagerState<
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic,
                          dynamic
                        >
                      >(state) {
                        if (sessionId) {
                          state =
                              state.withJoin(
                                    currentTable: table,
                                    currentColumn: table.sessionId,
                                    referencedTable:
                                        $$TimelineItemsTableReferences
                                            ._sessionIdTable(db),
                                    referencedColumn:
                                        $$TimelineItemsTableReferences
                                            ._sessionIdTable(db)
                                            .id,
                                  )
                                  as T;
                        }
                        if (turnId) {
                          state =
                              state.withJoin(
                                    currentTable: table,
                                    currentColumn: table.turnId,
                                    referencedTable:
                                        $$TimelineItemsTableReferences
                                            ._turnIdTable(db),
                                    referencedColumn:
                                        $$TimelineItemsTableReferences
                                            ._turnIdTable(db)
                                            .id,
                                  )
                                  as T;
                        }

                        return state;
                      },
                  getPrefetchedDataCallback: (items) async {
                    return [
                      if (modelCheckpointsRefs)
                        await $_getPrefetchedData<
                          TimelineItemRow,
                          $TimelineItemsTable,
                          ModelCheckpointRow
                        >(
                          currentTable: table,
                          referencedTable: $$TimelineItemsTableReferences
                              ._modelCheckpointsRefsTable(db),
                          managerFromTypedResult: (p0) =>
                              $$TimelineItemsTableReferences(
                                db,
                                table,
                                p0,
                              ).modelCheckpointsRefs,
                          referencedItemsForCurrentItem:
                              (item, referencedItems) => referencedItems.where(
                                (e) => e.timelineItemId == item.id,
                              ),
                          typedResults: items,
                        ),
                    ];
                  },
                );
              },
        ),
      );
}

typedef $$TimelineItemsTableProcessedTableManager =
    ProcessedTableManager<
      _$AtlasDatabase,
      $TimelineItemsTable,
      TimelineItemRow,
      $$TimelineItemsTableFilterComposer,
      $$TimelineItemsTableOrderingComposer,
      $$TimelineItemsTableAnnotationComposer,
      $$TimelineItemsTableCreateCompanionBuilder,
      $$TimelineItemsTableUpdateCompanionBuilder,
      (TimelineItemRow, $$TimelineItemsTableReferences),
      TimelineItemRow,
      PrefetchHooks Function({
        bool sessionId,
        bool turnId,
        bool modelCheckpointsRefs,
      })
    >;
typedef $$ModelCheckpointsTableCreateCompanionBuilder =
    ModelCheckpointsCompanion Function({
      required String timelineItemId,
      required String providerId,
      Value<String> reasoningSummary,
      Value<String> payloadJson,
      required DateTime createdAt,
      Value<int> rowid,
    });
typedef $$ModelCheckpointsTableUpdateCompanionBuilder =
    ModelCheckpointsCompanion Function({
      Value<String> timelineItemId,
      Value<String> providerId,
      Value<String> reasoningSummary,
      Value<String> payloadJson,
      Value<DateTime> createdAt,
      Value<int> rowid,
    });

final class $$ModelCheckpointsTableReferences
    extends
        BaseReferences<
          _$AtlasDatabase,
          $ModelCheckpointsTable,
          ModelCheckpointRow
        > {
  $$ModelCheckpointsTableReferences(
    super.$_db,
    super.$_table,
    super.$_typedResult,
  );

  static $TimelineItemsTable _timelineItemIdTable(_$AtlasDatabase db) => db
      .timelineItems
      .createAlias('model_checkpoints__timeline_item_id__timeline_items__id');

  $$TimelineItemsTableProcessedTableManager get timelineItemId {
    final $_column = $_itemColumn<String>('timeline_item_id')!;

    final manager = $$TimelineItemsTableTableManager(
      $_db,
      $_db.timelineItems,
    ).filter((f) => f.id.sqlEquals($_column));
    final item = $_typedResult.readTableOrNull(_timelineItemIdTable($_db));
    if (item == null) return manager;
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: [item]),
    );
  }
}

class $$ModelCheckpointsTableFilterComposer
    extends Composer<_$AtlasDatabase, $ModelCheckpointsTable> {
  $$ModelCheckpointsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get reasoningSummary => $composableBuilder(
    column: $table.reasoningSummary,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get payloadJson => $composableBuilder(
    column: $table.payloadJson,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnFilters(column),
  );

  $$TimelineItemsTableFilterComposer get timelineItemId {
    final $$TimelineItemsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.timelineItemId,
      referencedTable: $db.timelineItems,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TimelineItemsTableFilterComposer(
            $db: $db,
            $table: $db.timelineItems,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }
}

class $$ModelCheckpointsTableOrderingComposer
    extends Composer<_$AtlasDatabase, $ModelCheckpointsTable> {
  $$ModelCheckpointsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get reasoningSummary => $composableBuilder(
    column: $table.reasoningSummary,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get payloadJson => $composableBuilder(
    column: $table.payloadJson,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnOrderings(column),
  );

  $$TimelineItemsTableOrderingComposer get timelineItemId {
    final $$TimelineItemsTableOrderingComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.timelineItemId,
      referencedTable: $db.timelineItems,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TimelineItemsTableOrderingComposer(
            $db: $db,
            $table: $db.timelineItems,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }
}

class $$ModelCheckpointsTableAnnotationComposer
    extends Composer<_$AtlasDatabase, $ModelCheckpointsTable> {
  $$ModelCheckpointsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get reasoningSummary => $composableBuilder(
    column: $table.reasoningSummary,
    builder: (column) => column,
  );

  GeneratedColumn<String> get payloadJson => $composableBuilder(
    column: $table.payloadJson,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get createdAt =>
      $composableBuilder(column: $table.createdAt, builder: (column) => column);

  $$TimelineItemsTableAnnotationComposer get timelineItemId {
    final $$TimelineItemsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.timelineItemId,
      referencedTable: $db.timelineItems,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$TimelineItemsTableAnnotationComposer(
            $db: $db,
            $table: $db.timelineItems,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }
}

class $$ModelCheckpointsTableTableManager
    extends
        RootTableManager<
          _$AtlasDatabase,
          $ModelCheckpointsTable,
          ModelCheckpointRow,
          $$ModelCheckpointsTableFilterComposer,
          $$ModelCheckpointsTableOrderingComposer,
          $$ModelCheckpointsTableAnnotationComposer,
          $$ModelCheckpointsTableCreateCompanionBuilder,
          $$ModelCheckpointsTableUpdateCompanionBuilder,
          (ModelCheckpointRow, $$ModelCheckpointsTableReferences),
          ModelCheckpointRow,
          PrefetchHooks Function({bool timelineItemId})
        > {
  $$ModelCheckpointsTableTableManager(
    _$AtlasDatabase db,
    $ModelCheckpointsTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$ModelCheckpointsTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$ModelCheckpointsTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$ModelCheckpointsTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> timelineItemId = const Value.absent(),
                Value<String> providerId = const Value.absent(),
                Value<String> reasoningSummary = const Value.absent(),
                Value<String> payloadJson = const Value.absent(),
                Value<DateTime> createdAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => ModelCheckpointsCompanion(
                timelineItemId: timelineItemId,
                providerId: providerId,
                reasoningSummary: reasoningSummary,
                payloadJson: payloadJson,
                createdAt: createdAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String timelineItemId,
                required String providerId,
                Value<String> reasoningSummary = const Value.absent(),
                Value<String> payloadJson = const Value.absent(),
                required DateTime createdAt,
                Value<int> rowid = const Value.absent(),
              }) => ModelCheckpointsCompanion.insert(
                timelineItemId: timelineItemId,
                providerId: providerId,
                reasoningSummary: reasoningSummary,
                payloadJson: payloadJson,
                createdAt: createdAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable(table),
                  $$ModelCheckpointsTableReferences(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback: ({timelineItemId = false}) {
            return PrefetchHooks(
              db: db,
              explicitlyWatchedTables: [],
              addJoins:
                  <
                    T extends TableManagerState<
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic
                    >
                  >(state) {
                    if (timelineItemId) {
                      state =
                          state.withJoin(
                                currentTable: table,
                                currentColumn: table.timelineItemId,
                                referencedTable:
                                    $$ModelCheckpointsTableReferences
                                        ._timelineItemIdTable(db),
                                referencedColumn:
                                    $$ModelCheckpointsTableReferences
                                        ._timelineItemIdTable(db)
                                        .id,
                              )
                              as T;
                    }

                    return state;
                  },
              getPrefetchedDataCallback: (items) async {
                return [];
              },
            );
          },
        ),
      );
}

typedef $$ModelCheckpointsTableProcessedTableManager =
    ProcessedTableManager<
      _$AtlasDatabase,
      $ModelCheckpointsTable,
      ModelCheckpointRow,
      $$ModelCheckpointsTableFilterComposer,
      $$ModelCheckpointsTableOrderingComposer,
      $$ModelCheckpointsTableAnnotationComposer,
      $$ModelCheckpointsTableCreateCompanionBuilder,
      $$ModelCheckpointsTableUpdateCompanionBuilder,
      (ModelCheckpointRow, $$ModelCheckpointsTableReferences),
      ModelCheckpointRow,
      PrefetchHooks Function({bool timelineItemId})
    >;
typedef $$CompactionCheckpointsTableCreateCompanionBuilder =
    CompactionCheckpointsCompanion Function({
      required String sessionId,
      required int compactedThroughSequence,
      required String summary,
      required int inputTokensBefore,
      required int inputTokensAfter,
      required DateTime createdAt,
      Value<int> rowid,
    });
typedef $$CompactionCheckpointsTableUpdateCompanionBuilder =
    CompactionCheckpointsCompanion Function({
      Value<String> sessionId,
      Value<int> compactedThroughSequence,
      Value<String> summary,
      Value<int> inputTokensBefore,
      Value<int> inputTokensAfter,
      Value<DateTime> createdAt,
      Value<int> rowid,
    });

final class $$CompactionCheckpointsTableReferences
    extends
        BaseReferences<
          _$AtlasDatabase,
          $CompactionCheckpointsTable,
          CompactionCheckpointRow
        > {
  $$CompactionCheckpointsTableReferences(
    super.$_db,
    super.$_table,
    super.$_typedResult,
  );

  static $SessionsTable _sessionIdTable(_$AtlasDatabase db) => db.sessions
      .createAlias('compaction_checkpoints__session_id__sessions__id');

  $$SessionsTableProcessedTableManager get sessionId {
    final $_column = $_itemColumn<String>('session_id')!;

    final manager = $$SessionsTableTableManager(
      $_db,
      $_db.sessions,
    ).filter((f) => f.id.sqlEquals($_column));
    final item = $_typedResult.readTableOrNull(_sessionIdTable($_db));
    if (item == null) return manager;
    return ProcessedTableManager(
      manager.$state.copyWith(prefetchedData: [item]),
    );
  }
}

class $$CompactionCheckpointsTableFilterComposer
    extends Composer<_$AtlasDatabase, $CompactionCheckpointsTable> {
  $$CompactionCheckpointsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<int> get compactedThroughSequence => $composableBuilder(
    column: $table.compactedThroughSequence,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get summary => $composableBuilder(
    column: $table.summary,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get inputTokensBefore => $composableBuilder(
    column: $table.inputTokensBefore,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get inputTokensAfter => $composableBuilder(
    column: $table.inputTokensAfter,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnFilters(column),
  );

  $$SessionsTableFilterComposer get sessionId {
    final $$SessionsTableFilterComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableFilterComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }
}

class $$CompactionCheckpointsTableOrderingComposer
    extends Composer<_$AtlasDatabase, $CompactionCheckpointsTable> {
  $$CompactionCheckpointsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<int> get compactedThroughSequence => $composableBuilder(
    column: $table.compactedThroughSequence,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get summary => $composableBuilder(
    column: $table.summary,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get inputTokensBefore => $composableBuilder(
    column: $table.inputTokensBefore,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get inputTokensAfter => $composableBuilder(
    column: $table.inputTokensAfter,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnOrderings(column),
  );

  $$SessionsTableOrderingComposer get sessionId {
    final $$SessionsTableOrderingComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableOrderingComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }
}

class $$CompactionCheckpointsTableAnnotationComposer
    extends Composer<_$AtlasDatabase, $CompactionCheckpointsTable> {
  $$CompactionCheckpointsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<int> get compactedThroughSequence => $composableBuilder(
    column: $table.compactedThroughSequence,
    builder: (column) => column,
  );

  GeneratedColumn<String> get summary =>
      $composableBuilder(column: $table.summary, builder: (column) => column);

  GeneratedColumn<int> get inputTokensBefore => $composableBuilder(
    column: $table.inputTokensBefore,
    builder: (column) => column,
  );

  GeneratedColumn<int> get inputTokensAfter => $composableBuilder(
    column: $table.inputTokensAfter,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get createdAt =>
      $composableBuilder(column: $table.createdAt, builder: (column) => column);

  $$SessionsTableAnnotationComposer get sessionId {
    final $$SessionsTableAnnotationComposer composer = $composerBuilder(
      composer: this,
      getCurrentColumn: (t) => t.sessionId,
      referencedTable: $db.sessions,
      getReferencedColumn: (t) => t.id,
      builder:
          (
            joinBuilder, {
            $addJoinBuilderToRootComposer,
            $removeJoinBuilderFromRootComposer,
          }) => $$SessionsTableAnnotationComposer(
            $db: $db,
            $table: $db.sessions,
            $addJoinBuilderToRootComposer: $addJoinBuilderToRootComposer,
            joinBuilder: joinBuilder,
            $removeJoinBuilderFromRootComposer:
                $removeJoinBuilderFromRootComposer,
          ),
    );
    return composer;
  }
}

class $$CompactionCheckpointsTableTableManager
    extends
        RootTableManager<
          _$AtlasDatabase,
          $CompactionCheckpointsTable,
          CompactionCheckpointRow,
          $$CompactionCheckpointsTableFilterComposer,
          $$CompactionCheckpointsTableOrderingComposer,
          $$CompactionCheckpointsTableAnnotationComposer,
          $$CompactionCheckpointsTableCreateCompanionBuilder,
          $$CompactionCheckpointsTableUpdateCompanionBuilder,
          (CompactionCheckpointRow, $$CompactionCheckpointsTableReferences),
          CompactionCheckpointRow,
          PrefetchHooks Function({bool sessionId})
        > {
  $$CompactionCheckpointsTableTableManager(
    _$AtlasDatabase db,
    $CompactionCheckpointsTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$CompactionCheckpointsTableFilterComposer(
                $db: db,
                $table: table,
              ),
          createOrderingComposer: () =>
              $$CompactionCheckpointsTableOrderingComposer(
                $db: db,
                $table: table,
              ),
          createComputedFieldComposer: () =>
              $$CompactionCheckpointsTableAnnotationComposer(
                $db: db,
                $table: table,
              ),
          updateCompanionCallback:
              ({
                Value<String> sessionId = const Value.absent(),
                Value<int> compactedThroughSequence = const Value.absent(),
                Value<String> summary = const Value.absent(),
                Value<int> inputTokensBefore = const Value.absent(),
                Value<int> inputTokensAfter = const Value.absent(),
                Value<DateTime> createdAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => CompactionCheckpointsCompanion(
                sessionId: sessionId,
                compactedThroughSequence: compactedThroughSequence,
                summary: summary,
                inputTokensBefore: inputTokensBefore,
                inputTokensAfter: inputTokensAfter,
                createdAt: createdAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String sessionId,
                required int compactedThroughSequence,
                required String summary,
                required int inputTokensBefore,
                required int inputTokensAfter,
                required DateTime createdAt,
                Value<int> rowid = const Value.absent(),
              }) => CompactionCheckpointsCompanion.insert(
                sessionId: sessionId,
                compactedThroughSequence: compactedThroughSequence,
                summary: summary,
                inputTokensBefore: inputTokensBefore,
                inputTokensAfter: inputTokensAfter,
                createdAt: createdAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map(
                (e) => (
                  e.readTable(table),
                  $$CompactionCheckpointsTableReferences(db, table, e),
                ),
              )
              .toList(),
          prefetchHooksCallback: ({sessionId = false}) {
            return PrefetchHooks(
              db: db,
              explicitlyWatchedTables: [],
              addJoins:
                  <
                    T extends TableManagerState<
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic,
                      dynamic
                    >
                  >(state) {
                    if (sessionId) {
                      state =
                          state.withJoin(
                                currentTable: table,
                                currentColumn: table.sessionId,
                                referencedTable:
                                    $$CompactionCheckpointsTableReferences
                                        ._sessionIdTable(db),
                                referencedColumn:
                                    $$CompactionCheckpointsTableReferences
                                        ._sessionIdTable(db)
                                        .id,
                              )
                              as T;
                    }

                    return state;
                  },
              getPrefetchedDataCallback: (items) async {
                return [];
              },
            );
          },
        ),
      );
}

typedef $$CompactionCheckpointsTableProcessedTableManager =
    ProcessedTableManager<
      _$AtlasDatabase,
      $CompactionCheckpointsTable,
      CompactionCheckpointRow,
      $$CompactionCheckpointsTableFilterComposer,
      $$CompactionCheckpointsTableOrderingComposer,
      $$CompactionCheckpointsTableAnnotationComposer,
      $$CompactionCheckpointsTableCreateCompanionBuilder,
      $$CompactionCheckpointsTableUpdateCompanionBuilder,
      (CompactionCheckpointRow, $$CompactionCheckpointsTableReferences),
      CompactionCheckpointRow,
      PrefetchHooks Function({bool sessionId})
    >;

class $AtlasDatabaseManager {
  final _$AtlasDatabase _db;
  $AtlasDatabaseManager(this._db);
  $$SessionsTableTableManager get sessions =>
      $$SessionsTableTableManager(_db, _db.sessions);
  $$TurnsTableTableManager get turns =>
      $$TurnsTableTableManager(_db, _db.turns);
  $$TimelineItemsTableTableManager get timelineItems =>
      $$TimelineItemsTableTableManager(_db, _db.timelineItems);
  $$ModelCheckpointsTableTableManager get modelCheckpoints =>
      $$ModelCheckpointsTableTableManager(_db, _db.modelCheckpoints);
  $$CompactionCheckpointsTableTableManager get compactionCheckpoints =>
      $$CompactionCheckpointsTableTableManager(_db, _db.compactionCheckpoints);
}
