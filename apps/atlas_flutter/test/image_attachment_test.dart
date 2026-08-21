import 'dart:convert';
import 'dart:typed_data';

import 'package:atlas_flutter/features/workspace/data/image_attachment.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('imageMimeType sniffs png jpeg gif and webp', () {
    expect(
      imageMimeType(
        bytes: Uint8List.fromList(const [
          0x89,
          0x50,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
        ]),
      ),
      'image/png',
    );
    expect(
      imageMimeType(
        bytes: Uint8List.fromList(const [
          0xFF,
          0xD8,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
        ]),
      ),
      'image/jpeg',
    );
    expect(
      imageMimeType(
        bytes: Uint8List.fromList(const [
          0x47,
          0x49,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
        ]),
      ),
      'image/gif',
    );
    expect(
      imageMimeType(
        bytes: Uint8List.fromList(const [
          0x52,
          0x49,
          0,
          0,
          0,
          0,
          0,
          0,
          0x57,
          0x45,
          0,
          0,
        ]),
      ),
      'image/webp',
    );
    expect(imageMimeType(name: 'shot.JPG'), 'image/jpeg');
    expect(imageMimeType(name: 'notes.txt'), isNull);
  });

  test('bytesFromImageSource decodes data URLs', () {
    final bytes = Uint8List.fromList(const [1, 2, 3]);
    final source = 'data:image/png;base64,${base64Encode(bytes)}';
    expect(bytesFromImageSource(source), bytes);
    expect(bytesFromImageSource('https://example.com/a.png'), isNull);
  });

  test('PendingImage encodes a data URL content part', () {
    final image = PendingImage(
      bytes: Uint8List.fromList(const [1, 2, 3]),
      mimeType: 'image/png',
      name: 'a.png',
    );
    expect(image.toContent().mimeType, 'image/png');
    expect(image.toContent().source, startsWith('data:image/png;base64,'));
    expect(
      bytesFromImageSource(image.toContent().source),
      Uint8List.fromList(const [1, 2, 3]),
    );
  });
}
