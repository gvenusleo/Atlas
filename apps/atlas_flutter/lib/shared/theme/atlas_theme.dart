import 'package:material_ui/material_ui.dart';

/// Semantic colors shared by the Atlas application shell.
@immutable
class AtlasColors extends ThemeExtension<AtlasColors> {
  const AtlasColors({
    required this.canvas,
    required this.panel,
    required this.raised,
    required this.divider,
    required this.textPrimary,
    required this.textSecondary,
    required this.accent,
    required this.success,
    required this.error,
    required this.scrim,
  });

  static const light = AtlasColors(
    canvas: Color(0xFFFCFCFC),
    panel: Color(0xFFECECED),
    raised: Color(0xFFDFE0E1),
    divider: Color(0xFFCFD1D2),
    textPrimary: Color(0xFF5C6166),
    textSecondary: Color(0xFF8B8E92),
    accent: Color(0xFF3B9EE5),
    success: Color(0xFF85B304),
    error: Color(0xFFEF7271),
    scrim: Color(0x525C6166),
  );

  static const dark = AtlasColors(
    canvas: Color(0xFF0D1016),
    panel: Color(0xFF1F2127),
    raised: Color(0xFF2D2F34),
    divider: Color(0xFF3F4043),
    textPrimary: Color(0xFFBFBDB6),
    textSecondary: Color(0xFF8A8986),
    accent: Color(0xFF5AC1FE),
    success: Color(0xFFAAD84C),
    error: Color(0xFFEF7177),
    scrim: Color(0x66BFBDB6),
  );

  final Color canvas;
  final Color panel;
  final Color raised;
  final Color divider;
  final Color textPrimary;
  final Color textSecondary;
  final Color accent;
  final Color success;
  final Color error;
  final Color scrim;

  /// Returns the Atlas palette for [brightness].
  static AtlasColors forBrightness(Brightness brightness) {
    return brightness == Brightness.light ? light : dark;
  }

  /// Returns the active Atlas palette from the nearest theme.
  static AtlasColors of(BuildContext context) {
    final theme = Theme.of(context);
    return theme.extension<AtlasColors>() ??
        AtlasColors.forBrightness(theme.brightness);
  }

  @override
  AtlasColors copyWith({
    Color? canvas,
    Color? panel,
    Color? raised,
    Color? divider,
    Color? textPrimary,
    Color? textSecondary,
    Color? accent,
    Color? success,
    Color? error,
    Color? scrim,
  }) {
    return AtlasColors(
      canvas: canvas ?? this.canvas,
      panel: panel ?? this.panel,
      raised: raised ?? this.raised,
      divider: divider ?? this.divider,
      textPrimary: textPrimary ?? this.textPrimary,
      textSecondary: textSecondary ?? this.textSecondary,
      accent: accent ?? this.accent,
      success: success ?? this.success,
      error: error ?? this.error,
      scrim: scrim ?? this.scrim,
    );
  }

  @override
  AtlasColors lerp(covariant AtlasColors? other, double t) {
    if (other == null) {
      return this;
    }
    return AtlasColors(
      canvas: Color.lerp(canvas, other.canvas, t)!,
      panel: Color.lerp(panel, other.panel, t)!,
      raised: Color.lerp(raised, other.raised, t)!,
      divider: Color.lerp(divider, other.divider, t)!,
      textPrimary: Color.lerp(textPrimary, other.textPrimary, t)!,
      textSecondary: Color.lerp(textSecondary, other.textSecondary, t)!,
      accent: Color.lerp(accent, other.accent, t)!,
      success: Color.lerp(success, other.success, t)!,
      error: Color.lerp(error, other.error, t)!,
      scrim: Color.lerp(scrim, other.scrim, t)!,
    );
  }
}

/// Radius scale for controls in the otherwise flat application shell.
abstract final class AtlasRadii {
  static const small = 4.0;
  static const control = 6.0;
  static const surface = 10.0;
}

/// Builds the Atlas visual theme for [brightness].
ThemeData buildAtlasTheme(Brightness brightness) {
  final colors = AtlasColors.forBrightness(brightness);
  final colorScheme = ColorScheme(
    brightness: brightness,
    primary: colors.accent,
    onPrimary: colors.canvas,
    secondary: colors.success,
    onSecondary: colors.canvas,
    error: colors.error,
    onError: colors.canvas,
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
        color: colors.raised,
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
        backgroundColor: WidgetStatePropertyAll(colors.canvas),
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
      backgroundColor: colors.canvas,
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
