import '../ports/failures.dart';

/// Renders an error as a safe, model-visible message without raw details.
String safeErrorMessage(String prefix, Object error) {
  if (error is SafeMessageException) {
    return '$prefix (${error.runtimeType}): ${error.safeMessage}';
  }
  return '$prefix (${error.runtimeType})';
}

/// Extracts bounded provider diagnostic detail without exposing arbitrary
/// exceptions.
String? providerDetail(Object error) =>
    error is SafeMessageException ? error.diagnosticDetail : null;
