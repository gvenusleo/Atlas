import 'package:clipboard/clipboard.dart';
import 'package:file_picker/file_picker.dart';

import 'package:atlas_flutter/features/workspace/domain/image_attachment.dart';

/// Opens a file dialog for PNG, JPEG, WebP, and GIF images.
Future<List<PendingImage>> pickImageFiles() async {
  final files = await FilePicker.pickFiles(
    type: FileType.custom,
    allowedExtensions: const ['png', 'jpg', 'jpeg', 'webp', 'gif'],
  );
  final images = <PendingImage>[];
  for (final file in files) {
    final bytes = await file.readAsBytes();
    final mimeType = imageMimeType(name: file.name, bytes: bytes);
    if (mimeType == null) {
      continue;
    }
    images.add(PendingImage(bytes: bytes, mimeType: mimeType, name: file.name));
  }
  return images;
}

/// Reads an image from the system clipboard.
///
/// The clipboard package exposes a single image without a MIME type, so the
/// format is sniffed from magic bytes before the image is accepted.
Future<List<PendingImage>> readClipboardImages() async {
  try {
    final bytes = await FlutterClipboard.pasteImage();
    if (bytes == null || bytes.isEmpty) {
      return const [];
    }
    final mimeType = imageMimeType(bytes: bytes);
    if (mimeType == null) {
      return const [];
    }
    return [PendingImage(bytes: bytes, mimeType: mimeType)];
  } catch (_) {
    return const [];
  }
}
