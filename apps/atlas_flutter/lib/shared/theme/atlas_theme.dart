import 'package:material_ui/material_ui.dart';

import 'atlas_palette.dart';
export 'atlas_palette.dart';

/// Radius scale for controls in the otherwise flat application shell.
abstract final class AtlasRadii {
  static const small = 4.0;
  static const control = 6.0;
  static const surface = 10.0;
}

/// Builds the Atlas visual theme for [palette] at [brightness].
ThemeData buildAtlasTheme(AtlasPalette palette, Brightness brightness) {
  final colors = palette.colors(brightness);
  final colorScheme = ColorScheme(
    brightness: brightness,
    primary: colors.accent,
    onPrimary: colors.onAccent,
    secondary: colors.success,
    onSecondary: colors.onAccent,
    error: colors.error,
    onError: colors.onAccent,
    surface: colors.canvas,
    onSurface: colors.textPrimary,
    onSurfaceVariant: colors.textSecondary,
    outline: colors.divider,
    outlineVariant: colors.divider,
  );
  final base = ThemeData(brightness: brightness, useMaterial3: true);

  return base.copyWith(
    colorScheme: colorScheme,
    extensions: [colors],
    scaffoldBackgroundColor: colors.canvas,
    canvasColor: colors.canvas,
    dividerColor: colors.divider,
    splashFactory: NoSplash.splashFactory,
    splashColor: Colors.transparent,
    highlightColor: Colors.transparent,
    hoverColor: colors.raised,
    focusColor: colors.accent,
    textTheme: base.textTheme
        .apply(bodyColor: colors.textPrimary, displayColor: colors.textPrimary)
        .copyWith(
          bodyMedium: TextStyle(color: colors.textPrimary, fontSize: 13),
          bodySmall: TextStyle(color: colors.textSecondary, fontSize: 12),
          labelMedium: TextStyle(color: colors.textPrimary, fontSize: 12.5),
          titleMedium: TextStyle(
            color: colors.textPrimary,
            fontSize: 15,
            fontWeight: FontWeight.w600,
          ),
        ),
    iconTheme: IconThemeData(color: colors.textSecondary, size: 18),
    dividerTheme: DividerThemeData(
      color: colors.divider,
      thickness: 1,
      space: 1,
    ),
    drawerTheme: DrawerThemeData(
      backgroundColor: colors.panel,
      elevation: 0,
      scrimColor: colors.scrim,
      shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
      endShape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
    ),
    tooltipTheme: TooltipThemeData(
      waitDuration: const Duration(milliseconds: 450),
      decoration: BoxDecoration(
        color: colors.overlay,
        borderRadius: BorderRadius.circular(AtlasRadii.small),
      ),
      textStyle: TextStyle(
        color: colors.textPrimary,
        fontSize: 12,
        height: 1.2,
      ),
    ),
    menuTheme: MenuThemeData(
      style: MenuStyle(
        backgroundColor: WidgetStatePropertyAll(colors.overlay),
        elevation: WidgetStatePropertyAll(1),
        surfaceTintColor: WidgetStatePropertyAll(Colors.transparent),
        shape: WidgetStatePropertyAll(
          RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
          ),
        ),
        padding: WidgetStatePropertyAll(const EdgeInsets.all(8)),
      ),
    ),
    menuButtonTheme: MenuButtonThemeData(
      style: ButtonStyle(
        padding: WidgetStatePropertyAll(
          const EdgeInsets.fromLTRB(8, 16, 24, 16),
        ),
        shape: WidgetStatePropertyAll(
          RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(AtlasRadii.small),
          ),
        ),
        minimumSize: WidgetStatePropertyAll(const Size(0, 30)),
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
        textStyle: WidgetStatePropertyAll(
          TextStyle(color: colors.textPrimary, fontSize: 12.5),
        ),
        foregroundColor: WidgetStatePropertyAll(colors.textPrimary),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: ButtonStyle(
        shape: WidgetStatePropertyAll(
          RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
          ),
        ),
        foregroundColor: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.disabled)) {
            return colors.textSecondary.withValues(alpha: 0.5);
          }
          return colors.textPrimary;
        }),
        overlayColor: WidgetStatePropertyAll(colors.raised),
        padding: WidgetStatePropertyAll(
          const EdgeInsets.symmetric(horizontal: 12, vertical: 16),
        ),
        minimumSize: WidgetStatePropertyAll(const Size(0, 30)),
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
        textStyle: WidgetStatePropertyAll(
          TextStyle(color: colors.textPrimary, fontSize: 12.5),
        ),
      ),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: colors.overlay,
      elevation: 1,
      shadowColor: colors.scrim,
      barrierColor: colors.textSecondary.withAlpha(100),
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AtlasRadii.surface),
      ),
      constraints: const BoxConstraints(minWidth: 320, maxWidth: 400),
      titleTextStyle: TextStyle(
        color: colors.textPrimary,
        fontSize: 15,
        fontWeight: FontWeight.w600,
      ),
      contentTextStyle: TextStyle(
        color: colors.textPrimary,
        fontSize: 13,
        height: 1.4,
      ),
      actionsPadding: const EdgeInsets.fromLTRB(24, 0, 24, 18),
    ),
    inputDecorationTheme: InputDecorationTheme(
      contentPadding: const EdgeInsets.all(0),
      border: UnderlineInputBorder(
        borderSide: BorderSide(color: colors.textPrimary, width: 1),
      ),
      enabledBorder: UnderlineInputBorder(
        borderSide: BorderSide(color: colors.textPrimary, width: 1),
      ),
      focusedBorder: UnderlineInputBorder(
        borderSide: BorderSide(color: colors.textPrimary, width: 1),
      ),
    ),
  );
}
