import 'package:flutter/widgets.dart';

import 'app_localizations.dart';
import 'app_localizations_en.dart';

/// Looks up the current UI language. Standalone widget tests without an app
/// localization delegate keep their previous English behavior.
extension AtlasLocalizations on BuildContext {
  AppLocalizations get l10n =>
      Localizations.of<AppLocalizations>(this, AppLocalizations) ??
      AppLocalizationsEn();

  /// Translates errors produced by Atlas itself while preserving unknown OS,
  /// runtime, and server details verbatim.
  String localizeAtlasError(String message) {
    final known = switch (message) {
      'File is larger than the 512 KB preview limit.' => l10n.fileTooLarge,
      'Binary files cannot be previewed.' => l10n.binaryFileCannotPreview,
      'An item with that name already exists.' => l10n.itemAlreadyExists,
      'Cannot move a folder into itself.' => l10n.cannotMoveFolderIntoItself,
      'Enter a valid name.' => l10n.enterValidName,
      'Could not find a free name.' => l10n.couldNotFindFreeName,
      'That path is outside the workspace.' => l10n.pathOutsideWorkspace,
      'Could not move the item to Trash.' => l10n.couldNotMoveToTrash,
      'Could not reveal the item.' => l10n.couldNotRevealItem,
      'This file no longer exists.' => l10n.fileNoLongerExists,
      'Cannot locate the home directory for Atlas configuration.' =>
        l10n.cannotLocateHome,
      _ => null,
    };
    if (known != null) return known;

    const atlasPrefix = 'Cannot start Atlas: ';
    if (message.startsWith(atlasPrefix)) {
      return l10n.cannotStartAtlas(message.substring(atlasPrefix.length));
    }
    const acpPrefix = 'Cannot start ACP server: ';
    if (message.startsWith(acpPrefix)) {
      return l10n.cannotStartAcpServer(message.substring(acpPrefix.length));
    }
    const loadPrefix = 'Cannot load ';
    final separator = message.indexOf(': ', loadPrefix.length);
    if (message.startsWith(loadPrefix) && separator >= 0) {
      return l10n.cannotLoadConfiguration(
        message.substring(loadPrefix.length, separator),
        message.substring(separator + 2),
      );
    }
    return message;
  }
}
