import 'dart:convert';
import 'dart:io';

import 'package:atlas_provider/atlas_provider.dart';
import 'package:dio/dio.dart';
import 'package:test/test.dart';

void main() {
  test('bundled catalog includes supported offline models', () {
    final catalog = ModelCatalog.bundled();
    expect(catalog.providers.keys, containsAll(['openai', 'anthropic']));
    final model = catalog.providers['openai']!.firstWhere(
      (m) => m['id'] == 'gpt-4o',
    );
    expect(model['api'], 'openai-responses');
    expect(model['contextWindow'], 128000);
    expect(model['input'], ['text', 'image']);
  });

  test(
    'corrupt cache falls back and invalid refresh preserves the cache',
    () async {
      final dir = await Directory.systemTemp.createTemp('atlas_catalog_');
      addTearDown(() => dir.delete(recursive: true));
      final cache = File('${dir.path}/cache/model-catalog.json');
      await cache.parent.create();
      await cache.writeAsString('{invalid');
      expect(ModelCatalog.load(dir).providers['openai'], isNotEmpty);
      final client = Dio()
        ..interceptors.add(
          InterceptorsWrapper(
            onRequest: (options, handler) {
              handler.resolve(
                Response(
                  requestOptions: options,
                  data: <String, Object?>{},
                  statusCode: 200,
                ),
              );
            },
          ),
        );
      addTearDown(client.close);
      await expectLater(
        ModelCatalog.refresh(dir, client: client),
        throwsFormatException,
      );
      expect(await cache.readAsString(), '{invalid');
    },
  );

  for (final (label, invalidModel) in <(String, Map<String, Object?>)>[
    ('empty name', {'name': ''}),
    (
      'incomplete tier',
      {
        'cost': {
          'input': 1,
          'output': 2,
          'tiers': [
            {
              'tier': {'size': 200000},
              'input': 2,
            },
          ],
        },
      },
    ),
  ]) {
    test(
      'rejects $label before publishing cache and rejects an existing invalid cache',
      () async {
        final dir = await Directory.systemTemp.createTemp(
          'atlas_invalid_catalog_',
        );
        addTearDown(() => dir.delete(recursive: true));
        final cache = File('${dir.path}/cache/model-catalog.json');
        await cache.parent.create();
        await cache.writeAsString('previous-cache');
        final data = <String, Object?>{
          for (final provider in ['openai', 'anthropic'])
            provider: {
              'models': {
                'test': {
                  'name': 'Valid',
                  'tool_call': true,
                  'limit': {'context': 32000, 'output': 4096},
                  'modalities': {
                    'input': ['text'],
                    'output': ['text'],
                  },
                  ...invalidModel,
                },
              },
            },
        };
        final client = Dio()
          ..interceptors.add(
            InterceptorsWrapper(
              onRequest: (options, handler) {
                handler.resolve(
                  Response(
                    requestOptions: options,
                    statusCode: 200,
                    data: data,
                  ),
                );
              },
            ),
          );
        addTearDown(client.close);
        await expectLater(
          ModelCatalog.refresh(dir, client: client),
          throwsFormatException,
        );
        expect(await cache.readAsString(), 'previous-cache');
        await cache.writeAsString(jsonEncode({'version': 1, 'data': data}));
        expect(
          ModelCatalog.load(dir).providers['openai']!
              .any((m) => m['id'] == 'gpt-4o'),
          isTrue,
        );
      },
    );
  }

  test(
    'refresh atomically installs normalized data without trusting endpoints',
    () async {
      final dir = await Directory.systemTemp.createTemp('atlas_catalog_');
      addTearDown(() => dir.delete(recursive: true));
      final client = Dio()
        ..interceptors.add(
          InterceptorsWrapper(
            onRequest: (options, handler) {
              handler.resolve(
                Response(
                  requestOptions: options,
                  statusCode: 200,
                  data: {
                    for (final provider in ['openai', 'anthropic'])
                      provider: {
                        'api': 'https://untrusted.example',
                        'models': {
                          'test': {
                            'name': 'Fresh',
                            'tool_call': true,
                            'limit': {'context': 32000, 'output': 4096},
                            'modalities': {
                              'input': ['text'],
                              'output': ['text'],
                            },
                          },
                          'retired': {
                            'tool_call': true,
                            'status': 'deprecated',
                          },
                        },
                      },
                  },
                ),
              );
            },
          ),
        );
      addTearDown(client.close);
      await ModelCatalog.refresh(dir, client: client);
      final model = ModelCatalog.load(dir).providers['openai']!.single;
      expect(model['id'], 'test');
      expect(model['baseUrl'], 'https://api.openai.com/v1');
      expect(model['contextWindow'], 32000);
    },
  );
}
