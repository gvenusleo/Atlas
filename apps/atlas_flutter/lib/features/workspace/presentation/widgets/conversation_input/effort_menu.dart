import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../../shared/theme/atlas_theme.dart';
import '../workspace_controls.dart';
import 'floating_menu_card.dart';

/// Toolbar trigger that opens the reasoning-effort picker.
class const EffortMenu({
  super.key,
  required final List<ReasoningEffortOption> efforts,
  required final String? value,
  required final VoidCallback onTap,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? (efforts.isEmpty ? '' : efforts.first.value);
    final label = efforts
        .where((effort) => effort.value == current)
        .map((effort) => effort.name.isEmpty ? effort.value : effort.name)
        .firstOrNull;
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: TextButton(
        style: ButtonStyle(
          padding: WidgetStatePropertyAll(
            const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
          ),
          overlayColor: const WidgetStatePropertyAll(Colors.transparent),
        ),
        onPressed: onTap,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Icon(LucideIcons.brain, size: 12, color: colors.textSecondary),
            const SizedBox(width: 6),
            Text(
              label ?? current,
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Floating reasoning-effort picker card with a gliding highlight.
class const EffortMenuCard({
  super.key,
  required final List<ReasoningEffortOption> efforts,
  required final String? value,
  required final int highlighted,
  required final ValueChanged<int> onHighlighted,
  required final ValueChanged<ReasoningEffortOption> onSelected,
}) extends StatelessWidget {
  /// Row height used to size and position the floating card.
  static const rowHeight = 30.0;

  /// Vertical padding around the row list, used to size the floating card.
  static const cardPadding = 8.0;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? (efforts.isEmpty ? '' : efforts.first.value);
    final selectedIndex = efforts.indexWhere(
      (effort) => effort.value == current,
    );
    return FloatingMenuCard(
      items: efforts,
      selectedIndex: selectedIndex,
      highlighted: highlighted,
      onHighlighted: onHighlighted,
      onSelected: onSelected,
      rowHeight: rowHeight,
      cardPadding: cardPadding,
      maxVisibleRows: maxPickerRows,
      itemBuilder: (context, effort, index) => Row(
        children: [
          Expanded(
            child: Text(
              effort.name.isEmpty ? effort.value : effort.name,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: colors.textPrimary,
                fontSize: 12.5,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
          if (effort.value == current)
            Icon(LucideIcons.check, size: 13, color: colors.textPrimary),
        ],
      ),
    );
  }
}
