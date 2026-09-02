import 'dart:math' as math;

import 'package:flutter/gestures.dart';

import 'package:material_ui/material_ui.dart';

import '../../../../../shared/theme/atlas_theme.dart';

/// Maximum rows shown in a floating picker before its list scrolls.
const maxPickerRows = 8;

/// Floating picker card with a gliding highlight driven by hover or keys.
///
/// Rows live in a real scrollable, so mouse wheels scroll natively. macOS
/// trackpad two-finger scroll arrives as a PanZoom event stream that the
/// framework's `Scrollable` does not consume (verified on 3.47), so the card
/// handles it explicitly with drag semantics (content follows fingers).
/// Keyboard-driven navigation scrolls a centered window that follows the
/// highlight; hovering never scrolls.
class FloatingMenuCard<T> extends StatefulWidget {
  /// Creates a floating picker card.
  const FloatingMenuCard({
    super.key,
    required this.items,
    required this.selectedIndex,
    required this.highlighted,
    required this.onHighlighted,
    required this.onSelected,
    required this.rowHeight,
    required this.cardPadding,
    required this.itemBuilder,
    this.maxVisibleRows,
  });

  final List<T> items;
  final int selectedIndex;
  final int highlighted;
  final ValueChanged<int> onHighlighted;
  final ValueChanged<T> onSelected;
  final double rowHeight;
  final double cardPadding;
  final Widget Function(BuildContext context, T item, int index) itemBuilder;

  /// Maximum rows visible at once; the list scrolls beyond this.
  final int? maxVisibleRows;

  @override
  State<FloatingMenuCard<T>> createState() => _FloatingMenuCardState<T>();
}

class _FloatingMenuCardState<T> extends State<FloatingMenuCard<T>> {
  late final ScrollController _controller;

  /// Set while hover is driving the highlight so the resulting rebuild does
  /// not scroll: the pointer is already on the row it selected, and scrolling
  /// rows under a resting pointer would recreate a feedback loop.
  bool _highlightFromHover = false;

  int get _rowCount => widget.maxVisibleRows == null
      ? widget.items.length
      : math.min(widget.maxVisibleRows!, widget.items.length);

  double get _viewportHeight => _rowCount * widget.rowHeight;

  /// First visible row of the popup window that keeps [index] centered,
  /// mirroring the TUI slash popup; clamped so the window stays in range.
  int _windowStartFor(int index) {
    final count = widget.items.length;
    if (count <= _rowCount) {
      return 0;
    }
    final top = index - (_rowCount - 1) ~/ 2;
    return top.clamp(0, count - _rowCount);
  }

  @override
  void initState() {
    super.initState();
    _controller = ScrollController(
      initialScrollOffset:
          _windowStartFor(widget.highlighted) * widget.rowHeight,
    );
  }

  @override
  void didUpdateWidget(covariant FloatingMenuCard<T> oldWidget) {
    super.didUpdateWidget(oldWidget);
    final fromHover = _highlightFromHover;
    _highlightFromHover = false;
    if (fromHover ||
        widget.highlighted == oldWidget.highlighted ||
        !_controller.hasClients) {
      return;
    }
    // Keyboard or programmatic navigation: follow the highlight with a
    // centered window, matching the previous popup-window behavior.
    final offset = _windowStartFor(widget.highlighted) * widget.rowHeight;
    if (_controller.offset != offset) {
      _controller.jumpTo(offset);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _handleRowHover(bool hovered, int index) {
    if (!hovered) {
      return;
    }
    _highlightFromHover = true;
    widget.onHighlighted(index);
  }

  /// macOS trackpad two-finger scroll arrives as a PanZoom stream that
  /// `Scrollable` ignores (verified on 3.47); consume it with drag semantics
  /// — the offset moves opposite the finger movement so content follows
  /// fingers (natural scrolling). No End handling: no fling momentum.
  void _handlePanZoomUpdate(PointerPanZoomUpdateEvent event) {
    if (!_controller.hasClients) {
      return;
    }
    final target = (_controller.offset - event.panDelta.dy).clamp(
      0.0,
      _controller.position.maxScrollExtent,
    );
    _controller.jumpTo(target);
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Material(
      color: Colors.transparent,
      child: Container(
        padding: EdgeInsets.all(widget.cardPadding),
        decoration: BoxDecoration(
          color: colors.canvas,
          borderRadius: BorderRadius.circular(AtlasRadii.surface),
          border: Border.all(color: colors.divider),
          boxShadow: [
            BoxShadow(
              color: colors.scrim.withValues(alpha: 0.08),
              blurRadius: 4,
              offset: const Offset(0, 2),
            ),
          ],
        ),
        child: SizedBox(
          height: _viewportHeight,
          child: Listener(
            onPointerPanZoomUpdate: _handlePanZoomUpdate,
            child: SingleChildScrollView(
              controller: _controller,
              child: Stack(
                children: [
                  AnimatedPositioned(
                    duration: const Duration(milliseconds: 200),
                    curve: Curves.easeOutCubic,
                    left: 0,
                    right: 0,
                    top: widget.highlighted * widget.rowHeight,
                    height: widget.rowHeight,
                    child: Container(
                      decoration: BoxDecoration(
                        color: colors.raised,
                        borderRadius: BorderRadius.circular(AtlasRadii.control),
                      ),
                    ),
                  ),
                  Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      for (final (index, item) in widget.items.indexed)
                        InkWell(
                          onHover: (hovered) => _handleRowHover(hovered, index),
                          onTap: () => widget.onSelected(item),
                          child: Container(
                            height: widget.rowHeight,
                            padding: const EdgeInsets.symmetric(horizontal: 8),
                            child: widget.itemBuilder(context, item, index),
                          ),
                        ),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
