import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';

/// Compact segmented control shared by settings rows.
class const SettingsSegmentedButton<T>({
  super.key,
  required this.segments,
  required this.selected,
  required this.onChanged,
}) extends StatelessWidget {
  final List<ButtonSegment<T>> segments;
  final T selected;
  final ValueChanged<T> onChanged;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return SegmentedButton<T>(
      segments: segments,
      selected: {selected},
      showSelectedIcon: false,
      onSelectionChanged: (selection) => onChanged(selection.first),
      style: ButtonStyle(
        minimumSize: const WidgetStatePropertyAll(Size(0, 26)),
        padding: const WidgetStatePropertyAll(
          EdgeInsets.symmetric(horizontal: 10),
        ),
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
        backgroundColor: WidgetStateProperty.resolveWith(
          (states) => states.contains(WidgetState.selected)
              ? colors.raised
              : Colors.transparent,
        ),
        foregroundColor: WidgetStateProperty.resolveWith(
          (states) => states.contains(WidgetState.selected)
              ? colors.textPrimary
              : colors.textSecondary,
        ),
        side: WidgetStateProperty.all(BorderSide(color: colors.divider)),
        shape: WidgetStateProperty.all(
          RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
          ),
        ),
        textStyle: WidgetStateProperty.resolveWith(
          (states) => states.contains(WidgetState.selected)
              ? const TextStyle(fontSize: 12, fontWeight: FontWeight.w500)
              : const TextStyle(fontSize: 12, fontWeight: FontWeight.w400),
        ),
        visualDensity: VisualDensity.compact,
      ),
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
