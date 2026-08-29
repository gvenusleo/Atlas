import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../../shared/theme/atlas_theme.dart';
import '../workspace_controls.dart';
import 'floating_menu_card.dart';

/// Toolbar trigger that opens the session-mode picker.
class ModeMenu extends StatelessWidget {
  /// Creates a session-mode picker trigger.
  const ModeMenu({
    super.key,
    required this.modes,
    required this.value,
    required this.onTap,
  });

  final List<ModeOption> modes;
  final String? value;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? (modes.isEmpty ? '' : modes.first.id);
    final label = modes
        .where((option) => option.id == current)
        .map((option) => option.name.isEmpty ? option.id : option.name)
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
            Icon(LucideIcons.layoutGrid, size: 12, color: colors.textSecondary),
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

/// Floating session-mode picker card with a gliding highlight.
class ModeMenuCard extends StatelessWidget {
  /// Creates a session-mode picker card.
  const ModeMenuCard({
    super.key,
    required this.modes,
    required this.value,
    required this.highlighted,
    required this.onHighlighted,
    required this.onSelected,
  });

  /// Row height used to size and position the floating card.
  static const rowHeight = 30.0;

  /// Vertical padding around the row list, used to size the floating card.
  static const cardPadding = 8.0;

  final List<ModeOption> modes;
  final String? value;
  final int highlighted;
  final ValueChanged<int> onHighlighted;
  final ValueChanged<ModeOption> onSelected;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final current = value ?? (modes.isEmpty ? '' : modes.first.id);
    final selectedIndex = modes.indexWhere((option) => option.id == current);
    return FloatingMenuCard(
      items: modes,
      selectedIndex: selectedIndex,
      highlighted: highlighted,
      onHighlighted: onHighlighted,
      onSelected: onSelected,
      rowHeight: rowHeight,
      cardPadding: cardPadding,
      maxVisibleRows: maxPickerRows,
      itemBuilder: (context, option, index) => Row(
        children: [
          Expanded(
            child: Text(
              option.name.isEmpty ? option.id : option.name,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: colors.textPrimary,
                fontSize: 12.5,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
          if (option.id == current)
            Icon(LucideIcons.check, size: 13, color: colors.textPrimary),
        ],
      ),
    );
  }
}
