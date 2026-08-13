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
sealed class ContentPart {
  /// Creates a content segment.
  const ContentPart();
}

/// A plain text content segment.
final class TextContent extends ContentPart {
  /// Creates a text segment.
  const TextContent(this.text);

  /// The text value.
  final String text;
}

/// An image content segment.
final class ImageContent extends ContentPart {
  /// Creates an image segment from a URI or a data URL.
  const ImageContent({
    required this.source,
    this.mimeType,
    this.detail = ImageDetail.auto,
  });

  /// A URI or data URL accepted by the selected provider.
  final String source;

  /// The MIME type when known.
  final String? mimeType;

  /// The requested image detail.
  final ImageDetail detail;
}

/// An embedded resource content segment (text payload).
final class ResourceContent extends ContentPart {
  /// Creates a resource segment.
  const ResourceContent({required this.uri, this.mimeType, this.text = ''});

  /// The resource URI.
  final String uri;

  /// The MIME type when known.
  final String? mimeType;

  /// The embedded text payload.
  final String text;
}

/// Returns the text content from a list of parts.
String textFromContent(List<ContentPart> parts) => parts
    .whereType<TextContent>()
    .map((part) => part.text)
    .where((text) => text.isNotEmpty)
    .join('\n\n');
