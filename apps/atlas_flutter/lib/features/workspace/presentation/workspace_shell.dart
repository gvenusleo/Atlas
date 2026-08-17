import 'dart:async';
import 'dart:math' as math;

import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import '../../../app/runtime_environment.dart';
import '../../../shared/theme/atlas_theme.dart';
import '../application/workspace_controller.dart';
import 'widgets/workspace_controls.dart';
import 'widgets/workspace_panels.dart';
import 'workspace_metrics.dart';

/// Responsive Atlas workspace with desktop side panels and compact drawers.
class WorkspaceShell extends ConsumerStatefulWidget {
  const WorkspaceShell({super.key, this.environment, this.startupError});

  /// Shared runtime services, absent when bootstrap failed or in shell tests.
  final RuntimeEnvironment? environment;

  /// Configuration error shown in the empty conversation state.
  final String? startupError;

  @override
  ConsumerState<WorkspaceShell> createState() => _WorkspaceShellState();
}

class _WorkspaceShellState extends ConsumerState<WorkspaceShell>
    with TickerProviderStateMixin {
  final _scaffoldKey = GlobalKey<ScaffoldState>();

  late final AnimationController _leftSidebarAnimation;
  late final AnimationController _rightSidebarAnimation;
  bool _leftVisible = true;
  bool _rightVisible = true;
  double _leftWidth = WorkspaceMetrics.leftDefaultWidth;
  double _rightWidth = WorkspaceMetrics.rightDefaultWidth;
  double _leftClosingWidth = WorkspaceMetrics.leftDefaultWidth;
  double _rightClosingWidth = WorkspaceMetrics.rightDefaultWidth;

  @override
  void initState() {
    super.initState();
    if (widget.environment != null) {
      // Load the session list after the first frame so the workspace provider
      // already has listeners and its autoDispose lifecycle stays alive.
      SchedulerBinding.instance.addPostFrameCallback((_) {
        if (mounted) {
          ref.read(workspaceProvider.notifier).refreshSessions();
        }
      });
    }
    _leftSidebarAnimation = AnimationController(
      value: 1,
      duration: WorkspaceMetrics.sidebarAnimationDuration,
      vsync: this,
    );
    _rightSidebarAnimation = AnimationController(
      value: 1,
      duration: WorkspaceMetrics.sidebarAnimationDuration,
      vsync: this,
    );
  }

  @override
  void dispose() {
    _leftSidebarAnimation.dispose();
    _rightSidebarAnimation.dispose();
    super.dispose();
  }

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
    final leftPanelWidth = _leftVisible ? widths.left : _leftClosingWidth;
    final rightPanelWidth = _rightVisible ? widths.right : _rightClosingWidth;
    final closedLeftButtonX = WorkspaceMetrics.usesIntegratedTitlebar
        ? WorkspaceMetrics.macOSTrafficLightInset
        : 6.0;

    return Scaffold(
      body: SafeArea(
        child: Stack(
          children: [
            Positioned.fill(
              child: Row(
                children: [
                  _AnimatedSideRegion(
                    animation: _leftSidebarAnimation,
                    alignment: Alignment.centerLeft,
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        SizedBox(
                          key: const ValueKey('atlas-left-panel'),
                          width: leftPanelWidth,
                          child: const SessionsPanel(),
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
                      startupError: widget.startupError,
                      compact: false,
                      leftActive: _leftVisible,
                      onLeftPressed: () => _setLeftVisible(true, widths.left),
                      onRightPressed: () =>
                          _setRightVisible(true, widths.right),
                    ),
                  ),
                  _AnimatedSideRegion(
                    animation: _rightSidebarAnimation,
                    alignment: Alignment.centerRight,
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        WorkspaceResizeHandle(
                          key: const ValueKey('atlas-right-resize-handle'),
                          panelOnLeft: false,
                          onDrag: (delta) =>
                              _resizeRight(delta, availableWidth),
                        ),
                        SizedBox(
                          key: const ValueKey('atlas-right-panel'),
                          width: rightPanelWidth,
                          child: const DetailsPanel(),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
            Positioned(
              key: const ValueKey('atlas-left-toggle-positioned'),
              top: 6,
              left: closedLeftButtonX,
              child: WorkspaceToolbarButton(
                key: const ValueKey('atlas-left-toggle'),
                icon: LucideIcons.panelLeft,
                tooltip: _leftVisible ? 'Hide sessions' : 'Show sessions',
                onPressed: () => _setLeftVisible(!_leftVisible, leftPanelWidth),
              ),
            ),
            Positioned(
              key: const ValueKey('atlas-right-toggle-positioned'),
              top: 6,
              right: 6,
              child: WorkspaceToolbarButton(
                key: const ValueKey('atlas-right-toggle'),
                icon: LucideIcons.panelRight,
                tooltip: _rightVisible ? 'Hide details' : 'Show details',
                onPressed: () =>
                    _setRightVisible(!_rightVisible, rightPanelWidth),
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
          startupError: widget.startupError,
          compact: true,
          leftActive: false,
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

  void _setLeftVisible(bool visible, double panelWidth) {
    if (_leftVisible == visible) {
      return;
    }
    setState(() {
      if (!visible) {
        _leftClosingWidth = panelWidth;
      }
      _leftVisible = visible;
    });
    _animateSidebar(_leftSidebarAnimation, visible);
  }

  void _setRightVisible(bool visible, double panelWidth) {
    if (_rightVisible == visible) {
      return;
    }
    setState(() {
      if (!visible) {
        _rightClosingWidth = panelWidth;
      }
      _rightVisible = visible;
    });
    _animateSidebar(_rightSidebarAnimation, visible);
  }

  void _animateSidebar(AnimationController controller, bool visible) {
    final target = visible ? 1.0 : 0.0;
    if (MediaQuery.disableAnimationsOf(context)) {
      controller.value = target;
      return;
    }
    unawaited(controller.animateTo(target, curve: Curves.easeOutCubic));
  }
}

/// Clips a desktop sidebar toward its anchored window edge while it animates.
class _AnimatedSideRegion extends StatelessWidget {
  const _AnimatedSideRegion({
    required this.animation,
    required this.alignment,
    required this.child,
  });

  final Animation<double> animation;
  final Alignment alignment;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: animation,
      builder: (context, child) {
        final factor = animation.value;
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
