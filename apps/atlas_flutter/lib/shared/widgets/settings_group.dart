import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/shared/theme/atlas_theme.dart';

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
