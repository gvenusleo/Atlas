import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../../shared/theme/atlas_theme.dart';
import 'widgets/workspace_controls.dart';
import 'widgets/workspace_panels.dart';
import 'workspace_metrics.dart';

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
  double _leftWidth = WorkspaceMetrics.leftDefaultWidth;
  double _rightWidth = WorkspaceMetrics.rightDefaultWidth;
  double _leftClosingWidth = WorkspaceMetrics.leftDefaultWidth;
  double _rightClosingWidth = WorkspaceMetrics.rightDefaultWidth;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        if (!WorkspaceMetrics.usesCompactNavigation &&
            constraints.maxWidth >= WorkspaceMetrics.desktopBreakpoint) {
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
                    child: SessionsPanel(
                      titlebarInset: WorkspaceMetrics.usesIntegratedTitlebar,
                      onToggle: () {
                        setState(() {
                          _leftClosingWidth = widths.left;
                          _leftVisible = false;
                        });
                      },
                    ),
                  ),
                  WorkspaceResizeHandle(
                    key: const ValueKey('atlas-left-resize-handle'),
                    panelOnLeft: true,
                    onDrag: (delta) => _resizeLeft(delta, availableWidth),
                  ),
                ],
              ),
            ),
            Expanded(
              child: WorkspacePanel(
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
                  WorkspaceResizeHandle(
                    key: const ValueKey('atlas-right-resize-handle'),
                    panelOnLeft: false,
                    onDrag: (delta) => _resizeRight(delta, availableWidth),
                  ),
                  SizedBox(
                    key: const ValueKey('atlas-right-panel'),
                    width: _rightVisible ? widths.right : _rightClosingWidth,
                    child: DetailsPanel(
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
          child: SessionsPanel(onClose: () => Navigator.of(context).pop()),
        ),
      ),
      endDrawer: Drawer(
        width: drawerWidth,
        child: SafeArea(
          child: DetailsPanel(onClose: () => Navigator.of(context).pop()),
        ),
      ),
      body: SafeArea(
        child: WorkspacePanel(
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
      availableWidth -
          WorkspaceMetrics.centerMinimumWidth -
          visiblePanels * WorkspaceMetrics.resizeHandleWidth,
      0,
    );
    if (visiblePanels == 0) {
      return const _PanelWidths();
    }

    var left = _leftVisible
        ? _leftWidth.clamp(
            WorkspaceMetrics.leftMinimumWidth,
            WorkspaceMetrics.leftMaximumWidth,
          )
        : 0.0;
    var right = _rightVisible
        ? _rightWidth.clamp(
            WorkspaceMetrics.rightMinimumWidth,
            WorkspaceMetrics.rightMaximumWidth,
          )
        : 0.0;
    if (left + right <= sideBudget) {
      return _PanelWidths(left: left, right: right);
    }

    final minimumTotal =
        (_leftVisible ? WorkspaceMetrics.leftMinimumWidth : 0.0) +
        (_rightVisible ? WorkspaceMetrics.rightMinimumWidth : 0.0);
    final distributable = math.max(sideBudget - minimumTotal, 0.0);
    final leftFlex = _leftVisible
        ? left - WorkspaceMetrics.leftMinimumWidth
        : 0.0;
    final rightFlex = _rightVisible
        ? right - WorkspaceMetrics.rightMinimumWidth
        : 0.0;
    final totalFlex = leftFlex + rightFlex;
    if (totalFlex > 0) {
      left = _leftVisible
          ? WorkspaceMetrics.leftMinimumWidth +
                distributable * leftFlex / totalFlex
          : 0.0;
      right = _rightVisible
          ? WorkspaceMetrics.rightMinimumWidth +
                distributable * rightFlex / totalFlex
          : 0.0;
    } else {
      left = _leftVisible ? WorkspaceMetrics.leftMinimumWidth : 0.0;
      right = _rightVisible ? WorkspaceMetrics.rightMinimumWidth : 0.0;
    }
    return _PanelWidths(left: left, right: right);
  }

  void _resizeLeft(double delta, double availableWidth) {
    final widths = _resolvePanelWidths(availableWidth);
    final right = _rightVisible ? widths.right : 0.0;
    final dividers = (_leftVisible ? 1 : 0) + (_rightVisible ? 1 : 0);
    final maximum = math.min(
      WorkspaceMetrics.leftMaximumWidth,
      availableWidth -
          WorkspaceMetrics.centerMinimumWidth -
          right -
          dividers * WorkspaceMetrics.resizeHandleWidth,
    );
    setState(() {
      _leftWidth = (widths.left + delta).clamp(
        WorkspaceMetrics.leftMinimumWidth,
        maximum,
      );
    });
  }

  void _resizeRight(double delta, double availableWidth) {
    final widths = _resolvePanelWidths(availableWidth);
    final left = _leftVisible ? widths.left : 0.0;
    final dividers = (_leftVisible ? 1 : 0) + (_rightVisible ? 1 : 0);
    final maximum = math.min(
      WorkspaceMetrics.rightMaximumWidth,
      availableWidth -
          WorkspaceMetrics.centerMinimumWidth -
          left -
          dividers * WorkspaceMetrics.resizeHandleWidth,
    );
    setState(() {
      _rightWidth = (widths.right - delta).clamp(
        WorkspaceMetrics.rightMinimumWidth,
        maximum,
      );
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
      duration: reduceMotion
          ? Duration.zero
          : WorkspaceMetrics.sidebarAnimationDuration,
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
