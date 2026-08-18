import 'dart:convert';
import 'dart:io';

/// Limits used by the workspace file browser.
final class FileBrowserLimits {
  /// Maximum bytes loaded into the text preview.
  static const previewBytes = 512 * 1024;

  /// Maximum entries loaded from one directory.
  static const entriesPerFolder = 500;
}

/// Performs bounded filesystem reads for the workspace file browser.
final class FileBrowserService {
  /// Creates a filesystem browser service.
  const FileBrowserService();

  /// Lists one directory without following symbolic links.
  Future<List<FileSystemEntity>> listDirectory(Directory directory) async {
    final entries = await directory
        .list(followLinks: false)
        .take(FileBrowserLimits.entriesPerFolder)
        .toList();
    entries.sort((a, b) {
      final directoryOrder =
          (a is Directory ? 0 : 1) - (b is Directory ? 0 : 1);
      if (directoryOrder != 0) {
        return directoryOrder;
      }
      return a.path.toLowerCase().compareTo(b.path.toLowerCase());
    });
    return entries;
  }

  /// Reads a bounded UTF-8 text preview from [file].
  Future<String> readPreview(File file) async {
    final length = await file.length();
    if (length > FileBrowserLimits.previewBytes) {
      throw const FormatException(
        'File is larger than the 512 KB preview limit.',
      );
    }
    final bytes = await file.readAsBytes();
    if (bytes.contains(0)) {
      throw const FormatException('Binary files cannot be previewed.');
    }
    return utf8.decode(bytes);
  }
}
