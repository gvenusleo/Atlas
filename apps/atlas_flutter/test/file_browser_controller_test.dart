import 'dart:async';
import 'dart:io';

import 'package:atlas_flutter/features/workspace/application/file_browser_controller.dart';
import 'package:atlas_flutter/features/workspace/application/file_browser_state.dart';
import 'package:atlas_flutter/features/workspace/data/file_browser_service.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late _FileService service;
  late ProviderContainer container;
  late NotifierProvider<FileBrowserController, FileBrowserState> provider;

  setUp(() {
    service = _FileService();
    container = ProviderContainer();
    provider =
        NotifierProvider.autoDispose<FileBrowserController, FileBrowserState>(
          () => FileBrowserController(
            workingDirectory: '/workspace',
            service: service,
          ),
        );
    addTearDown(() async {
      container.dispose();
      await service.close();
    });
  });

  test(
    'publishes immutable rows and caches collapsed directory children',
    () async {
      service.listings['/workspace'] = [Directory('/workspace/sub')];
      service.listings['/workspace/sub'] = [File('/workspace/sub/a.txt')];
      container.listen(provider, (_, _) {});
      await pumpEventQueue();
      final original = container.read(provider);
      final controller = container.read(provider.notifier);
      await controller.toggle(original.entries.single);
      expect(container.read(provider).entries, hasLength(2));
      expect(original.entries.single.expanded, isFalse);
      expect(() => original.entries.clear(), throwsUnsupportedError);
      await controller.toggle(container.read(provider).entries.first);
      await controller.toggle(container.read(provider).entries.first);
      expect(service.reads['/workspace/sub'], 1);
      expect(container.read(provider).entries, hasLength(2));
    },
  );

  test(
    'late previews cannot replace a newer file selection or a closed preview',
    () async {
      container.listen(provider, (_, _) {});
      await pumpEventQueue();
      final controller = container.read(provider.notifier);
      final first = Completer<String>();
      final second = Completer<String>();
      service.previews['/workspace/a'] = first.future;
      service.previews['/workspace/b'] = second.future;
      final a = controller.openFile(File('/workspace/a'));
      final b = controller.openFile(File('/workspace/b'));
      second.complete('second');
      await b;
      first.complete('first');
      await a;
      expect(container.read(provider).preview, 'second');
      final late = Completer<String>();
      service.previews['/workspace/a'] = late.future;
      final pending = controller.openFile(File('/workspace/a'));
      controller.closePreview();
      late.complete('obsolete');
      await pending;
      expect(container.read(provider).selectedFile, isNull);
      expect(container.read(provider).preview, isNull);
    },
  );

  test(
    'newer directory results supersede an older in-flight refresh',
    () async {
      container.listen(provider, (_, _) {});
      await pumpEventQueue();
      final controller = container.read(provider.notifier);
      final old = Completer<List<FileSystemEntity>>();
      service.nextListing = old.future;
      final pending = controller.refresh();
      service.listings['/workspace'] = [File('/workspace/new')];
      await controller.refresh();
      old.complete([File('/workspace/old')]);
      await pending;
      expect(
        container.read(provider).entries.single.entity.path,
        '/workspace/new',
      );
    },
  );

  test('disposal ignores pending loads without creating watchers', () async {
    final pending = Completer<List<FileSystemEntity>>();
    service.nextListing = pending.future;
    container.listen(provider, (_, _) {});
    await pumpEventQueue();
    container.dispose();
    pending.complete([File('/workspace/late')]);
    await pumpEventQueue();
    expect(service.watchers, isEmpty);
  });

  testWidgets(
    'watch events debounce reloads and disposal cancels subscriptions',
    (tester) async {
      container.listen(provider, (_, _) {});
      await tester.pump();
      expect(service.reads['/workspace'], 1);
      final watcher = service.watchers['/workspace']!;
      service.listings['/workspace'] = [File('/workspace/new')];
      watcher.add(FileSystemCreateEvent('/workspace/new', false));
      watcher.add(FileSystemModifyEvent('/workspace/new', false, true));
      await tester.pump(const Duration(milliseconds: 299));
      expect(service.reads['/workspace'], 1);
      await tester.pump(const Duration(milliseconds: 1));
      expect(service.reads['/workspace'], 2);
      expect(
        container.read(provider).entries.single.entity.path,
        '/workspace/new',
      );
      watcher.add(FileSystemModifyEvent('/workspace/new', false, true));
      container.dispose();
      await tester.pump(const Duration(seconds: 1));
      expect(watcher.hasListener, isFalse);
      expect(service.reads['/workspace'], 2);
    },
  );

  test(
    'unavailable watchers retain manual refresh and report load errors',
    () async {
      service.failWatch = true;
      container.listen(provider, (_, _) {});
      await pumpEventQueue();
      final controller = container.read(provider.notifier);
      service.nextListing = Future.error(
        const FileSystemException('unavailable'),
      );
      await controller.refresh();
      expect(container.read(provider).rootError, 'unavailable');
      service.listings['/workspace'] = [File('/workspace/recovered')];
      await controller.refresh();
      expect(container.read(provider).rootError, isNull);
      expect(
        container.read(provider).entries.single.entity.path,
        '/workspace/recovered',
      );
    },
  );
}

class _FileService extends FileBrowserService {
  final listings = <String, List<FileSystemEntity>>{};
  final previews = <String, Future<String>>{};
  final watchers = <String, StreamController<FileSystemEvent>>{};
  final reads = <String, int>{};
  Future<List<FileSystemEntity>>? nextListing;
  bool failWatch = false;

  @override
  Future<List<FileSystemEntity>> listDirectory(Directory directory) async {
    reads.update(directory.path, (count) => count + 1, ifAbsent: () => 1);
    final pending = nextListing;
    nextListing = null;
    return pending ?? listings[directory.path] ?? [];
  }

  @override
  Stream<FileSystemEvent> watchDirectory(String path) {
    if (failWatch) throw UnsupportedError('watch unavailable');
    return watchers
        .putIfAbsent(path, () => StreamController(sync: true))
        .stream;
  }

  @override
  Future<String> readPreview(File file) => previews[file.path]!;

  Future<void> close() async {
    for (final watcher in watchers.values) {
      await watcher.close();
    }
  }
}
