import 'package:material_ui/material_ui.dart';

/// Semantic colors shared by the Atlas application shell.
///
/// Token names are the stable contract for feature code; each palette maps
/// them onto one upstream color role. Values live in [AtlasPalette], never
/// in widgets.
@immutable
class const AtlasColors({
  /// App background behind the content column.
  required final Color canvas,

  /// Side panels and docked surfaces layered onto the canvas.
  required final Color panel,

  /// Floating surfaces such as menus, dialogs, and tooltips.
  required final Color overlay,

  /// Hover fills and other transient surface tints (may be translucent).
  required final Color raised,

  /// Hairline separators and control borders.
  required final Color divider,

  /// Primary text and icons.
  required final Color textPrimary,

  /// Secondary text, placeholders, and inactive icons.
  required final Color textSecondary,

  /// Accent for links, focus rings, active states, and filled controls.
  required final Color accent,

  /// Text and icons placed on [accent] or status emphasis fills.
  required final Color onAccent,

  /// Success accent for connected or completed states.
  required final Color success,

  /// Warning accent for pending or degraded states.
  required final Color warning,

  /// Error accent for failures and destructive actions.
  required final Color error,

  /// Scrim behind drawers and modal surfaces.
  required final Color scrim,
}) extends ThemeExtension<AtlasColors> {
  /// Returns the active Atlas palette from the nearest theme.
  static AtlasColors of(BuildContext context) {
    final theme = Theme.of(context);
    return theme.extension<AtlasColors>() ??
        AtlasPalette.standard.colors(theme.brightness);
  }

  @override
  AtlasColors copyWith({
    Color? canvas,
    Color? panel,
    Color? overlay,
    Color? raised,
    Color? divider,
    Color? textPrimary,
    Color? textSecondary,
    Color? accent,
    Color? onAccent,
    Color? success,
    Color? warning,
    Color? error,
    Color? scrim,
  }) {
    return AtlasColors(
      canvas: canvas ?? this.canvas,
      panel: panel ?? this.panel,
      overlay: overlay ?? this.overlay,
      raised: raised ?? this.raised,
      divider: divider ?? this.divider,
      textPrimary: textPrimary ?? this.textPrimary,
      textSecondary: textSecondary ?? this.textSecondary,
      accent: accent ?? this.accent,
      onAccent: onAccent ?? this.onAccent,
      success: success ?? this.success,
      warning: warning ?? this.warning,
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
      overlay: Color.lerp(overlay, other.overlay, t)!,
      raised: Color.lerp(raised, other.raised, t)!,
      divider: Color.lerp(divider, other.divider, t)!,
      textPrimary: Color.lerp(textPrimary, other.textPrimary, t)!,
      textSecondary: Color.lerp(textSecondary, other.textSecondary, t)!,
      accent: Color.lerp(accent, other.accent, t)!,
      onAccent: Color.lerp(onAccent, other.onAccent, t)!,
      success: Color.lerp(success, other.success, t)!,
      warning: Color.lerp(warning, other.warning, t)!,
      error: Color.lerp(error, other.error, t)!,
      scrim: Color.lerp(scrim, other.scrim, t)!,
    );
  }
}

/// Terminal color roles for one palette brightness.
///
/// The sixteen ANSI slots use upstream terminal values; the remaining roles
/// follow the editor so the terminal reads as part of the shell.
@immutable
class const AtlasTerminalColors({
  required final Color background,
  required final Color foreground,
  required final Color cursor,
  required final Color selection,
  required final Color black,
  required final Color red,
  required final Color green,
  required final Color yellow,
  required final Color blue,
  required final Color magenta,
  required final Color cyan,
  required final Color white,
  required final Color brightBlack,
  required final Color brightRed,
  required final Color brightGreen,
  required final Color brightYellow,
  required final Color brightBlue,
  required final Color brightMagenta,
  required final Color brightCyan,
  required final Color brightWhite,

  /// Fill behind non-current search matches.
  required final Color searchHitBackground,

  /// Fill behind the active search match.
  required final Color searchHitBackgroundCurrent,

  /// Text color inside search matches.
  required final Color searchHitForeground,
}) {}

/// A named light/dark pair of Atlas colors that can be selected by users.
@immutable
class const AtlasPalette({
  /// Stable identifier used when persisting a user choice.
  required final String id,

  /// Human-readable name for a theme picker.
  required final String label,
  required final AtlasColors light,
  required final AtlasColors dark,
  required final AtlasTerminalColors terminalLight,
  required final AtlasTerminalColors terminalDark,
}) {
  /// Returns the UI colors for [brightness].
  AtlasColors colors(Brightness brightness) =>
      brightness == Brightness.light ? light : dark;

  /// Returns the terminal colors for [brightness].
  AtlasTerminalColors terminal(Brightness brightness) =>
      brightness == Brightness.light ? terminalLight : terminalDark;

  /// The palette used while no user choice exists.
  static const AtlasPalette standard = github;

  /// GitHub palette from [primer/github-vscode-theme](https://github.com/primer/github-vscode-theme)
  /// v6.3.5: light follows `github-light` and dark follows `github-dark-dimmed`.
  ///
  /// Both are generated from `@primer/primitives` 7.10.0 by `src/theme.js`,
  /// including the `src/colors.js` overrides for light (`fg.default`
  /// `#1f2328`, `fg.muted` `#656d76`). Terminal slots map `terminal.ansi*`;
  /// background/foreground follow the editor canvas, cursor and selection
  /// use `editorCursor.foreground` and `editor.selectionBackground`
  /// (accent at 20%), and search hits use the editor find-match colors.
  static const AtlasPalette github = AtlasPalette(
    id: 'github',
    label: 'GitHub',
    light: _githubLight,
    dark: _githubDarkDimmed,
    terminalLight: _githubLightTerminal,
    terminalDark: _githubDarkDimmedTerminal,
  );

  static const _githubLight = AtlasColors(
    canvas: Color(0xFFFFFFFF),
    panel: Color(0xFFF6F8FA),
    overlay: Color(0xFFFFFFFF),
    raised: Color(0x80EAEEF2),
    divider: Color(0xFFD0D7DE),
    textPrimary: Color(0xFF1F2328),
    textSecondary: Color(0xFF656D76),
    accent: Color(0xFF0969DA),
    onAccent: Color(0xFFFFFFFF),
    success: Color(0xFF1A7F37),
    warning: Color(0xFF9A6700),
    error: Color(0xFFCF222E),
    scrim: Color(0x521F2328),
  );

  static const _githubDarkDimmed = AtlasColors(
    canvas: Color(0xFF22272E),
    panel: Color(0xFF1C2128),
    overlay: Color(0xFF2D333B),
    raised: Color(0x1A636E7B),
    divider: Color(0xFF444C56),
    textPrimary: Color(0xFFADBAC7),
    textSecondary: Color(0xFF768390),
    accent: Color(0xFF539BF5),
    onAccent: Color(0xFFCDD9E5),
    success: Color(0xFF57AB5A),
    warning: Color(0xFFC69026),
    error: Color(0xFFE5534B),
    scrim: Color(0x66ADBAC7),
  );

  /// GitHub light ANSI palette from `@primer/primitives` 7.10.0.
  static const _githubLightTerminal = AtlasTerminalColors(
    background: Color(0xFFFFFFFF),
    foreground: Color(0xFF1F2328),
    cursor: Color(0xFF0969DA),
    selection: Color(0x330969DA),
    black: Color(0xFF24292F),
    red: Color(0xFFCF222E),
    green: Color(0xFF116329),
    yellow: Color(0xFF4D2D00),
    blue: Color(0xFF0969DA),
    magenta: Color(0xFF8250DF),
    cyan: Color(0xFF1B7C83),
    white: Color(0xFF6E7781),
    brightBlack: Color(0xFF57606A),
    brightRed: Color(0xFFA40E26),
    brightGreen: Color(0xFF1A7F37),
    brightYellow: Color(0xFF633C01),
    brightBlue: Color(0xFF218BFF),
    brightMagenta: Color(0xFFA475F9),
    brightCyan: Color(0xFF3192AA),
    brightWhite: Color(0xFF8C959F),
    searchHitBackground: Color(0x80FAE17D),
    searchHitBackgroundCurrent: Color(0xFFBF8700),
    searchHitForeground: Color(0xFF1F2328),
  );

  /// GitHub dark-dimmed ANSI palette from `@primer/primitives` 7.10.0.
  static const _githubDarkDimmedTerminal = AtlasTerminalColors(
    background: Color(0xFF22272E),
    foreground: Color(0xFFADBAC7),
    cursor: Color(0xFF539BF5),
    selection: Color(0x33539BF5),
    black: Color(0xFF545D68),
    red: Color(0xFFF47067),
    green: Color(0xFF57AB5A),
    yellow: Color(0xFFC69026),
    blue: Color(0xFF539BF5),
    magenta: Color(0xFFB083F0),
    cyan: Color(0xFF39C5CF),
    white: Color(0xFF909DAB),
    brightBlack: Color(0xFF636E7B),
    brightRed: Color(0xFFFF938A),
    brightGreen: Color(0xFF6BC46D),
    brightYellow: Color(0xFFDAAA3F),
    brightBlue: Color(0xFF6CB6FF),
    brightMagenta: Color(0xFFDCBDFB),
    brightCyan: Color(0xFF56D4DD),
    brightWhite: Color(0xFFCDD9E5),
    searchHitBackground: Color(0x80EAC55F),
    searchHitBackgroundCurrent: Color(0xFF966600),
    searchHitForeground: Color(0xFFADBAC7),
  );
}
