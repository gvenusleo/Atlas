import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:crypto/crypto.dart';

/// Persists and validates the bearer token that guards the remote WebSocket
/// server.
///
/// The token is a 256-bit random value written to a file with owner-only
/// permissions (`0600` on POSIX systems). Validation reloads the file on
/// every call so rotating the token takes effect without a server restart.
final class RemoteTokenFile {
  /// Creates a token store backed by [file].
  RemoteTokenFile(this._file);

  final File _file;

  /// Loads the current token, creating and persisting one on first use.
  ///
  /// Returns the token text. The token is written only when the file does
  /// not already contain one.
  Future<String> loadOrCreate() async {
    final existing = _readToken();
    if (existing != null) {
      return existing;
    }
    return _writeToken(generateToken());
  }

  /// Replaces the token with a fresh random value and returns it.
  ///
  /// Existing connections stay open; new connections must present the new
  /// token.
  Future<String> rotate() => _writeToken(generateToken());

  /// Returns true when [authorization] is a `Bearer` header carrying the
  /// current token.
  ///
  /// Comparison is constant-time over the token digests so the response
  /// reveals nothing about the stored value.
  Future<bool> authorize(String? authorization) async {
    if (authorization == null) {
      return false;
    }
    final parts = authorization.split(' ');
    if (parts.length != 2 || parts[0] != 'Bearer') {
      return false;
    }
    final token = _readToken();
    return token != null && _constantTimeEquals(parts[1], token);
  }

  /// Returns the token stored in [file], or null when absent or unreadable.
  String? _readToken() {
    try {
      if (!_file.existsSync()) {
        return null;
      }
      final value = _file.readAsStringSync().trim();
      return value.isEmpty ? null : value;
    } on FileSystemException {
      return null;
    }
  }

  Future<String> _writeToken(String token) async {
    await _file.parent.create(recursive: true);
    await _file.writeAsString('$token\n', flush: true);
    if (!Platform.isWindows) {
      // Dart cannot set POSIX modes; keep the token readable by its owner
      // only. Failures are ignored so exotic platforms still work.
      await Process.run('chmod', ['600', _file.path]);
    }
    return token;
  }
}

/// Generates a new 256-bit token, URL-safe and without padding.
String generateToken() {
  final random = List<int>.generate(32, (_) => Random.secure().nextInt(256));
  return base64Url.encode(random).replaceAll('=', '');
}

/// Compares two tokens by their SHA-256 digests in constant time.
bool _constantTimeEquals(String a, String b) {
  final digestA = sha256.convert(utf8.encode(a)).bytes;
  final digestB = sha256.convert(utf8.encode(b)).bytes;
  var difference = 0;
  for (var i = 0; i < digestA.length; i++) {
    difference |= digestA[i] ^ digestB[i];
  }
  return difference == 0;
}
