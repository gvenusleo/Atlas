import 'dart:convert';
import 'dart:typed_data';

import 'package:atlas_runtime/atlas_runtime.dart';

/// Limits applied to images attached to a prompt.
abstract final class ImageAttachmentLimits {
  /// Maximum encoded size of one image.
  static const maxBytes = 10 * 1024 * 1024;

  /// Maximum images sent with one turn.
  static const maxCount = 6;
}

/// An image waiting to be sent with the next prompt.
final class const PendingImage({
  /// Encoded image bytes.
  required final Uint8List bytes,

  /// MIME type such as `image/png`.
  required final String mimeType,

  /// Original file name when known.
  final String? name,
}) {
  /// Creates a pending image from decoded bytes.
  this;

  /// Runtime content part using a data URL.
  ImageContent toContent() => ImageContent(
    source: 'data:$mimeType;base64,${base64Encode(bytes)}',
    mimeType: mimeType,
  );
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
