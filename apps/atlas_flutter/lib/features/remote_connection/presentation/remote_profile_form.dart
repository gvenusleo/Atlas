import 'package:material_ui/material_ui.dart';

import '../../../app/remote_connections.dart';
import '../../../shared/theme/atlas_theme.dart';

/// Form for creating or editing a remote connection profile.
///
/// [workingDirectory] is optional: it is only needed once the first message
/// is sent, and the app asks for it then (an absolute path on the computer,
/// for example `/home/you/projects`).
class RemoteProfileFormDialog extends StatefulWidget {
  /// Creates the form; [profile] pre-fills an existing connection.
  const RemoteProfileFormDialog({super.key, this.profile});

  /// The profile being edited, or null for a new connection.
  final RemoteConnectionProfile? profile;

  @override
  State<RemoteProfileFormDialog> createState() =>
      _RemoteProfileFormDialogState();
}

class _RemoteProfileFormDialogState extends State<RemoteProfileFormDialog> {
  final _name = TextEditingController();
  final _wsUrl = TextEditingController();
  final _token = TextEditingController();
  final _workingDirectory = TextEditingController();
  String? _error;

  @override
  void initState() {
    super.initState();
    final profile = widget.profile;
    if (profile != null) {
      _name.text = profile.name;
      _wsUrl.text = profile.wsUrl;
      _token.text = profile.token;
      _workingDirectory.text = profile.workingDirectory ?? '';
    }
  }

  @override
  void dispose() {
    _name.dispose();
    _wsUrl.dispose();
    _token.dispose();
    _workingDirectory.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return AlertDialog(
      title: Text(
        widget.profile == null
            ? 'Add remote connection'
            : 'Edit remote connection',
      ),
      content: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 420),
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _field(_name, 'Name', 'My computer'),
              const SizedBox(height: 10),
              _field(_wsUrl, 'WebSocket URL', 'ws://my-computer:8765/acp'),
              const SizedBox(height: 10),
              _field(_token, 'Token', 'printed by `atlas server` on startup'),
              const SizedBox(height: 10),
              _field(
                _workingDirectory,
                'Working directory on the computer (optional)',
                '/home/you/projects — needed for the first message',
              ),
              if (_error != null) ...[
                const SizedBox(height: 10),
                Text(
                  _error!,
                  style: TextStyle(color: colors.error, fontSize: 12),
                ),
              ],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(onPressed: _submit, child: const Text('Save')),
      ],
    );
  }

  Widget _field(TextEditingController controller, String label, String hint) {
    return TextField(
      controller: controller,
      decoration: InputDecoration(
        labelText: label,
        hintText: hint,
        isDense: true,
        border: const OutlineInputBorder(),
      ),
    );
  }

  void _submit() {
    final name = _name.text.trim();
    final wsUrl = _wsUrl.text.trim();
    final token = _token.text.trim();
    final workingDirectory = _workingDirectory.text.trim();
    if (name.isEmpty) {
      setState(() => _error = 'Name is required.');
      return;
    }
    if (wsUrl.isEmpty) {
      setState(() => _error = 'WebSocket URL is required.');
      return;
    }
    if (token.isEmpty) {
      setState(() => _error = 'Token is required.');
      return;
    }
    Navigator.of(context).pop(
      RemoteConnectionProfile(
        name: name,
        wsUrl: wsUrl,
        token: token,
        // Null until the first message is sent: the app asks for the
        // directory when a session actually needs it.
        workingDirectory: workingDirectory.isEmpty ? null : workingDirectory,
      ),
    );
  }
}
