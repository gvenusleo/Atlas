import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/features/settings/domain/app_language.dart';
import 'package:atlas_flutter/features/settings/presentation/widgets/settings_controls.dart';
import 'package:atlas_flutter/l10n/localizations.dart';

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
    return SettingsDropdown<AppLanguage>(
      key: const ValueKey('atlas-language-selector'),
      options: [
        SettingsDropdownOption(
          value: AppLanguage.system,
          label: context.l10n.system,
          tooltip: context.l10n.systemLanguageTooltip,
        ),
        SettingsDropdownOption(
          value: AppLanguage.english,
          label: context.l10n.english,
        ),
        SettingsDropdownOption(
          value: AppLanguage.simplifiedChinese,
          label: context.l10n.simplifiedChinese,
        ),
      ],
      selected: language,
      onChanged: onChanged,
    );
  }
}
