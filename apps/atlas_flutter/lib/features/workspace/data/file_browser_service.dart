import 'dart:convert';
import 'dart:io';

/// Limits used by the workspace file browser.
final class FileBrowserLimits {
  /// Maximum bytes loaded into the text preview.
  static const previewBytes = 512 * 1024;

  /// Maximum entries loaded from one directory.
  static const entriesPerFolder = 500;
}

/// Clipboard payload for copy and cut inside one file browser.
final class FileClipboard {
  /// Creates a clipboard entry.
  const FileClipboard({required this.path, required this.cut});

  /// Absolute path of the copied or cut entry.
  final String path;

  /// Whether paste should move instead of copy.
  final bool cut;
}

/// Performs bounded filesystem reads and writes for the workspace file browser.
final class FileBrowserService {
  /// Creates a filesystem browser service.
  const FileBrowserService({
    this.trash = moveToTrash,
    this.reveal = revealInFileManager,
  });

  /// Moves a path into the platform trash.
  final Future<void> Function(String path) trash;

  /// Reveals a path in the platform file manager.
  final Future<void> Function(String path) reveal;

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

  /// Creates an empty file named [name] under [directory].
  Future<File> createFile({
    required Directory directory,
    required String name,
    required String root,
  }) async {
    final file = File(_childPath(directory, name, root));
    if (await file.exists() || await Directory(file.path).exists()) {
      throw const FileSystemException('An item with that name already exists.');
    }
    return file.create();
  }

  /// Creates a folder named [name] under [directory].
  Future<Directory> createDirectory({
    required Directory directory,
    required String name,
    required String root,
  }) async {
    final folder = Directory(_childPath(directory, name, root));
    if (await folder.exists() || await File(folder.path).exists()) {
      throw const FileSystemException('An item with that name already exists.');
    }
    return folder.create();
  }

  /// Renames [entity] to [name] in the same parent directory.
  Future<FileSystemEntity> rename({
    required FileSystemEntity entity,
    required String name,
    required String root,
  }) async {
    final parent = entity.parent;
    final destination = _childPath(parent, name, root);
    if (await File(destination).exists() ||
        await Directory(destination).exists()) {
      throw const FileSystemException('An item with that name already exists.');
    }
    return entity.rename(destination);
  }

  /// Copies [source] into [directory], choosing a free name on collision.
  Future<FileSystemEntity> copyInto({
    required FileSystemEntity source,
    required Directory directory,
    required String root,
  }) async {
    _assertInsideRoot(source.path, root);
    final destination = _uniqueDestination(directory, _basename(source.path));
    _assertInsideRoot(destination, root);
    if (source is Directory) {
      return _copyDirectory(source, Directory(destination));
    }
    await File(source.path).copy(destination);
    return File(destination);
  }

  /// Moves [source] into [directory], choosing a free name on collision.
  Future<FileSystemEntity> moveInto({
    required FileSystemEntity source,
    required Directory directory,
    required String root,
  }) async {
    _assertInsideRoot(source.path, root);
    if (_isSameOrAncestor(source.path, directory.path)) {
      throw const FileSystemException('Cannot move a folder into itself.');
    }
    final destination = _uniqueDestination(directory, _basename(source.path));
    _assertInsideRoot(destination, root);
    return source.rename(destination);
  }

  /// Moves [path] to the platform trash after confirming it is under [root].
  Future<void> trashPath(String path, String root) async {
    _assertInsideRoot(path, root);
    await trash(path);
  }

  /// Reveals [path] in the platform file manager after confirming it is under
  /// [root].
  Future<void> revealPath(String path, String root) async {
    _assertInsideRoot(path, root);
    await reveal(path);
  }

  String _childPath(Directory directory, String name, String root) {
    final trimmed = name.trim();
    if (trimmed.isEmpty ||
        trimmed.contains('/') ||
        trimmed.contains('\\') ||
        trimmed == '.' ||
        trimmed == '..') {
      throw const FileSystemException('Enter a valid name.');
    }
    final path = '${directory.path}${Platform.pathSeparator}$trimmed';
    _assertInsideRoot(path, root);
    return path;
  }

  String _uniqueDestination(Directory directory, String name) {
    final separator = Platform.pathSeparator;
    var candidate = '${directory.path}$separator$name';
    if (!File(candidate).existsSync() && !Directory(candidate).existsSync()) {
      return candidate;
    }
    final dot = name.lastIndexOf('.');
    final stem = dot > 0 ? name.substring(0, dot) : name;
    final extension = dot > 0 ? name.substring(dot) : '';
    for (var i = 2; i < 1000; i++) {
      candidate = '${directory.path}$separator$stem $i$extension';
      if (!File(candidate).existsSync() && !Directory(candidate).existsSync()) {
        return candidate;
      }
    }
    throw const FileSystemException('Could not find a free name.');
  }

  Future<Directory> _copyDirectory(
    Directory source,
    Directory destination,
  ) async {
    await destination.create(recursive: true);
    await for (final entity in source.list(followLinks: false)) {
      final name = _basename(entity.path);
      final next = '${destination.path}${Platform.pathSeparator}$name';
      if (entity is Directory) {
        await _copyDirectory(entity, Directory(next));
      } else if (entity is File) {
        await entity.copy(next);
      }
    }
    return destination;
  }

  void _assertInsideRoot(String path, String root) {
    final resolved = canonicalFilePath(path);
    final rootPath = canonicalFilePath(root);
    final prefix = rootPath.endsWith(Platform.pathSeparator)
        ? rootPath
        : '$rootPath${Platform.pathSeparator}';
    if (resolved != rootPath && !resolved.startsWith(prefix)) {
      throw const FileSystemException('That path is outside the workspace.');
    }
  }

  bool _isSameOrAncestor(String source, String destination) {
    final from = canonicalFilePath(source);
    final to = canonicalFilePath(destination);
    return to == from || to.startsWith('$from${Platform.pathSeparator}');
  }

  String _basename(String path) => path.split(Platform.pathSeparator).last;
}

/// Resolves [path] through symlinks when the target exists.
String canonicalFilePath(String path) {
  try {
    return File(path).resolveSymbolicLinksSync();
  } on FileSystemException {
    final parent = File(path).parent;
    try {
      return '${parent.resolveSymbolicLinksSync()}${Platform.pathSeparator}'
          '${path.split(Platform.pathSeparator).last}';
    } on FileSystemException {
      return File(path).absolute.path;
    }
  }
}

/// Platform command used to move [path] to the trash.
({String executable, List<String> arguments}) trashCommand(
  String path, {
  required bool isDirectory,
  required String os,
}) {
  if (os == 'macos') {
    return (
      executable: 'osascript',
      arguments: [
        '-e',
        'tell application "Finder" to delete POSIX file ${jsonEncode(path)}',
      ],
    );
  }
  if (os == 'windows') {
    final method = isDirectory ? 'DeleteDirectory' : 'DeleteFile';
    return (
      executable: 'powershell',
      arguments: [
        '-NoProfile',
        '-Command',
        'Add-Type -AssemblyName Microsoft.VisualBasic; '
            '\$ui = [Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs; '
            '\$recycle = [Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin; '
            '[Microsoft.VisualBasic.FileIO.FileSystem]::$method('
            '${jsonEncode(path)}, \$ui, \$recycle)',
      ],
    );
  }
  return (executable: 'gio', arguments: ['trash', path]);
}

/// Moves [path] to the platform trash.
Future<void> moveToTrash(String path) async {
  final command = trashCommand(
    path,
    isDirectory: Directory(path).existsSync(),
    os: _currentOs,
  );
  final result = await Process.run(command.executable, command.arguments);
  if (result.exitCode != 0) {
    final stderr = result.stderr.toString().trim();
    throw FileSystemException(
      stderr.isEmpty ? 'Could not move the item to Trash.' : stderr,
      path,
    );
  }
}

/// Reveals [path] in the platform file manager.
Future<void> revealInFileManager(String path) async {
  final result = await Process.run(_revealExecutable(), _revealArguments(path));
  if (result.exitCode != 0) {
    throw FileSystemException('Could not reveal the item.', path);
  }
}

String get _currentOs {
  if (Platform.isMacOS) {
    return 'macos';
  }
  if (Platform.isWindows) {
    return 'windows';
  }
  return 'linux';
}

String _revealExecutable() {
  if (Platform.isMacOS) {
    return 'open';
  }
  if (Platform.isWindows) {
    return 'explorer';
  }
  return 'xdg-open';
}

List<String> _revealArguments(String path) {
  if (Platform.isMacOS) {
    return ['-R', path];
  }
  if (Platform.isWindows) {
    return ['/select,$path'];
  }
  return [File(path).parent.path];
}
