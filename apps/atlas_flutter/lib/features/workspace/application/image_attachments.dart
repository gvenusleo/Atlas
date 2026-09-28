import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:atlas_flutter/features/workspace/data/image_attachment.dart';
import 'package:atlas_flutter/features/workspace/domain/image_attachment.dart';

/// Picks image files from disk; overridable in tests.
final imagePickerProvider = Provider<Future<List<PendingImage>> Function()>(
  (ref) => pickImageFiles,
);

/// Reads images from the system clipboard; overridable in tests.
final imageClipboardProvider = Provider<Future<List<PendingImage>> Function()>(
  (ref) => readClipboardImages,
);
