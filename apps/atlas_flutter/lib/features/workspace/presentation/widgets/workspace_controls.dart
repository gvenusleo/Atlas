import 'dart:async';

import 'dart:io';

import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';
import 'package:morphnext/morphnext.dart';
import 'package:window_manager/window_manager.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../workspace_metrics.dart';

/// Whether the host window lacks native edge resizing and needs the
/// in-widget resize ring. macOS keeps native resize edges, and the
/// window_manager Windows plugin installs hit-test edges for frameless
/// windows; on Linux the hidden title bar leaves the GTK window without
/// native decorations, which drops both SSD and CSD edges on every backend.
bool get needsResizeRing {
  if (!WorkspaceMetrics.usesIntegratedTitlebar || !Platform.isLinux) {
    return false;
  }
  return true;
}

/// Whether custom minimize/maximize/close buttons should be shown. Tiling
/// Wayland compositors (Hyprland, Sway, i3) drive those commands through
/// keybindings and never show caption buttons, so the toolbar keeps a
/// clean right edge there; floating desktops keep the controls.
bool usesCaptionControls({
  required String desktop,
  required String sessionType,
}) {
  if (!WorkspaceMetrics.usesIntegratedTitlebar) {
    return false;
  }
  if (!Platform.isLinux) {
    return true;
  }
  // Empty XDG_CURRENT_DESKTOP values fall through to showing the controls:
  // an unidentifiable compositor is assumed to be floating-desktop-like.
  final onTilingCompositor =
      sessionType == 'wayland' &&
      const ['hyprland', 'sway', 'i3'].contains(desktop.toLowerCase());
  return !onTilingCompositor;
}

/// A transparent ring around the workspace that restores edge resizing on
/// platforms where the hidden title bar removes native resize handles.
class WorkspaceResizeRing extends StatelessWidget {
  const WorkspaceResizeRing({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    if (!needsResizeRing) {
      return child;
    }
    return DragToResizeArea(resizeEdgeSize: 4, child: child);
  }
}

/// Minimize, maximize and close buttons for platforms without native
/// caption controls, matching the workspace toolbar's visual language.
class AtlasWindowControls extends StatefulWidget {
  const AtlasWindowControls({super.key});

  @override
  State<AtlasWindowControls> createState() => _AtlasWindowControlsState();
}

class _AtlasWindowControlsState extends State<AtlasWindowControls>
    with WindowListener {
  bool _maximized = false;

  @override
  void initState() {
    super.initState();
    windowManager.addListener(this);
    unawaited(_refreshMaximized());
  }

  @override
  void dispose() {
    windowManager.removeListener(this);
    super.dispose();
  }

  @override
  void onWindowMaximize() {
    if (mounted) {
      setState(() => _maximized = true);
    }
  }

  @override
  void onWindowUnmaximize() {
    if (mounted) {
      setState(() => _maximized = false);
    }
  }

  Future<void> _toggleMaximize() async {
    if (await windowManager.isMaximized()) {
      await windowManager.unmaximize();
    } else {
      await windowManager.maximize();
    }
  }

  Future<void> _refreshMaximized() async {
    final maximized = await windowManager.isMaximized();
    if (mounted && maximized != _maximized) {
      setState(() => _maximized = maximized);
    }
  }

  @override
  Widget build(BuildContext context) {
    // The glyph mirrors the resulting state: tapping restore shows the
    // overlaid squares, tapping maximize shows the plain square.
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        WorkspaceToolbarButton(
          icon: LucideIcons.minus,
          tooltip: 'Minimize',
          onPressed: windowManager.minimize,
        ),
        WorkspaceToolbarButton(
          icon: _maximized ? LucideIcons.copy : LucideIcons.square,
          tooltip: _maximized ? 'Restore' : 'Maximize',
          onPressed: () async {
            await _toggleMaximize();
            await _refreshMaximized();
          },
        ),
        WorkspaceToolbarButton(
          icon: LucideIcons.x,
          tooltip: 'Close',
          onPressed: windowManager.close,
        ),
      ],
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
          icon: AnimatedMorphIcon(
            icon: widget.icon,
            color: widget.active ? colors.textPrimary : colors.textSecondary,
            size: 16,
            semanticLabel: widget.tooltip,
          ),
        ),
      ),
    );
  }
}
