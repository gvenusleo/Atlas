import 'dart:async';
import 'dart:io';
import 'dart:math' as math;

import 'package:flutter/scheduler.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/features/connections/domain/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/details_panel.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/sessions_panel.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/workspace_panel.dart';
import 'package:atlas_flutter/l10n/localizations.dart';
import 'package:atlas_flutter/shared/layout/atlas_layout_metrics.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:atlas_flutter/shared/widgets/window_controls.dart';

/// Responsive Atlas workspace with desktop side panels and compact drawers.
class const WorkspaceShell({
  super.key,

  /// Shared runtime services, absent when bootstrap failed or in shell tests.
  final RuntimeEnvironment? environment,

  /// Configuration error shown in the empty conversation state.
  final String? startupError,
}) extends ConsumerStatefulWidget {
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
  double _leftWidth = AtlasLayoutMetrics.leftDefaultWidth;
  double _rightWidth = AtlasLayoutMetrics.rightDefaultWidth;
  double _leftClosingWidth = AtlasLayoutMetrics.leftDefaultWidth;
  double _rightClosingWidth = AtlasLayoutMetrics.rightDefaultWidth;

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
      duration: AtlasLayoutMetrics.sidebarAnimationDuration,
      vsync: this,
    );
    _rightSidebarAnimation = AnimationController(
      value: 1,
      duration: AtlasLayoutMetrics.sidebarAnimationDuration,
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
        if (constraints.maxWidth >= AtlasLayoutMetrics.desktopBreakpoint) {
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
    final closedLeftButtonX = AtlasLayoutMetrics.showsTrafficLights
        ? AtlasLayoutMetrics.macOSTrafficLightInset
        : 6.0;

    return Scaffold(
      body: AtlasResizeRing(
        child: SafeArea(
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
                          AtlasResizeHandle(
                            key: const ValueKey('atlas-left-resize-handle'),
                            panelOnLeft: true,
                            onDrag: (delta) =>
                                _resizeLeft(delta, availableWidth),
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
                          AtlasResizeHandle(
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
                child: AtlasToolbarButton(
                  key: const ValueKey('atlas-left-toggle'),
                  icon: _leftVisible
                      ? LucideIcons.panelLeft
                      : LucideIcons.panelLeftOpen,
                  tooltip: _leftVisible
                      ? context.l10n.hideSessions
                      : context.l10n.showSessions,
                  onPressed: () =>
                      _setLeftVisible(!_leftVisible, leftPanelWidth),
                ),
              ),
              Positioned(
                key: const ValueKey('atlas-right-toggle-positioned'),
                top: 6,
                right: 6,
                child: AtlasToolbarButton(
                  key: const ValueKey('atlas-right-toggle'),
                  icon: _rightVisible
                      ? LucideIcons.panelRight
                      : LucideIcons.panelRightOpen,
                  tooltip: _rightVisible
                      ? context.l10n.hideDetails
                      : context.l10n.showDetails,
                  onPressed: () =>
                      _setRightVisible(!_rightVisible, rightPanelWidth),
                ),
              ),
              if (usesCaptionControls(
                desktop: Platform.environment['XDG_CURRENT_DESKTOP'] ?? '',
                sessionType: Platform.environment['XDG_SESSION_TYPE'] ?? '',
              ))
                const Positioned(
                  key: ValueKey('atlas-window-controls'),
                  top: 6,
                  right: 6,
                  child: AtlasWindowControls(),
                ),
            ],
          ),
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
      body: AtlasResizeRing(
        child: SafeArea(
          child: WorkspacePanel(
            startupError: widget.startupError,
            compact: true,
            leftActive: false,
            onLeftPressed: () => _scaffoldKey.currentState?.openDrawer(),
            onRightPressed: () => _scaffoldKey.currentState?.openEndDrawer(),
          ),
        ),
      ),
    );
  }

  _PanelWidths _resolvePanelWidths(double availableWidth) {
    final visiblePanels = (_leftVisible ? 1 : 0) + (_rightVisible ? 1 : 0);
    final sideBudget = math.max(
      availableWidth -
          AtlasLayoutMetrics.centerMinimumWidth -
          visiblePanels * AtlasLayoutMetrics.resizeHandleWidth,
      0,
    );
    if (visiblePanels == 0) {
      return const _PanelWidths();
    }

    var left = _leftVisible
        ? _leftWidth.clamp(
            AtlasLayoutMetrics.leftMinimumWidth,
            AtlasLayoutMetrics.leftMaximumWidth,
          )
        : 0.0;
    var right = _rightVisible
        ? _rightWidth.clamp(
            AtlasLayoutMetrics.rightMinimumWidth,
            AtlasLayoutMetrics.rightMaximumWidth,
          )
        : 0.0;
    if (left + right <= sideBudget) {
      return _PanelWidths(left: left, right: right);
    }

    final minimumTotal =
        (_leftVisible ? AtlasLayoutMetrics.leftMinimumWidth : 0.0) +
        (_rightVisible ? AtlasLayoutMetrics.rightMinimumWidth : 0.0);
    final distributable = math.max(sideBudget - minimumTotal, 0.0);
    final leftFlex = _leftVisible
        ? left - AtlasLayoutMetrics.leftMinimumWidth
        : 0.0;
    final rightFlex = _rightVisible
        ? right - AtlasLayoutMetrics.rightMinimumWidth
        : 0.0;
    final totalFlex = leftFlex + rightFlex;
    if (totalFlex > 0) {
      left = _leftVisible
          ? AtlasLayoutMetrics.leftMinimumWidth +
                distributable * leftFlex / totalFlex
          : 0.0;
      right = _rightVisible
          ? AtlasLayoutMetrics.rightMinimumWidth +
                distributable * rightFlex / totalFlex
          : 0.0;
    } else {
      left = _leftVisible ? AtlasLayoutMetrics.leftMinimumWidth : 0.0;
      right = _rightVisible ? AtlasLayoutMetrics.rightMinimumWidth : 0.0;
    }
    return _PanelWidths(left: left, right: right);
  }

  void _resizeLeft(double delta, double availableWidth) {
    final widths = _resolvePanelWidths(availableWidth);
    final right = _rightVisible ? widths.right : 0.0;
    final dividers = (_leftVisible ? 1 : 0) + (_rightVisible ? 1 : 0);
    final maximum = math.min(
      AtlasLayoutMetrics.leftMaximumWidth,
      availableWidth -
          AtlasLayoutMetrics.centerMinimumWidth -
          right -
          dividers * AtlasLayoutMetrics.resizeHandleWidth,
    );
    setState(() {
      _leftWidth = (widths.left + delta).clamp(
        AtlasLayoutMetrics.leftMinimumWidth,
        maximum,
      );
    });
  }

  void _resizeRight(double delta, double availableWidth) {
    final widths = _resolvePanelWidths(availableWidth);
    final left = _leftVisible ? widths.left : 0.0;
    final dividers = (_leftVisible ? 1 : 0) + (_rightVisible ? 1 : 0);
    final maximum = math.min(
      AtlasLayoutMetrics.rightMaximumWidth,
      availableWidth -
          AtlasLayoutMetrics.centerMinimumWidth -
          left -
          dividers * AtlasLayoutMetrics.resizeHandleWidth,
    );
    setState(() {
      _rightWidth = (widths.right - delta).clamp(
        AtlasLayoutMetrics.rightMinimumWidth,
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
class const _AnimatedSideRegion({
  required final Animation<double> animation,
  required final Alignment alignment,
  required final Widget child,
}) extends StatelessWidget {
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

class const _PanelWidths({final double left = 0, final double right = 0}) {}
