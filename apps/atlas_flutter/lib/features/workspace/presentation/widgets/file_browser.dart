import 'dart:convert';
import 'dart:io';

import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';

const _maximumPreviewBytes = 512 * 1024;

/// Browses a working directory and previews UTF-8 text files.
class FileBrowser extends StatefulWidget {
  /// Creates a browser rooted at [workingDirectory].
  const FileBrowser({super.key, required this.workingDirectory});

  /// Directory that users cannot navigate above.
  final String workingDirectory;

  @override
  State<FileBrowser> createState() => _FileBrowserState();
}

class _FileBrowserState extends State<FileBrowser> {
  late Directory _root;
  late Directory _current;
  List<FileSystemEntity> _entries = const [];
  File? _selectedFile;
  String? _preview;
  String? _error;
  bool _loading = false;

  @override
  void initState() {
    super.initState();
    _resetRoot();
  }

  @override
  void didUpdateWidget(covariant FileBrowser oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.workingDirectory != widget.workingDirectory) {
      _resetRoot();
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SizedBox(
          height: 38,
          child: Row(
            children: [
              _FileAction(
                icon: _selectedFile == null
                    ? LucideIcons.chevronLeft
                    : LucideIcons.arrowLeft,
                tooltip: _selectedFile == null
                    ? 'Parent folder'
                    : 'Back to files',
                enabled: _selectedFile != null || !_samePath(_current, _root),
                onPressed: _selectedFile == null ? _goUp : _closePreview,
              ),
              Expanded(
                child: Text(
                  _selectedFile?.path.split(Platform.pathSeparator).last ??
                      _relativeCurrentPath,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    color: colors.textSecondary,
                    fontSize: 11.5,
                    fontFamily: _selectedFile == null ? null : 'monospace',
                  ),
                ),
              ),
              _FileAction(
                icon: LucideIcons.refreshCw,
                tooltip: 'Refresh files',
                onPressed: _selectedFile == null ? _loadEntries : _loadPreview,
              ),
            ],
          ),
        ),
        const Divider(),
        Expanded(child: _buildContent(colors)),
      ],
    );
  }

  Widget _buildContent(AtlasColors colors) {
    if (_loading) {
      return Center(
        child: SizedBox.square(
          dimension: 18,
          child: CircularProgressIndicator(
            strokeWidth: 1.5,
            color: colors.accent,
          ),
        ),
      );
    }
    if (_error != null) {
      return Padding(
        padding: const EdgeInsets.all(14),
        child: Text(
          _error!,
          style: TextStyle(color: colors.error, fontSize: 12, height: 1.45),
        ),
      );
    }
    if (_selectedFile != null) {
      return SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(12, 10, 12, 20),
        child: SelectableText(
          _preview ?? '',
          style: TextStyle(
            color: colors.textPrimary,
            fontFamily: 'monospace',
            fontSize: 11.5,
            height: 1.45,
          ),
        ),
      );
    }
    if (_entries.isEmpty) {
      return Center(
        child: Text(
          'Empty folder',
          style: TextStyle(color: colors.textSecondary, fontSize: 12),
        ),
      );
    }
    return ListView.builder(
      padding: const EdgeInsets.symmetric(vertical: 4),
      itemCount: _entries.length,
      itemBuilder: (context, index) {
        final entry = _entries[index];
        final isDirectory = entry is Directory;
        return InkWell(
          onTap: () =>
              isDirectory ? _openDirectory(entry) : _openFile(entry as File),
          child: SizedBox(
            height: 34,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 10),
              child: Row(
                children: [
                  Icon(
                    isDirectory ? LucideIcons.folder : LucideIcons.file,
                    size: 15,
                    color: isDirectory ? colors.accent : colors.textSecondary,
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      entry.path.split(Platform.pathSeparator).last,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(color: colors.textPrimary, fontSize: 12),
                    ),
                  ),
                ],
              ),
            ),
          ),
        );
      },
    );
  }

  String get _relativeCurrentPath {
    if (_samePath(_current, _root)) {
      return _root.path;
    }
    final prefix = '${_root.path}${Platform.pathSeparator}';
    return _current.path.startsWith(prefix)
        ? _current.path.substring(prefix.length)
        : _current.path;
  }

  void _resetRoot() {
    _root = Directory(widget.workingDirectory).absolute;
    _current = _root;
    _selectedFile = null;
    _preview = null;
    _error = null;
    _loadEntries();
  }

  Future<void> _loadEntries() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final entries = await _current
          .list(followLinks: false)
          .take(500)
          .toList();
      entries.sort((a, b) {
        final directoryOrder =
            (a is Directory ? 0 : 1) - (b is Directory ? 0 : 1);
        if (directoryOrder != 0) {
          return directoryOrder;
        }
        return a.path.toLowerCase().compareTo(b.path.toLowerCase());
      });
      if (mounted) {
        setState(() => _entries = entries);
      }
    } on FileSystemException catch (error) {
      if (mounted) {
        setState(() => _error = error.message);
      }
    } finally {
      if (mounted) {
        setState(() => _loading = false);
      }
    }
  }

  void _openDirectory(Directory directory) {
    _current = directory.absolute;
    _loadEntries();
  }

  void _goUp() {
    if (_samePath(_current, _root)) {
      return;
    }
    final parent = _current.parent.absolute;
    _current = parent.path.startsWith(_root.path) ? parent : _root;
    _loadEntries();
  }

  void _openFile(File file) {
    _selectedFile = file;
    _loadPreview();
  }

  void _closePreview() {
    setState(() {
      _selectedFile = null;
      _preview = null;
      _error = null;
    });
  }

  Future<void> _loadPreview() async {
    final file = _selectedFile;
    if (file == null) {
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final length = await file.length();
      if (length > _maximumPreviewBytes) {
        throw const FormatException(
          'File is larger than the 512 KB preview limit.',
        );
      }
      final bytes = await file.readAsBytes();
      if (bytes.contains(0)) {
        throw const FormatException('Binary files cannot be previewed.');
      }
      final preview = utf8.decode(bytes);
      if (mounted) {
        setState(() => _preview = preview);
      }
    } on FileSystemException catch (error) {
      if (mounted) {
        setState(() => _error = error.message);
      }
    } on FormatException catch (error) {
      if (mounted) {
        setState(() => _error = error.message);
      }
    } finally {
      if (mounted) {
        setState(() => _loading = false);
      }
    }
  }

  static bool _samePath(FileSystemEntity a, FileSystemEntity b) =>
      a.absolute.path == b.absolute.path;
}

class _FileAction extends StatelessWidget {
  const _FileAction({
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.enabled = true,
  });

  final IconData icon;
  final String tooltip;
  final VoidCallback onPressed;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Tooltip(
      message: tooltip,
      child: IconButton(
        padding: EdgeInsets.zero,
        constraints: const BoxConstraints.tightFor(width: 38, height: 38),
        onPressed: enabled ? onPressed : null,
        icon: Icon(
          icon,
          size: 15,
          color: enabled ? colors.textSecondary : colors.divider,
        ),
      ),
    );
  }
}
