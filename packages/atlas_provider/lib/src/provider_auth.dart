import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:path/path.dart' as p;

import 'json_document.dart';

/// A credential failure whose message never contains secret values or commands.
final class const ProviderAuthException(@override final String safeMessage)
    implements SafeMessageException {
  /// Creates a redacted authentication failure.
  this;
  @override
  String? get diagnosticDetail => null;

  @override
  String toString() => safeMessage;
}

/// Resolves Pi API-key/header values without caching command output.
final class ConfigValueResolver(Map<String, String> environment) {
  /// Captures the environment used by all value resolutions.
  this : environment = Map.unmodifiable(environment);

  /// The host's startup environment snapshot.
  final Map<String, String> environment;

  /// Resolves literals, `$NAME`, `${NAME}`, `$$`, `$!`, and `!command`.
  Future<String> resolve(String value) async {
    if (value.startsWith('!')) {
      final process = await Process.start(
        Platform.isWindows ? 'cmd.exe' : '/bin/sh',
        [Platform.isWindows ? '/c' : '-c', value.substring(1)],
        environment: environment,
        includeParentEnvironment: false,
      );
      final bytes = <int>[];
      var tooLarge = false;
      final output = process.stdout.listen((chunk) {
        if (bytes.length + chunk.length > 65536) {
          tooLarge = true;
          process.kill();
        } else {
          bytes.addAll(chunk);
        }
      });
      final outputDone = output.asFuture<void>();
      final errors = process.stderr.listen((_) {});
      try {
        final completion = await Future.wait<Object?>([
          process.exitCode,
          outputDone,
        ]).timeout(const Duration(seconds: 10));
        final code = completion.first;
        if (code != 0 || tooLarge) {
          throw const ProviderAuthException('Credential command failed');
        }
        final result = utf8.decode(bytes).trim();
        if (result.isEmpty) {
          throw const ProviderAuthException(
            'Credential command returned no value',
          );
        }
        return result;
      } on TimeoutException {
        process.kill(ProcessSignal.sigkill);
        throw const ProviderAuthException('Credential command timed out');
      } finally {
        await output.cancel();
        await errors.cancel();
      }
    }
    final pattern = RegExp(
      r'\$(\$|!|\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)',
    );
    return value.replaceAllMapped(pattern, (match) {
      final token = match[1]!;
      if (token == r'$' || token == '!') return token;
      final name = token.startsWith('{')
          ? token.substring(1, token.length - 1)
          : token;
      final replacement = environment[name];
      if (replacement == null || replacement.isEmpty) {
        throw ProviderAuthException('Missing environment variable $name');
      }
      return replacement;
    });
  }
}

/// Pi-shaped API-key storage, read again for every request.
final class AuthStore(
  /// The private credential document.
  final File file,
) {
  /// Creates a store without reading credentials.
  this;
  static final _writes = <String, Future<void>>{};

  /// Reads a provider's saved key; unsupported credential types fail explicitly.
  String? readKey(String provider) {
    final data = _read();
    final value = data[provider];
    if (value == null) return null;
    if (value is! Map ||
        value['type'] != 'api_key' ||
        value['key'] is! String) {
      throw ProviderAuthException(
        'Unsupported auth.json credential for $provider',
      );
    }
    return value['key'] as String;
  }

  Map<String, Object?> _read() {
    if (!file.existsSync()) return {};
    try {
      return decodeJsonDocument(file.readAsStringSync());
    } on Object {
      throw const ProviderAuthException('Cannot read auth.json');
    }
  }

  /// Saves or removes one provider while preserving other credentials.
  Future<void> setKey(String provider, String? key) {
    final path = p.normalize(file.absolute.path);
    final previous = _writes[path] ?? Future<void>.value();
    final next = previous.then((_) => _update(provider, key));
    final settled = next.then<void>((_) {}, onError: (Object _) {});
    _writes[path] = settled;
    unawaited(
      settled.whenComplete(() {
        if (identical(_writes[path], settled)) _writes.remove(path);
      }),
    );
    return next;
  }

  Future<void> _update(String provider, String? key) async {
    await file.parent.create(recursive: true);
    final lock = await File('${file.path}.lock').open(mode: FileMode.append);
    Directory? staging;
    try {
      await lock.lock(FileLock.blockingExclusive);
      final data = _read();
      if (key == null) {
        data.remove(provider);
      } else {
        if (key.trim().isEmpty) {
          throw const ProviderAuthException('API key must not be empty');
        }
        data[provider] = {'type': 'api_key', 'key': key};
      }
      staging = await file.parent.createTemp('.auth-');
      final temp = File(p.join(staging.path, 'auth.json'));
      await temp.writeAsString(
        '${const JsonEncoder.withIndent('  ').convert(data)}\n',
        flush: true,
      );
      if (!Platform.isWindows) {
        final result = await Process.run('chmod', ['600', temp.path]);
        if (result.exitCode != 0) {
          throw const ProviderAuthException(
            'Cannot restrict auth.json permissions',
          );
        }
      }
      await temp.rename(file.path);
    } finally {
      await lock.close();
      await staging?.delete(recursive: true);
    }
  }
}

/// Request-time authentication and custom headers for a configured provider.
final class ProviderAuthentication({
  required final String provider,
  required final ConfigValueResolver resolver,
  final AuthStore? store,
  final String? apiKey,
  final String? runtimeApiKey,
  final List<String> environmentKeys = const [],
  final Map<String, String> headers = const {},
  final bool authHeader = false,
}) {
  /// Creates request-time authentication for one provider/model configuration.
  this;

  /// Resolves the effective key and merged custom headers.
  Future<({String? key, Map<String, Object> headers})> resolve() async {
    try {
      final stored = runtimeApiKey == null ? store?.readKey(provider) : null;
      final source = runtimeApiKey ?? stored ?? apiKey;
      String? key;
      if (source != null) {
        key = await resolver.resolve(source);
      } else {
        for (final name in environmentKeys) {
          final candidate = resolver.environment[name];
          if (candidate != null && candidate.isNotEmpty) {
            key = candidate;
            break;
          }
        }
      }
      final resolved = <String, Object>{};
      for (final entry in headers.entries) {
        final value = await resolver.resolve(entry.value);
        if (value.contains('\r') ||
            value.contains('\n') ||
            value.contains('\x00')) {
          throw const ProviderAuthException('Invalid credential header');
        }
        resolved[entry.key.toLowerCase()] = value;
      }
      if (authHeader) {
        if (key == null || key.isEmpty) {
          throw ProviderAuthException('No API key configured for $provider');
        }
        resolved['authorization'] = 'Bearer $key';
      }
      if (key == null && resolved.isEmpty) {
        throw ProviderAuthException('No API key configured for $provider');
      }
      return (key: key, headers: resolved);
    } on ProviderAuthException {
      rethrow;
    } on Object {
      throw ProviderAuthException('Cannot resolve credentials for $provider');
    }
  }
}
