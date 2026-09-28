import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import 'workspace_controls.dart';

/// A choice in a settings dropdown.
class const SettingsDropdownOption<T>({
  required this.value,
  required this.label,
  this.tooltip,
}) {
  final T value;
  final String label;
  final String? tooltip;
}

/// Compact dropdown shared by settings rows.
class const SettingsDropdown<T>({
  super.key,
  required this.options,
  required this.selected,
  required this.onChanged,
}) extends StatelessWidget {
  static const _width = 144.0;
  static const _menuPadding = 8.0;

  final List<SettingsDropdownOption<T>> options;
  final T selected;
  final ValueChanged<T> onChanged;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final selectedLabel = options
        .firstWhere((option) => option.value == selected)
        .label;
    return MenuAnchor(
      style: const MenuStyle(
        alignment: AlignmentDirectional.bottomStart,
        fixedSize: WidgetStatePropertyAll(Size.fromWidth(_width)),
      ),
      menuChildren: [
        for (final option in options)
          SizedBox(
            width: _width - 2 * _menuPadding,
            child: Tooltip(
              message: option.tooltip ?? option.label,
              child: WorkspaceHoverSurface(
                child: MenuItemButton(
                  style: const ButtonStyle(
                    overlayColor: WidgetStatePropertyAll(Colors.transparent),
                    padding: WidgetStatePropertyAll(
                      EdgeInsets.symmetric(horizontal: 8, vertical: 16),
                    ),
                  ),
                  onPressed: () => onChanged(option.value),
                  child: Row(
                    children: [
                      Expanded(
                        child: Text(
                          option.label,
                          style: TextStyle(
                            color: colors.textPrimary,
                            fontSize: 12.5,
                          ),
                        ),
                      ),
                      if (option.value == selected)
                        Icon(
                          LucideIcons.check,
                          size: 14,
                          color: colors.textPrimary,
                        ),
                    ],
                  ),
                ),
              ),
            ),
          ),
      ],
      builder: (context, controller, child) => SizedBox(
        width: _width,
        height: 32,
        child: OutlinedButton(
          onPressed: () =>
              controller.isOpen ? controller.close() : controller.open(),
          style: ButtonStyle(
            padding: const WidgetStatePropertyAll(
              EdgeInsets.symmetric(horizontal: 10),
            ),
            backgroundColor: WidgetStatePropertyAll(colors.canvas),
            overlayColor: WidgetStatePropertyAll(colors.raised),
            foregroundColor: WidgetStatePropertyAll(colors.textPrimary),
            side: WidgetStatePropertyAll(BorderSide(color: colors.divider)),
            shape: WidgetStatePropertyAll(
              RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(AtlasRadii.control),
              ),
            ),
            textStyle: const WidgetStatePropertyAll(TextStyle(fontSize: 12.5)),
            tapTargetSize: MaterialTapTargetSize.shrinkWrap,
          ),
          child: Row(
            children: [
              Expanded(
                child: Text(selectedLabel, overflow: TextOverflow.ellipsis),
              ),
              const SizedBox(width: 8),
              Icon(
                LucideIcons.chevronDown,
                size: 14,
                color: colors.textSecondary,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// A surface grouping settings with dividers that meet its border.
class const SettingsGroupCard({super.key, required this.child})
    extends StatelessWidget {
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(vertical: 4),
      decoration: BoxDecoration(
        color: colors.panel,
        border: Border.all(color: colors.divider),
        borderRadius: BorderRadius.circular(AtlasRadii.surface),
      ),
      child: child,
    );
  }
}

/// Section title with the one-line summary that introduces its rows.
class const SettingsSectionHeader({
  super.key,

  /// Section title, for example "Appearance".
  required final String title,

  /// One-line summary of what the section controls.
  required final String description,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: TextStyle(
              color: colors.textPrimary,
              fontSize: 16,
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 3),
          Text(
            description,
            style: TextStyle(
              color: colors.textSecondary,
              fontSize: 12,
              height: 1.5,
            ),
          ),
        ],
      ),
    );
  }
}
