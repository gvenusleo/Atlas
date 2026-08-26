import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';

/// Hosts permission dialogs for the current workspace.
///
/// Watches pending agent permission requests and shows one dialog at a time;
/// dismissing the dialog rejects the request so the agent never waits.
class PermissionHost extends ConsumerStatefulWidget {
  /// Creates a permission host wrapping [child].
  const PermissionHost({super.key, required this.child});

  /// The workspace content rendered beneath the dialogs.
  final Widget child;

  @override
  ConsumerState<PermissionHost> createState() => _PermissionHostState();
}

class _PermissionHostState extends ConsumerState<PermissionHost> {
  bool _showing = false;

  @override
  Widget build(BuildContext context) {
    ref.listen<List<PermissionRequest>>(
      workspaceProvider.select((state) => state.pendingPermissions),
      (previous, next) {
        if (next.isNotEmpty && !_showing) {
          _showNext(next.first);
        }
      },
    );
    return widget.child;
  }

  Future<void> _showNext(PermissionRequest request) async {
    _showing = true;
    final reply = await showDialog<PermissionReply>(
      context: context,
      builder: (context) => PermissionDialog(request: request),
    );
    _showing = false;
    final controller = ref.read(workspaceProvider.notifier);
    await controller.respondPermission(
      request.requestId,
      reply ?? PermissionReply.reject,
    );
    final remaining = ref.read(workspaceProvider).pendingPermissions;
    if (remaining.isNotEmpty) {
      _showNext(remaining.first);
    }
  }
}

/// A permission request dialog with allow-once, allow-always, and reject.
class PermissionDialog extends StatelessWidget {
  /// Creates a permission dialog for [request].
  const PermissionDialog({super.key, required this.request});

  /// The pending request being answered.
  final PermissionRequest request;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final allowOnce = _optionFor(PermissionReply.allowOnce);
    final allowAlways = _optionFor(PermissionReply.allowAlways);
    return AlertDialog(
      backgroundColor: colors.panel,
      title: Row(
        children: [
          Icon(LucideIcons.shieldAlert, color: colors.accent, size: 20),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              'Allow ${request.toolName}?',
              style: const TextStyle(fontSize: 15),
            ),
          ),
        ],
      ),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (request.title.isNotEmpty)
              Text(
                request.title,
                style: TextStyle(color: colors.textSecondary, fontSize: 12.5),
              ),
            if (request.input.isNotEmpty) ...[
              const SizedBox(height: 10),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(10),
                decoration: BoxDecoration(
                  color: colors.raised,
                  borderRadius: BorderRadius.circular(AtlasRadii.control),
                ),
                child: Text(
                  const JsonEncoder.withIndent('  ').convert(request.input),
                  style: TextStyle(
                    color: colors.textSecondary,
                    fontSize: 11.5,
                    height: 1.4,
                    fontFamily: 'monospace',
                  ),
                ),
              ),
            ],
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(PermissionReply.reject),
          child: Text(
            _optionLabel(allowOnce, 'Reject'),
            style: TextStyle(color: colors.textSecondary),
          ),
        ),
        TextButton(
          onPressed: () => Navigator.of(context).pop(PermissionReply.allowOnce),
          child: Text(_optionLabel(allowOnce, 'Allow once')),
        ),
        if (allowAlways != null)
          FilledButton(
            onPressed: () =>
                Navigator.of(context).pop(PermissionReply.allowAlways),
            child: Text(_optionLabel(allowAlways, 'Always allow')),
          ),
      ],
    );
  }

  /// The first option matching [reply], if the agent offered one.
  PermissionOption? _optionFor(PermissionReply reply) {
    for (final option in request.options) {
      if (option.kind == reply) {
        return option;
      }
    }
    return null;
  }

  /// The agent-provided option label, falling back to [fallback].
  static String _optionLabel(PermissionOption? option, String fallback) {
    if (option == null || option.name.isEmpty) {
      return fallback;
    }
    return option.name;
  }
}
