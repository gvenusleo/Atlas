/// The requested image processing detail.
enum ImageDetail {
  /// Let the provider choose the image resolution.
  auto,

  /// Request low-resolution processing.
  low,

  /// Request high-resolution processing.
  high,
}

/// A model-visible content segment.
sealed class const ContentPart() {
  /// Creates a content segment.
  this;
}

/// A plain text content segment.
final class const TextContent(
  /// The text value.
  final String text,
) extends ContentPart {
  /// Creates a text segment.
  this;
}

/// An image content segment.
final class const ImageContent({
  /// A URI or data URL accepted by the selected provider.
  required final String source,

  /// The MIME type when known.
  final String? mimeType,

  /// The requested image detail.
  final ImageDetail detail = ImageDetail.auto,
}) extends ContentPart {
  /// Creates an image segment from a URI or a data URL.
  this;
}

/// An embedded resource content segment (text payload).
final class const ResourceContent({
  /// The resource URI.
  required final String uri,

  /// The MIME type when known.
  final String? mimeType,

  /// The embedded text payload.
  final String text = '',
}) extends ContentPart {
  /// Creates a resource segment.
  this;
}

/// Returns the text content from a list of parts.
String textFromContent(List<ContentPart> parts) => parts
    .whereType<TextContent>()
    .map((part) => part.text)
    .where((text) => text.isNotEmpty)
    .join('\n\n');
