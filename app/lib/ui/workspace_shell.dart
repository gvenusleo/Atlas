import 'dart:math' as math;

import 'package:flutter/cupertino.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:window_manager/window_manager.dart';

import 'atlas_theme.dart';

const _desktopBreakpoint = 960.0;
const _centerMinimumWidth = 420.0;
const _leftDefaultWidth = 224.0;
const _leftMinimumWidth = 184.0;
const _leftMaximumWidth = 360.0;
const _rightDefaultWidth = 260.0;
const _rightMinimumWidth = 220.0;
const _rightMaximumWidth = 380.0;
const _resizeHandleWidth = 8.0;
const _desktopToolbarHeight = 44.0;
const _compactToolbarHeight = 48.0;
const _macOSTrafficLightInset = 76.0;
const _sidebarAnimationDuration = Duration(milliseconds: 180);

bool get _usesIntegratedTitlebar =>
    !kIsWeb && defaultTargetPlatform == TargetPlatform.macOS;

/// Whether the platform uses touch-first navigation with drawers.
bool get _usesCompactNavigation =>
    !kIsWeb &&
    (defaultTargetPlatform == TargetPlatform.android ||
        defaultTargetPlatform == TargetPlatform.iOS);

/// Responsive Atlas workspace with desktop side panels and compact drawers.
class WorkspaceShell extends StatefulWidget {
  const WorkspaceShell({super.key});

  @override
  State<WorkspaceShell> createState() => _WorkspaceShellState();
}

class _WorkspaceShellState extends State<WorkspaceShell> {
  final _scaffoldKey = GlobalKey<ScaffoldState>();

  bool _leftVisible = true;
  bool _rightVisible = true;
  double _leftWidth = _leftDefaultWidth;
  double _rightWidth = _rightDefaultWidth;
  double _leftClosingWidth = _leftDefaultWidth;
  double _rightClosingWidth = _rightDefaultWidth;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        if (!_usesCompactNavigation &&
            constraints.maxWidth >= _desktopBreakpoint) {
          return _buildDesktop(constraints.maxWidth);
        }
        return _buildCompact(constraints.maxWidth);
      },
    );
  }

  Widget _buildDesktop(double availableWidth) {
    final widths = _resolvePanelWidths(availableWidth);

    return Scaffold(
      body: SafeArea(
        child: Row(
          children: [
            _AnimatedSideRegion(
              visible: _leftVisible,
              alignment: Alignment.centerLeft,
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  SizedBox(
                    key: const ValueKey('atlas-left-panel'),
                    width: _leftVisible ? widths.left : _leftClosingWidth,
                    child: _SessionsPanel(
                      titlebarInset: _usesIntegratedTitlebar,
                      onToggle: () {
                        setState(() {
                          _leftClosingWidth = widths.left;
                          _leftVisible = false;
                        });
                      },
                    ),
                  ),
                  _ResizeHandle(
                    key: const ValueKey('atlas-left-resize-handle'),
                    panelOnLeft: true,
                    onDrag: (delta) => _resizeLeft(delta, availableWidth),
                  ),
                ],
              ),
            ),
            Expanded(
              child: _WorkspacePanel(
                compact: false,
                leftActive: _leftVisible,
                rightActive: _rightVisible,
                onLeftPressed: () => setState(() => _leftVisible = true),
                onRightPressed: () => setState(() => _rightVisible = true),
              ),
            ),
            _AnimatedSideRegion(
              visible: _rightVisible,
              alignment: Alignment.centerRight,
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  _ResizeHandle(
                    key: const ValueKey('atlas-right-resize-handle'),
                    panelOnLeft: false,
                    onDrag: (delta) => _resizeRight(delta, availableWidth),
                  ),
                  SizedBox(
                    key: const ValueKey('atlas-right-panel'),
                    width: _rightVisible ? widths.right : _rightClosingWidth,
                    child: _DetailsPanel(
                      onToggle: () {
                        setState(() {
                          _rightClosingWidth = widths.right;
                          _rightVisible = false;
                        });
                      },
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildCompact(double availableWidth) {
    final drawerWidth = math.min(availableWidth * 0.88, 320.0);
    final colors = AtlasColors.of(context);

    return Scaffold(
      key: _scaffoldKey,
      drawerScrimColor: colors.scrim,
      drawerEnableOpenDragGesture: true,
      endDrawerEnableOpenDragGesture: true,
      drawer: Drawer(
        width: drawerWidth,
        child: SafeArea(
          child: _SessionsPanel(onClose: () => Navigator.of(context).pop()),
        ),
      ),
      endDrawer: Drawer(
        width: drawerWidth,
        child: SafeArea(
          child: _DetailsPanel(onClose: () => Navigator.of(context).pop()),
        ),
      ),
      body: SafeArea(
        child: _WorkspacePanel(
          compact: true,
          leftActive: false,
          rightActive: false,
          onLeftPressed: () => _scaffoldKey.currentState?.openDrawer(),
          onRightPressed: () => _scaffoldKey.currentState?.openEndDrawer(),
        ),
      ),
    );
  }

  _PanelWidths _resolvePanelWidths(double availableWidth) {
    final visiblePanels = (_leftVisible ? 1 : 0) + (_rightVisible ? 1 : 0);
    final sideBudget = math.max(
      availableWidth - _centerMinimumWidth - visiblePanels * _resizeHandleWidth,
      0,
    );
    if (visiblePanels == 0) {
      return const _PanelWidths();
    }

    var left = _leftVisible
        ? _leftWidth.clamp(_leftMinimumWidth, _leftMaximumWidth)
        : 0.0;
    var right = _rightVisible
        ? _rightWidth.clamp(_rightMinimumWidth, _rightMaximumWidth)
        : 0.0;
    if (left + right <= sideBudget) {
      return _PanelWidths(left: left, right: right);
    }

    final minimumTotal =
        (_leftVisible ? _leftMinimumWidth : 0.0) +
        (_rightVisible ? _rightMinimumWidth : 0.0);
    final distributable = math.max(sideBudget - minimumTotal, 0.0);
    final leftFlex = _leftVisible ? left - _leftMinimumWidth : 0.0;
    final rightFlex = _rightVisible ? right - _rightMinimumWidth : 0.0;
    final totalFlex = leftFlex + rightFlex;
    if (totalFlex > 0) {
      left = _leftVisible
          ? _leftMinimumWidth + distributable * leftFlex / totalFlex
          : 0.0;
      right = _rightVisible
          ? _rightMinimumWidth + distributable * rightFlex / totalFlex
          : 0.0;
    } else {
      left = _leftVisible ? _leftMinimumWidth : 0.0;
      right = _rightVisible ? _rightMinimumWidth : 0.0;
    }
    return _PanelWidths(left: left, right: right);
  }

  void _resizeLeft(double delta, double availableWidth) {
    final widths = _resolvePanelWidths(availableWidth);
    final right = _rightVisible ? widths.right : 0.0;
    final dividers = (_leftVisible ? 1 : 0) + (_rightVisible ? 1 : 0);
    final maximum = math.min(
      _leftMaximumWidth,
      availableWidth -
          _centerMinimumWidth -
          right -
          dividers * _resizeHandleWidth,
    );
    setState(() {
      _leftWidth = (widths.left + delta).clamp(_leftMinimumWidth, maximum);
    });
  }

  void _resizeRight(double delta, double availableWidth) {
    final widths = _resolvePanelWidths(availableWidth);
    final left = _leftVisible ? widths.left : 0.0;
    final dividers = (_leftVisible ? 1 : 0) + (_rightVisible ? 1 : 0);
    final maximum = math.min(
      _rightMaximumWidth,
      availableWidth -
          _centerMinimumWidth -
          left -
          dividers * _resizeHandleWidth,
    );
    setState(() {
      _rightWidth = (widths.right - delta).clamp(_rightMinimumWidth, maximum);
    });
  }
}

/// Clips a desktop sidebar toward its anchored window edge while it animates.
class _AnimatedSideRegion extends StatelessWidget {
  const _AnimatedSideRegion({
    required this.visible,
    required this.alignment,
    required this.child,
  });

  final bool visible;
  final Alignment alignment;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final reduceMotion = MediaQuery.disableAnimationsOf(context);
    final target = visible ? 1.0 : 0.0;

    return TweenAnimationBuilder<double>(
      tween: Tween<double>(begin: target, end: target),
      duration: reduceMotion ? Duration.zero : _sidebarAnimationDuration,
      curve: Curves.easeOutCubic,
      builder: (context, factor, child) {
        if (factor == 0) {
          return const SizedBox.shrink();
        }
        return ClipRect(
          child: Align(alignment: alignment, widthFactor: factor, child: child),
        );
      },
      child: child,
    );
  }
}

class _PanelWidths {
  const _PanelWidths({this.left = 0, this.right = 0});

  final double left;
  final double right;
}

class _SessionsPanel extends StatelessWidget {
  const _SessionsPanel({
    this.onClose,
    this.onToggle,
    this.titlebarInset = false,
  });

  final VoidCallback? onClose;
  final VoidCallback? onToggle;
  final bool titlebarInset;

  @override
  Widget build(BuildContext context) {
    return _SidePanel(
      title: 'Sessions',
      compact: onClose != null,
      leadingInset: titlebarInset ? _macOSTrafficLightInset : 0,
      action: onClose != null
          ? _ToolbarButton(
              icon: CupertinoIcons.xmark,
              tooltip: 'Close sessions',
              size: 44,
              onPressed: onClose!,
            )
          : onToggle != null
          ? _ToolbarButton(
              key: const ValueKey('atlas-left-toggle'),
              icon: CupertinoIcons.sidebar_left,
              tooltip: 'Hide sessions',
              active: true,
              onPressed: onToggle!,
            )
          : null,
      child: const _EmptyPanelState(
        icon: CupertinoIcons.bubble_left_bubble_right,
        message: 'No sessions yet',
      ),
    );
  }
}

class _DetailsPanel extends StatelessWidget {
  const _DetailsPanel({this.onClose, this.onToggle});

  final VoidCallback? onClose;
  final VoidCallback? onToggle;

  @override
  Widget build(BuildContext context) {
    return _SidePanel(
      title: 'Details',
      compact: onClose != null,
      action: onClose != null
          ? _ToolbarButton(
              icon: CupertinoIcons.xmark,
              tooltip: 'Close details',
              size: 44,
              onPressed: onClose!,
            )
          : onToggle != null
          ? _ToolbarButton(
              key: const ValueKey('atlas-right-toggle'),
              icon: CupertinoIcons.sidebar_right,
              tooltip: 'Hide details',
              active: true,
              onPressed: onToggle!,
            )
          : null,
      child: const _EmptyPanelState(
        icon: CupertinoIcons.info_circle,
        message: 'No active session',
      ),
    );
  }
}

class _SidePanel extends StatelessWidget {
  const _SidePanel({
    required this.title,
    required this.child,
    this.compact = false,
    this.action,
    this.leadingInset = 0,
  });

  final String title;
  final Widget child;
  final bool compact;
  final Widget? action;
  final double leadingInset;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return ColoredBox(
      color: colors.panel,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _TitlebarDragArea(
            child: SizedBox(
              height: compact ? _compactToolbarHeight : _desktopToolbarHeight,
              child: Padding(
                padding: EdgeInsets.only(left: 12 + leadingInset, right: 6),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        title,
                        maxLines: 1,
                        overflow: TextOverflow.clip,
                        style: TextStyle(
                          color: colors.textPrimary,
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                    ?action,
                  ],
                ),
              ),
            ),
          ),
          const Divider(),
          Expanded(child: child),
        ],
      ),
    );
  }
}

class _WorkspacePanel extends StatelessWidget {
  const _WorkspacePanel({
    required this.compact,
    required this.leftActive,
    required this.rightActive,
    required this.onLeftPressed,
    required this.onRightPressed,
  });

  final bool compact;
  final bool leftActive;
  final bool rightActive;
  final VoidCallback onLeftPressed;
  final VoidCallback onRightPressed;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final leftToolbarInset = _usesIntegratedTitlebar && (compact || !leftActive)
        ? _macOSTrafficLightInset
        : 6.0;

    return ColoredBox(
      key: const ValueKey('atlas-center-panel'),
      color: colors.canvas,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _TitlebarDragArea(
            child: SizedBox(
              height: compact ? _compactToolbarHeight : _desktopToolbarHeight,
              child: Padding(
                padding: EdgeInsets.only(left: leftToolbarInset, right: 6),
                child: Row(
                  children: [
                    if (compact || !leftActive)
                      _ToolbarButton(
                        key: const ValueKey('atlas-left-toggle'),
                        icon: CupertinoIcons.sidebar_left,
                        tooltip: compact ? 'Open sessions' : 'Show sessions',
                        size: compact ? 44 : 40,
                        onPressed: onLeftPressed,
                      ),
                    const SizedBox(width: 4),
                    const _WorkspaceTab(label: 'New session'),
                    const Spacer(),
                    if (compact || !rightActive)
                      _ToolbarButton(
                        key: const ValueKey('atlas-right-toggle'),
                        icon: CupertinoIcons.sidebar_right,
                        tooltip: compact ? 'Open details' : 'Show details',
                        size: compact ? 44 : 40,
                        onPressed: onRightPressed,
                      ),
                  ],
                ),
              ),
            ),
          ),
          const Divider(),
          const Expanded(child: Center(child: _EmptyWorkspaceState())),
        ],
      ),
    );
  }
}

class _WorkspaceTab extends StatelessWidget {
  const _WorkspaceTab({required this.label});

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

class _TitlebarDragArea extends StatelessWidget {
  const _TitlebarDragArea({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    if (!_usesIntegratedTitlebar) {
      return child;
    }
    // The drag area stays behind the content so its double-tap recognizer
    // never delays taps on toolbar controls.
    return Stack(
      children: [
        const Positioned.fill(
          child: DragToMoveArea(child: SizedBox.expand()),
        ),
        child,
      ],
    );
  }
}

class _EmptyWorkspaceState extends StatelessWidget {
  const _EmptyWorkspaceState();

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(CupertinoIcons.sparkles, color: colors.accent, size: 22),
          const SizedBox(height: 12),
          Text(
            'Start a new session',
            textAlign: TextAlign.center,
            style: TextStyle(
              color: colors.textPrimary,
              fontSize: 14,
              fontWeight: FontWeight.w500,
            ),
          ),
        ],
      ),
    );
  }
}

class _EmptyPanelState extends StatelessWidget {
  const _EmptyPanelState({required this.icon, required this.message});

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

class _ResizeHandle extends StatefulWidget {
  const _ResizeHandle({
    super.key,
    required this.panelOnLeft,
    required this.onDrag,
  });

  final bool panelOnLeft;
  final ValueChanged<double> onDrag;

  @override
  State<_ResizeHandle> createState() => _ResizeHandleState();
}

class _ResizeHandleState extends State<_ResizeHandle> {
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
          width: _resizeHandleWidth,
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
                top: _desktopToolbarHeight,
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

class _ToolbarButton extends StatefulWidget {
  const _ToolbarButton({
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
  State<_ToolbarButton> createState() => _ToolbarButtonState();
}

class _ToolbarButtonState extends State<_ToolbarButton> {
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
