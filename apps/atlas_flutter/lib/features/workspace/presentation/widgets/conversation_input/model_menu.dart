import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../../shared/theme/atlas_theme.dart';
import '../workspace_controls.dart';
import 'floating_menu_card.dart';

/// Toolbar trigger that opens the model picker.
class const ModelMenu({
  super.key,
  required final List<ModelDescriptor> models,
  required final ModelDescriptor activeModel,
  required final VoidCallback onTap,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
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
            Icon(LucideIcons.package, size: 12, color: colors.textSecondary),
            const SizedBox(width: 6),
            Text(
              activeModel.name.isEmpty
                  ? activeModel.ref.modelId.value
                  : activeModel.name,
              style: TextStyle(color: colors.textSecondary, fontSize: 12),
            ),
          ],
        ),
      ),
    );
  }
}

/// Floating model picker card with a gliding highlight.
class const ModelMenuCard({
  super.key,
  required final List<ModelDescriptor> models,
  required final ModelDescriptor activeModel,
  required final int highlighted,
  required final ValueChanged<int> onHighlighted,
  required final ValueChanged<ModelDescriptor> onSelected,
}) extends StatelessWidget {
  /// Row height used to size and position the floating card.
  static const rowHeight = 30.0;

  /// Vertical padding around the row list, used to size the floating card.
  static const cardPadding = 8.0;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final selectedIndex = models.indexWhere(
      (model) => model.ref == activeModel.ref,
    );
    return FloatingMenuCard(
      items: models,
      selectedIndex: selectedIndex,
      highlighted: highlighted,
      onHighlighted: onHighlighted,
      onSelected: onSelected,
      rowHeight: rowHeight,
      cardPadding: cardPadding,
      maxVisibleRows: maxPickerRows,
      itemBuilder: (context, model, index) => Row(
        children: [
          Expanded(
            child: Text(
              model.name.isEmpty ? model.ref.modelId.value : model.name,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: colors.textPrimary,
                fontSize: 12.5,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
          if (model.ref == activeModel.ref)
            Icon(LucideIcons.check, size: 13, color: colors.textPrimary),
        ],
      ),
    );
  }
}
