import 'package:material_ui/material_ui.dart';
import 'package:window_manager/window_manager.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../workspace_metrics.dart';

/// Makes a toolbar draggable on platforms with an integrated titlebar.
class WorkspaceTitlebarDragArea extends StatelessWidget {
  const WorkspaceTitlebarDragArea({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    if (!WorkspaceMetrics.usesIntegratedTitlebar) {
      return child;
    }
    // The drag area stays behind the content so its double-tap recognizer
    // never delays taps on toolbar controls.
    return Stack(
      children: [
        const Positioned.fill(child: DragToMoveArea(child: SizedBox.expand())),
        child,
      ],
    );
  }
}

/// Resizable gutter joining a desktop sidebar to the central workspace.
class WorkspaceResizeHandle extends StatefulWidget {
  const WorkspaceResizeHandle({
    super.key,
    required this.panelOnLeft,
    required this.onDrag,
  });

  final bool panelOnLeft;
  final ValueChanged<double> onDrag;

  @override
  State<WorkspaceResizeHandle> createState() => _WorkspaceResizeHandleState();
}

class _WorkspaceResizeHandleState extends State<WorkspaceResizeHandle> {
  bool _hovered = false;
  bool _dragging = false;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return MouseRegion(
      cursor: SystemMouseCursors.resizeColumn,
      onEnter: (_) => setState(() => _hovered = true),
      onExit: (_) => setState(() => _hovered = false),
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onHorizontalDragStart: (_) => setState(() => _dragging = true),
        onHorizontalDragUpdate: (details) => widget.onDrag(details.delta.dx),
        onHorizontalDragEnd: (_) => setState(() => _dragging = false),
        onHorizontalDragCancel: () => setState(() => _dragging = false),
        child: SizedBox(
          width: WorkspaceMetrics.resizeHandleWidth,
          child: Stack(
            fit: StackFit.expand,
            children: [
              Row(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Expanded(
                    child: ColoredBox(
                      color: widget.panelOnLeft ? colors.panel : colors.canvas,
                    ),
                  ),
                  Expanded(child: ColoredBox(color: colors.canvas)),
                ],
              ),
              Positioned(
                top: WorkspaceMetrics.desktopToolbarHeight,
                left: 0,
                right: 0,
                child: SizedBox(
                  key: const ValueKey('atlas-resize-header-divider'),
                  height: 1,
                  child: DecoratedBox(
                    decoration: BoxDecoration(color: colors.divider),
                  ),
                ),
              ),
              Center(
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 120),
                  width: 1,
                  color: _hovered || _dragging ? colors.accent : colors.divider,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Paints the workspace hover highlight behind its child.
///
/// While the pointer is over the child, [hoveredColor] (defaults to the
/// raised surface color) fades in as a rounded background behind the child,
/// matching the shared button hover interaction.
class WorkspaceHoverSurface extends StatefulWidget {
  /// Creates a hover-highlighted surface.
  const WorkspaceHoverSurface({
    super.key,
    required this.child,
    this.color,
    this.hoveredColor,
    this.borderRadius = const BorderRadius.all(
      Radius.circular(AtlasRadii.control),
    ),
    this.enabled = true,
  });

  /// Background color while not hovered.
  final Color? color;

  /// Background color while hovered; defaults to the raised surface color.
  final Color? hoveredColor;

  /// Corner radius of the hover background.
  final BorderRadiusGeometry borderRadius;

  /// Whether hover tracking is active.
  final bool enabled;

  /// The child painted above the hover background.
  final Widget child;

  @override
  State<WorkspaceHoverSurface> createState() => _WorkspaceHoverSurfaceState();
}

class _WorkspaceHoverSurfaceState extends State<WorkspaceHoverSurface> {
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final reduceMotion = MediaQuery.disableAnimationsOf(context);
    return MouseRegion(
      onEnter: widget.enabled ? (_) => setState(() => _hovered = true) : null,
      onExit: widget.enabled ? (_) => setState(() => _hovered = false) : null,
      child: AnimatedContainer(
        duration: reduceMotion
            ? Duration.zero
            : const Duration(milliseconds: 120),
        decoration: BoxDecoration(
          color: _hovered
              ? (widget.hoveredColor ?? colors.raised)
              : widget.color,
          borderRadius: widget.borderRadius,
        ),
        child: widget.child,
      ),
    );
  }
}

/// Cupertino toolbar action with Atlas hover and tooltip styling.
class WorkspaceToolbarButton extends StatefulWidget {
  const WorkspaceToolbarButton({
    super.key,
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.active = false,
    this.size = WorkspaceMetrics.desktopToolbarButtonSize,
  });

  final IconData icon;
  final String tooltip;
  final VoidCallback onPressed;
  final bool active;

  /// Square side length; use a larger value for touch layouts.
  final double size;

  @override
  State<WorkspaceToolbarButton> createState() => _WorkspaceToolbarButtonState();
}

class _WorkspaceToolbarButtonState extends State<WorkspaceToolbarButton> {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return WorkspaceHoverSurface(
      // Active buttons keep the highlight while not hovered.
      color: widget.active ? colors.raised : null,
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: Tooltip(
        message: widget.tooltip,
        child: IconButton(
          padding: EdgeInsets.zero,
          constraints: BoxConstraints.tightFor(
            width: widget.size,
            height: widget.size,
          ),
          style: IconButton.styleFrom(
            // The outer surface paints the hover/active background as a
            // rounded rectangle; keep the button's own overlay transparent
            // so it never draws a circular highlight on top.
            overlayColor: Colors.transparent,
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(AtlasRadii.control),
            ),
          ),
          onPressed: widget.onPressed,
          icon: Icon(
            widget.icon,
            color: widget.active ? colors.textPrimary : colors.textSecondary,
            size: 16,
          ),
        ),
      ),
    );
  }
}
