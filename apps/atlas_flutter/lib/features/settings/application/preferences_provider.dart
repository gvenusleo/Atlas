import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:atlas_flutter/features/settings/data/preferences_repository.dart';

/// Settings repository loaded before the first frame by the composition root.
final preferencesRepositoryProvider = Provider<PreferencesRepository>(
  (ref) => const PreferencesRepository(),
);
