import 'dart:convert';
import 'dart:typed_data';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:clipboard/clipboard.dart';
import 'package:file_selector/file_selector.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Limits applied to images attached to a prompt.
abstract final class ImageAttachmentLimits {
  /// Maximum encoded size of one image.
  static const maxBytes = 10 * 1024 * 1024;

  /// Maximum images sent with one turn.
  static const maxCount = 6;
}

/// An image waiting to be sent with the next prompt.
final class PendingImage {
  /// Creates a pending image from decoded bytes.
  const PendingImage({required this.bytes, required this.mimeType, this.name});

  /// Encoded image bytes.
  final Uint8List bytes;

  /// MIME type such as `image/png`.
  final String mimeType;

  /// Original file name when known.
  final String? name;

  /// Runtime content part using a data URL.
  ImageContent toContent() => ImageContent(
    source: 'data:$mimeType;base64,${base64Encode(bytes)}',
    mimeType: mimeType,
  );
}

/// Picks image files from disk; overridable in tests.
final imagePickerProvider = Provider<Future<List<PendingImage>> Function()>(
  (ref) => pickImageFiles,
);

/// Reads images from the system clipboard; overridable in tests.
final imageClipboardProvider = Provider<Future<List<PendingImage>> Function()>(
  (ref) => readClipboardImages,
);

/// Opens a file dialog for PNG, JPEG, WebP, and GIF images.
Future<List<PendingImage>> pickImageFiles() async {
  final files = await openFiles(
    acceptedTypeGroups: const [
      XTypeGroup(
        label: 'Images',
        extensions: ['png', 'jpg', 'jpeg', 'webp', 'gif'],
        mimeTypes: ['image/png', 'image/jpeg', 'image/webp', 'image/gif'],
        uniformTypeIdentifiers: [
          'public.png',
          'public.jpeg',
          'org.webmproject.webp',
          'com.compuserve.gif',
          'public.image',
        ],
      ),
    ],
  );
  final images = <PendingImage>[];
  for (final file in files) {
    final bytes = Uint8List.fromList(await file.readAsBytes());
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

/// Sniffs a MIME type from magic bytes, falling back to [name]'s extension.
String? imageMimeType({String? name, Uint8List? bytes}) {
  if (bytes != null && bytes.length >= 12) {
    if (bytes[0] == 0x89 && bytes[1] == 0x50) {
      return 'image/png';
    }
    if (bytes[0] == 0xFF && bytes[1] == 0xD8) {
      return 'image/jpeg';
    }
    if (bytes[0] == 0x47 && bytes[1] == 0x49) {
      return 'image/gif';
    }
    if (bytes[0] == 0x52 &&
        bytes[1] == 0x49 &&
        bytes[8] == 0x57 &&
        bytes[9] == 0x45) {
      return 'image/webp';
    }
  }
  final extension = name?.split('.').last.toLowerCase();
  return switch (extension) {
    'png' => 'image/png',
    'jpg' || 'jpeg' => 'image/jpeg',
    'webp' => 'image/webp',
    'gif' => 'image/gif',
    _ => null,
  };
}

/// Decodes a `data:` URL into bytes, or returns null when the source is not
/// a base64 data URL.
Uint8List? bytesFromImageSource(String source) {
  const marker = 'base64,';
  final index = source.indexOf(marker);
  if (!source.startsWith('data:') || index < 0) {
    return null;
  }
  try {
    return base64Decode(source.substring(index + marker.length));
  } on FormatException {
    return null;
  }
}
