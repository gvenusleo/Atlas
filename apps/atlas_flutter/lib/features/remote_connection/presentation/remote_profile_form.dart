import 'package:material_ui/material_ui.dart';

import '../data/remote_connections.dart';
import '../../../shared/theme/atlas_theme.dart';
import '../../../l10n/localizations.dart';
import '../../../shared/widgets/animated_caret.dart';

/// Form for creating or editing a remote connection profile.
///
/// [workingDirectory] is optional: it is only needed once the first message
/// is sent, and the app asks for it then (an absolute path on the computer,
/// for example `/home/you/projects`).
class const RemoteProfileFormDialog({
  super.key,

  /// The profile being edited, or null for a new connection.
  final RemoteConnectionProfile? profile,
}) extends StatefulWidget {
  /// Creates the form; [profile] pre-fills an existing connection.
  this;

  @override
  State<RemoteProfileFormDialog> createState() =>
      _RemoteProfileFormDialogState();
}

class _RemoteProfileFormDialogState extends State<RemoteProfileFormDialog> {
  final _name = TextEditingController();
  final _wsUrl = TextEditingController();
  final _token = TextEditingController();
  final _workingDirectory = TextEditingController();
  int? _error;

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
            ? context.l10n.addRemoteConnection
            : context.l10n.editRemoteConnection,
      ),
      content: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 420),
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _field(_name, context.l10n.name, context.l10n.myComputer),
              const SizedBox(height: 10),
              _field(
                _wsUrl,
                context.l10n.webSocketUrl,
                'ws://my-computer:8765/acp',
              ),
              const SizedBox(height: 10),
              _field(_token, context.l10n.token, context.l10n.tokenHint),
              const SizedBox(height: 10),
              _field(
                _workingDirectory,
                context.l10n.remoteWorkingDirectoryOptional,
                '/home/you/projects — needed for the first message',
              ),
              if (_error != null) ...[
                const SizedBox(height: 10),
                Text(switch (_error) {
                  1 => context.l10n.nameRequired,
                  2 => context.l10n.urlRequired,
                  _ => context.l10n.tokenRequired,
                }, style: TextStyle(color: colors.error, fontSize: 12)),
              ],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: Text(context.l10n.cancel),
        ),
        FilledButton(onPressed: _submit, child: Text(context.l10n.save)),
      ],
    );
  }

  Widget _field(TextEditingController controller, String label, String hint) {
    return AnimatedCaret(
      controller: controller,
      child: TextField(
        showCursor: false,
        controller: controller,
        decoration: InputDecoration(
          labelText: label,
          hintText: hint,
          isDense: true,
          border: const OutlineInputBorder(),
        ),
      ),
    );
  }

  void _submit() {
    final name = _name.text.trim();
    final wsUrl = _wsUrl.text.trim();
    final token = _token.text.trim();
    final workingDirectory = _workingDirectory.text.trim();
    if (name.isEmpty) {
      setState(() => _error = 1);
      return;
    }
    if (wsUrl.isEmpty) {
      setState(() => _error = 2);
      return;
    }
    if (token.isEmpty) {
      setState(() => _error = 3);
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
