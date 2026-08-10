import 'dart:async';
import 'dart:convert';
import 'dart:math' as math;

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:dio/dio.dart';

/// An established streaming HTTP response with cancellation wiring.
final class ActiveHttpStream {
  /// Creates an active stream.
  const ActiveHttpStream({
    required this.response,
    required this.cancelToken,
    required this.finished,
  });

  /// The established Dio stream response.
  final Response<ResponseBody> response;

  /// The token used to cancel the underlying request.
  final CancelToken cancelToken;

  /// Completes when the caller stops using the stream.
  final Completer<void> finished;

  /// Releases the request; cancelling an established response is a no-op.
  void close() {
    if (!finished.isCompleted) {
      finished.complete();
    }
    cancelToken.cancel();
  }
}

/// A provider-safe streaming request failure.
final class HttpStreamException implements Exception {
  /// Creates a stream failure.
  const HttpStreamException({required this.message, this.statusCode});

  /// A redacted, provider-safe error message.
  final String message;

  /// The HTTP status when the server returned one.
  final int? statusCode;

  @override
  String toString() {
    final status = statusCode == null ? '' : ' (status $statusCode)';
    return 'HttpStreamException$status: $message';
  }
}

/// Opens streaming HTTP requests with bounded retries and cancellation.
abstract interface class HttpStreamClient {
  /// Opens a streaming POST and returns the established response.
  ///
  /// Connection failures, HTTP 429, and HTTP 5xx are retried before the
  /// stream starts. Other failures throw [HttpStreamException]; cancellation
  /// raises [TurnCancelledException], including during retry waits. The
  /// optional [secret] is redacted from error bodies.
  Future<ActiveHttpStream> openStream({
    required Uri uri,
    required Map<String, Object?> body,
    required Map<String, Object> headers,
    String? secret,
    CancellationToken? cancellation,
  });
}

/// Dio-backed [HttpStreamClient] with bounded retries and timeouts.
final class DioHttpStreamClient implements HttpStreamClient {
  /// Creates a client with an optional injected [Dio] instance.
  DioHttpStreamClient({Dio? dio, this.maxAttempts = 4})
    : _dio =
          dio ??
          Dio(
            BaseOptions(
              connectTimeout: const Duration(seconds: 10),
              sendTimeout: const Duration(seconds: 30),
              receiveTimeout: const Duration(minutes: 2),
            ),
          );

  final Dio _dio;

  /// Maximum request attempts before streaming starts.
  final int maxAttempts;

  @override
  Future<ActiveHttpStream> openStream({
    required Uri uri,
    required Map<String, Object?> body,
    required Map<String, Object> headers,
    String? secret,
    CancellationToken? cancellation,
  }) async {
    Object? lastError;
    for (var attempt = 0; attempt < maxAttempts; attempt++) {
      cancellation?.throwIfCancelled();
      final cancelToken = CancelToken();
      final token = cancellation;
      final requestFinished = Completer<void>();
      final cancelSubscription = token == null
          ? null
          : Future.any<void>([
              token.whenCancelled,
              requestFinished.future,
            ]).then((_) {
              if (!requestFinished.isCompleted && !cancelToken.isCancelled) {
                cancelToken.cancel();
              }
            });
      unawaited(cancelSubscription);
      var keepCancellationActive = false;
      try {
        final response = await _dio.postUri<ResponseBody>(
          uri,
          data: jsonEncode(body),
          cancelToken: cancelToken,
          options: Options(
            responseType: ResponseType.stream,
            headers: headers,
            validateStatus: (_) => true,
          ),
        );
        final status = response.statusCode ?? 0;
        if (status >= 200 && status < 300) {
          keepCancellationActive = true;
          return ActiveHttpStream(
            response: response,
            cancelToken: cancelToken,
            finished: requestFinished,
          );
        }
        final retryable = status == 429 || status >= 500;
        lastError = HttpStreamException(
          statusCode: status,
          message: await _errorBody(response.data?.stream, secret),
        );
        if (!retryable || attempt == maxAttempts - 1) {
          throw lastError;
        }
        await _waitBeforeRetry(
          cancellation,
          _retryDelay(attempt, response.headers),
        );
      } on DioException catch (_) {
        if (cancelToken.isCancelled || token?.isCancelled == true) {
          throw const TurnCancelledException();
        }
        lastError = const HttpStreamException(
          message: 'request failed before streaming started',
        );
        if (attempt == maxAttempts - 1) {
          throw lastError;
        }
        await _waitBeforeRetry(cancellation, _retryDelay(attempt, null));
      } finally {
        if (!keepCancellationActive) {
          requestFinished.complete();
        }
      }
    }
    throw lastError ?? StateError('request attempts exhausted');
  }
}

Future<String> _errorBody(Stream<List<int>>? stream, String? secret) async {
  if (stream == null) return 'provider returned an error';
  final bytes = <int>[];
  await for (final chunk in stream) {
    final remaining = 65536 - bytes.length;
    if (remaining > 0) {
      bytes.addAll(chunk.take(remaining));
    }
  }
  final text = utf8.decode(bytes, allowMalformed: true).trim();
  if (text.isEmpty) return 'provider returned an error';
  return secret == null || secret.isEmpty
      ? text
      : text.replaceAll(secret, '[redacted]');
}

Duration _retryDelay(int attempt, Headers? headers) {
  final retryAfter = headers?.value('retry-after');
  final seconds = num.tryParse(retryAfter ?? '');
  if (seconds != null && seconds >= 0) {
    return Duration(milliseconds: (seconds * 1000).round());
  }
  return Duration(seconds: math.pow(2, attempt).toInt());
}

Future<void> _waitBeforeRetry(CancellationToken? token, Duration delay) async {
  if (token == null) {
    await Future<void>.delayed(delay);
    return;
  }
  await Future.any<void>([Future<void>.delayed(delay), token.whenCancelled]);
  token.throwIfCancelled();
}
