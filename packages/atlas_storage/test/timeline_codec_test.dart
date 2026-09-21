import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/src/mappers/timeline_codec.dart';
import 'package:test/test.dart';

void main() {
  final codec = TimelineCodec();

  UserMessageItem decodeContent(Object? content) =>
      codec
              .decode(
                id: TimelineItemId('item'),
                sessionId: SessionId('session'),
                turnId: TurnId('turn'),
                sequence: 1,
                occurredAt: DateTime.utc(2026),
                kind: 'user_message',
                version: 1,
                payload: jsonEncode({'content': content}),
              )
              .item
          as UserMessageItem;

  test('decodes every content variant with its optional fields', () {
    final item = decodeContent([
      {'type': 'text', 'text': 'hello'},
      {
        'type': 'image',
        'source': 'data:image/png;base64,AAAA',
        'detail': 'high',
      },
      {
        'type': 'image',
        'source': 'https://example.test/a.png',
        'detail': 'low',
      },
      {
        'type': 'resource',
        'uri': 'file:///tmp/a.dart',
        'text': 'void main() {}',
      },
      {'type': 'resource', 'uri': 'file:///tmp/b.dart'},
    ]);

    expect(item.content, hasLength(5));
    expect((item.content[0] as TextContent).text, 'hello');
    final high = item.content[1] as ImageContent;
    expect(high.source, 'data:image/png;base64,AAAA');
    expect(high.mimeType, isNull);
    expect(high.detail, ImageDetail.high);
    expect((item.content[2] as ImageContent).detail, ImageDetail.low);
    final resource = item.content[3] as ResourceContent;
    expect(resource.uri, 'file:///tmp/a.dart');
    expect(resource.mimeType, isNull);
    expect(resource.text, 'void main() {}');
    // An omitted text payload defaults to the empty string.
    expect((item.content[4] as ResourceContent).text, isEmpty);
  });

  test('keeps an explicit null mime type as null', () {
    final item = decodeContent([
      {
        'type': 'image',
        'source': 'https://example.test/a.png',
        'mime_type': null,
        'detail': 'auto',
      },
    ]);

    expect((item.content.single as ImageContent).mimeType, isNull);
  });

  test('ignores unknown keys inside a content item', () {
    final item = decodeContent([
      {
        'type': 'text',
        'text': 'hello',
        'annotations': {'unexpected': true},
      },
    ]);

    expect((item.content.single as TextContent).text, 'hello');
  });

  test('rejects an unsupported content type with the offending value', () {
    expect(
      () => decodeContent([
        {'type': 'video', 'source': 'file:///tmp/a.mp4'},
      ]),
      throwsA(
        isA<FormatException>().having(
          (error) => error.message,
          'message',
          'Unsupported content type: video',
        ),
      ),
    );
  });

  test('rejects a content item without a usable type', () {
    for (final item in <Map<String, Object?>>[
      {'text': 'hello'},
      {'type': ''},
      {'type': 7},
      {'type': null},
    ]) {
      expect(
        () => decodeContent([item]),
        throwsA(
          isA<FormatException>().having(
            (error) => error.message,
            'message',
            'type must be a non-empty string',
          ),
        ),
        reason: 'for $item',
      );
    }
  });

  test('rejects content that is not a JSON array', () {
    expect(
      () => decodeContent({'type': 'text', 'text': 'hello'}),
      throwsA(
        isA<FormatException>().having(
          (error) => error.message,
          'message',
          'content must be a JSON array',
        ),
      ),
    );
  });

  test('rejects a content item that is not a JSON object', () {
    expect(
      () => decodeContent(const ['text']),
      throwsA(
        isA<FormatException>().having(
          (error) => error.message,
          'message',
          'content item must be a JSON object',
        ),
      ),
    );
  });

  test('rejects content parts with missing required fields', () {
    expect(
      () => decodeContent([
        {'type': 'text'},
      ]),
      throwsA(isA<FormatException>()),
    );
    expect(
      () => decodeContent([
        {'type': 'image', 'detail': 'auto'},
      ]),
      throwsA(isA<FormatException>()),
    );
  });
}
