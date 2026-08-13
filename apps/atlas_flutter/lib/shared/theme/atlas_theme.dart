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
    canvas: Color(0xFFF8F8F9),
    panel: Color(0xFFF1F1F2),
    raised: Color(0xFFE8E9EC),
    divider: Color(0xFFD3D5DA),
    textPrimary: Color(0xFF1F2733),
    textSecondary: Color(0xFF626D7D),
    accent: Color(0xFF1E66F5),
    success: Color(0xFF237A47),
    error: Color(0xFFB93845),
    scrim: Color(0x520F172A),
  );

  static const dark = AtlasColors(
    canvas: Color(0xFF20232A),
    panel: Color(0xFF282C34),
    raised: Color(0xFF30353E),
    divider: Color(0xFF3A404B),
    textPrimary: Color(0xFFE6E9EF),
    textSecondary: Color(0xFF9AA2B1),
    accent: Color(0xFF89B4FA),
    success: Color(0xFF7AD98B),
    error: Color(0xFFFF8792),
    scrim: Color(0x66000000),
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
    return Theme.of(context).extension<AtlasColors>()!;
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
    textTheme: base.textTheme.apply(
      bodyColor: colors.textPrimary,
      displayColor: colors.textPrimary,
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
  );
}
