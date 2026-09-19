import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';

/// Section title with the one-line summary that introduces its rows.
///
/// Used by every settings section so the pane reads as one surface regardless
/// of which section the rail has selected.
class SettingsSectionHeader extends StatelessWidget {
  /// Creates a settings section header.
  const SettingsSectionHeader({
    super.key,
    required this.title,
    required this.description,
  });

  /// Section title, for example "Appearance".
  final String title;

  /// One-line summary of what the section controls.
  final String description;

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
              fontSize: 13,
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
