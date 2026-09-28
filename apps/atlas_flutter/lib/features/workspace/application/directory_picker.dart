import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:atlas_flutter/features/workspace/data/directory_picker_service.dart';

/// Picks a working directory for a new session; overridable in tests.
final directoryPickerProvider = Provider<Future<String?> Function()>(
  (ref) => const DirectoryPickerService().pick,
);
