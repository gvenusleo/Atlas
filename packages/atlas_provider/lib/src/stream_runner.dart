import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';

import 'http_stream_client.dart';
import 'sse.dart';

/// Consumes SSE events for one request and assembles the final response.
abstract interface class StreamParser {
  /// Accepts one SSE event and yields incremental model events.
  Iterable<ModelStreamEvent> accept(SseEvent event);

  /// Returns the accumulated response once the stream is complete.
  ModelResponse finish();
}

/// Emits a failed event for a model that no configured entry owns.
Stream<ModelStreamEvent> notFoundStream(ModelRef model) =>
    Stream<ModelStreamEvent>.value(
      ModelFailedEvent(
        ArgumentError.value(model, 'model', 'is not configured'),
        StackTrace.current,
      ),
    );

/// Runs one streaming model request through [openStream] and [createParser].
///
/// Handles cancellation, failure conversion, and the single-terminal-event
/// contract shared by every provider implementation.
Stream<ModelStreamEvent> runModelStream({
  required ModelRequest request,
  required Future<ActiveHttpStream> Function() openStream,
  required StreamParser Function() createParser,
  required Object Function(Object error) toFailure,
}) {
  return Stream<ModelStreamEvent>.multi((controller) async {
    ActiveHttpStream? active;
    StreamSubscription<SseEvent>? sseSubscription;
    var disposed = false;

    controller.onCancel = () async {
      // Interrupt the request so the connection is not held until the timeout.
      disposed = true;
      await sseSubscription?.cancel();
      active?.close();
    };

    void emitFailure(Object error, StackTrace stackTrace) {
      if (disposed) {
        return;
      }
      disposed = true;
      final subscription = sseSubscription;
      if (subscription != null) {
        unawaited(subscription.cancel());
      }
      active?.close();
      controller.add(ModelFailedEvent(toFailure(error), stackTrace));
      controller.close();
    }

    try {
      request.cancellation?.throwIfCancelled();
      active = await openStream();
      final parser = createParser();
      sseSubscription = decodeSse(active.response.data!.stream).listen(
        (event) {
          if (disposed) {
            return;
          }
          try {
            request.cancellation?.throwIfCancelled();
            final updates = parser.accept(event);
            for (final update in updates) {
              controller.add(update);
            }
          } catch (error, stackTrace) {
            emitFailure(error, stackTrace);
          }
        },
        onError: (Object error, StackTrace stackTrace) {
          emitFailure(error, stackTrace);
        },
        onDone: () {
          if (disposed) {
            return;
          }
          try {
            request.cancellation?.throwIfCancelled();
            controller.add(ModelCompletedEvent(parser.finish()));
            disposed = true;
            controller.close();
          } catch (error, stackTrace) {
            emitFailure(error, stackTrace);
          } finally {
            active?.close();
          }
        },
      );
    } catch (error, stackTrace) {
      emitFailure(error, stackTrace);
    }
  });
}

/// Rejects requests that exceed the descriptor's input capabilities.
void validateRequestCapabilities(
  ModelRequest request,
  ModelDescriptor descriptor,
  Object Function(String message) fail,
) {
  if (request.messages.any(
        (message) => message.content.any((part) => part is ImageContent),
      ) &&
      !descriptor.inputCapabilities.contains(ModelInputCapability.image)) {
    throw fail('model does not support image input');
  }
  if (request.reasoningEffort != null &&
      descriptor.reasoningEfforts.isNotEmpty &&
      !descriptor.reasoningEfforts.any(
        (option) => option.value == request.reasoningEffort,
      )) {
    throw fail('unsupported reasoning effort');
  }
}
