import 'package:material_ui/material_ui.dart';

import '../../../../../app/locale_mode.dart';
import '../../../../../l10n/localizations.dart';
import 'settings_controls.dart';

/// A compact selector that stays usable in narrow settings panes.
class const LanguageSelector({
  super.key,
  required this.language,
  required this.onChanged,
}) extends StatelessWidget {
  final AppLanguage language;
  final ValueChanged<AppLanguage> onChanged;

  @override
  Widget build(BuildContext context) {
    return SettingsSegmentedButton<AppLanguage>(
      key: const ValueKey('atlas-language-selector'),
      segments: [
        ButtonSegment(
          value: AppLanguage.system,
          label: Text(context.l10n.system),
          tooltip: context.l10n.systemLanguageTooltip,
        ),
        ButtonSegment(
          value: AppLanguage.english,
          label: Text(context.l10n.english),
        ),
        ButtonSegment(
          value: AppLanguage.simplifiedChinese,
          label: Text(context.l10n.simplifiedChinese),
        ),
      ],
      selected: language,
      onChanged: onChanged,
    );
  }
}
