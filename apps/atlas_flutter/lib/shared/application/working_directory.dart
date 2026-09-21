import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Working directory shared by connection selection and workspace drafts.
final workspaceWorkingDirectoryProvider =
    NotifierProvider<WorkspaceWorkingDirectory, String>(
      WorkspaceWorkingDirectory.new,
    );

/// Holds the directory for subsequent sessions, initially the user's home.
class WorkspaceWorkingDirectory extends Notifier<String> {
  @override
  String build() {
    final home =
        Platform.environment['HOME'] ?? Platform.environment['USERPROFILE'];
    return home == null || home.isEmpty ? Directory.current.path : home;
  }

  /// Switches the working directory for subsequent sessions.
  void set(String directory) => state = directory;
}
