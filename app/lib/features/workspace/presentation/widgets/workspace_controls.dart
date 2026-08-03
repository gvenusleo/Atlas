import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:window_manager/window_manager.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../workspace_metrics.dart';

/// Tab-shaped label shown in the workspace toolbar.
class WorkspaceTab extends StatelessWidget {
  const WorkspaceTab({super.key, required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return Container(
      height: 32,
      constraints: const BoxConstraints(minWidth: 112, maxWidth: 180),
      padding: const EdgeInsets.symmetric(horizontal: 12),
      alignment: Alignment.centerLeft,
      decoration: BoxDecoration(
        color: colors.raised,
        borderRadius: BorderRadius.circular(AtlasRadii.control),
      ),
      child: Text(
        label,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(
          color: colors.textPrimary,
          fontSize: 12,
          fontWeight: FontWeight.w500,
        ),
      ),
    );
  }
}

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
                  Expanded(
                    child: ColoredBox(
                      color: widget.panelOnLeft ? colors.canvas : colors.panel,
                    ),
                  ),
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

/// Cupertino toolbar action with Atlas hover and tooltip styling.
class WorkspaceToolbarButton extends StatefulWidget {
  const WorkspaceToolbarButton({
    super.key,
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.active = false,
    this.size = 40,
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
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final reduceMotion = MediaQuery.disableAnimationsOf(context);
    final colors = AtlasColors.of(context);

    return MouseRegion(
      onEnter: (_) => setState(() => _hovered = true),
      onExit: (_) => setState(() => _hovered = false),
      child: AnimatedContainer(
        duration: reduceMotion
            ? Duration.zero
            : const Duration(milliseconds: 120),
        width: widget.size,
        height: widget.size,
        decoration: BoxDecoration(
          color: _hovered ? colors.raised : null,
          borderRadius: BorderRadius.circular(AtlasRadii.control),
        ),
        child: Tooltip(
          message: widget.tooltip,
          child: CupertinoButton(
            padding: EdgeInsets.zero,
            minimumSize: Size.square(widget.size),
            pressedOpacity: 1,
            focusColor: Colors.transparent,
            onPressed: widget.onPressed,
            child: Icon(widget.icon, color: colors.textSecondary, size: 18),
          ),
        ),
      ),
    );
  }
}
