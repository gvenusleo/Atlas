import 'package:flutter/foundation.dart';

/// Shared geometry and platform rules for the workspace shell.
abstract final class WorkspaceMetrics {
  /// Width at which the desktop three-panel layout becomes available.
  static const desktopBreakpoint = 960.0;

  /// Minimum width preserved for the central workspace.
  static const centerMinimumWidth = 420.0;

  /// Initial width of the sessions sidebar.
  static const leftDefaultWidth = 224.0;

  /// Minimum width of the sessions sidebar.
  static const leftMinimumWidth = 184.0;

  /// Maximum width of the sessions sidebar.
  static const leftMaximumWidth = 360.0;

  /// Initial width of the details sidebar.
  static const rightDefaultWidth = 260.0;

  /// Minimum width of the details sidebar.
  static const rightMinimumWidth = 220.0;

  /// Maximum width of the details sidebar.
  static const rightMaximumWidth = 380.0;

  /// Width of the interactive resize gutter.
  static const resizeHandleWidth = 8.0;

  /// Toolbar height used by desktop panels.
  static const desktopToolbarHeight = 44.0;

  /// Toolbar height used by compact layouts.
  static const compactToolbarHeight = 48.0;

  /// Horizontal inset reserved for macOS traffic-light controls.
  static const macOSTrafficLightInset = 76.0;

  /// Duration of the desktop sidebar reveal animation.
  static const sidebarAnimationDuration = Duration(milliseconds: 180);

  /// Whether the platform uses the integrated Atlas titlebar.
  static bool get usesIntegratedTitlebar =>
      !kIsWeb && defaultTargetPlatform == TargetPlatform.macOS;

  /// Whether the platform uses touch-first navigation with drawers.
  static bool get usesCompactNavigation =>
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.android ||
          defaultTargetPlatform == TargetPlatform.iOS);
}
