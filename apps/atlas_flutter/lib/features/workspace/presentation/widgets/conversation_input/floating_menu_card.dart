import 'dart:math' as math;

import 'package:material_ui/material_ui.dart';

import '../../../../../shared/theme/atlas_theme.dart';

/// Maximum rows shown in a floating picker; the window follows the highlight
/// so long catalogs stay within the viewport.
const maxPickerRows = 8;

/// Floating picker card with a gliding highlight driven by hover or keys.
class FloatingMenuCard<T> extends StatelessWidget {
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

  /// Maximum rows rendered at once; the window follows the highlight.
  final int? maxVisibleRows;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final rows = maxVisibleRows == null
        ? items.length
        : math.min(maxVisibleRows!, items.length);
    final windowStart = maxVisibleRows == null
        ? 0
        : popupWindowStart(highlighted, items.length, maxVisibleRows!);
    final visible = maxVisibleRows == null
        ? items
        : items.sublist(windowStart, windowStart + rows);
    return Material(
      color: Colors.transparent,
      child: Container(
        padding: EdgeInsets.all(cardPadding),
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
        child: Stack(
          children: [
            AnimatedPositioned(
              duration: const Duration(milliseconds: 200),
              curve: Curves.easeOutCubic,
              left: 0,
              right: 0,
              top: (highlighted - windowStart) * rowHeight,
              height: rowHeight,
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
                for (final (index, item) in visible.indexed)
                  InkWell(
                    onHover: (hovered) => onHighlighted(
                      hovered ? windowStart + index : selectedIndex,
                    ),
                    onTap: () => onSelected(item),
                    child: Container(
                      height: rowHeight,
                      padding: const EdgeInsets.symmetric(horizontal: 8),
                      child: itemBuilder(context, item, windowStart + index),
                    ),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

/// First visible row of a scrolling popup window that keeps the highlight
/// centered, mirroring the TUI slash popup.
int popupWindowStart(int selected, int count, int maxRows) {
  if (count <= maxRows) {
    return 0;
  }
  final top = selected - (maxRows - 1) ~/ 2;
  return top.clamp(0, count - maxRows);
}
