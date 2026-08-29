import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../workspace_metrics.dart';
import 'workspace_controls.dart';

/// Shared scaffolding for the workspace side panels: a labeled toolbar row
/// above a scrolling body.
class SidePanel extends StatelessWidget {
  /// Creates a side panel.
  const SidePanel({
    super.key,
    required this.semanticLabel,
    required this.child,
    this.compact = false,
    this.title,
    this.action,
    this.useCanvasColor = false,
  });

  /// Accessibility label describing the panel contents.
  final String semanticLabel;
  final Widget child;

  /// Whether the panel is presented inside a compact drawer.
  final bool compact;
  final Widget? title;
  final Widget? action;
  final bool useCanvasColor;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Semantics(
      container: true,
      label: semanticLabel,
      child: ColoredBox(
        color: useCanvasColor ? colors.canvas : colors.panel,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            WorkspaceTitlebarDragArea(
              child: SizedBox(
                height: compact
                    ? WorkspaceMetrics.compactToolbarHeight
                    : WorkspaceMetrics.desktopToolbarHeight,
                child: Row(
                  children: [
                    Expanded(
                      child: Padding(
                        padding: const EdgeInsets.only(left: 4, top: 6),
                        child: Align(
                          alignment: Alignment.topLeft,
                          child: title,
                        ),
                      ),
                    ),
                    Padding(
                      padding: const EdgeInsets.only(right: 6),
                      child: action,
                    ),
                  ],
                ),
              ),
            ),
            const Divider(),
            Expanded(child: child),
          ],
        ),
      ),
    );
  }
}

/// Centered placeholder shown when a panel has nothing to display.
class PanelEmptyState extends StatelessWidget {
  /// Creates an empty state.
  const PanelEmptyState({super.key, required this.icon, required this.message});

  final IconData icon;
  final String message;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, color: colors.textSecondary, size: 18),
            const SizedBox(height: 10),
            Text(
              message,
              textAlign: TextAlign.center,
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 12,
                height: 1.4,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
