/// Clipboard payload for copy and cut inside one file browser.
final class const FileClipboard({
  /// Absolute path of the copied or cut entry.
  required final String path,

  /// Whether paste should move instead of copy.
  required final bool cut,
});
