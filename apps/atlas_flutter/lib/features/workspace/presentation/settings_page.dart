import 'dart:async';
import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../app/theme_mode.dart';
import '../../../../app/locale_mode.dart';
import '../../../../l10n/localizations.dart';
import '../../../../shared/theme/atlas_theme.dart';
import 'widgets/connections_settings.dart';
import 'widgets/language_selector.dart';
import 'widgets/settings_controls.dart';
import 'widgets/side_panel.dart';
import 'widgets/workspace_controls.dart';
import 'workspace_metrics.dart';

/// Sections offered by the settings surface.
enum SettingsSection {
  /// Client-local appearance and language preferences.
  appearance(LucideIcons.sunMoon),

  /// ACP server connections and runtime switching.
  connections(LucideIcons.plug);

  const SettingsSection(this.icon);
  final IconData icon;

  String label(BuildContext context) => switch (this) {
    SettingsSection.appearance => context.l10n.appearance,
    SettingsSection.connections => context.l10n.connections,
  };
}

/// Settings page for client-local preferences and ACP connections.
///
/// The page is a section rail beside a content pane, and it owns the window
/// chrome the workspace shell provides elsewhere: the traffic-light inset, the
/// titlebar drag areas, and the custom caption controls on platforms without
/// native ones.
///
/// Navigation lives in the rail, not in a toolbar above the whole window: the
/// rail reuses the sessions sidebar skeleton, so its header carries the back
/// button and the surface title while the pane starts at the same baseline.
/// Compact layouts hide the rail and switch sections from a strip inside the
/// pane instead.
class const SettingsPage({super.key}) extends ConsumerStatefulWidget {
  /// Width of the section rail on desktop layouts.
  static const railWidth = WorkspaceMetrics.leftDefaultWidth;

  /// Maximum width of the settings rows inside the content pane.
  ///
  /// Rows stop short of the pane edge so a label and its control stay in one
  /// glance on a wide window.
  static const columnWidth = 640.0;

  @override
  ConsumerState<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends ConsumerState<SettingsPage> {
  var _section = SettingsSection.appearance;

  void _select(SettingsSection section) => setState(() => _section = section);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: WorkspaceResizeRing(
        child: SafeArea(
          child: LayoutBuilder(
            builder: (context, constraints) {
              final desktop =
                  constraints.maxWidth >= WorkspaceMetrics.desktopBreakpoint;
              return Row(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (desktop) ...[
                    _SectionRail(section: _section, onSelect: _select),
                    const _VerticalHairline(),
                  ],
                  Expanded(
                    child: _SettingsPane(
                      section: _section,
                      onSelect: _select,
                      showRail: desktop,
                    ),
                  ),
                ],
              );
            },
          ),
        ),
      ),
    );
  }
}

/// Section list with the surface navigation in its header.
class const _SectionRail({
  required final SettingsSection section,
  required final ValueChanged<SettingsSection> onSelect,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return SizedBox(
      key: const ValueKey('atlas-settings-rail'),
      width: SettingsPage.railWidth,
      child: SidePanel(
        semanticLabel: context.l10n.settings,
        // The panel header already pads 4px; the rest of the traffic-light
        // inset keeps the back button clear of the native controls.
        title: Padding(
          padding: EdgeInsets.only(
            left: WorkspaceMetrics.showsTrafficLights
                ? WorkspaceMetrics.macOSTrafficLightInset - 4
                : 0,
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              WorkspaceToolbarButton(
                key: const ValueKey('atlas-settings-back'),
                icon: LucideIcons.arrowLeft,
                tooltip: context.l10n.back,
                onPressed: () => context.pop(),
              ),
              const SizedBox(width: 8),
              Text(
                context.l10n.settings,
                style: TextStyle(
                  color: colors.textPrimary,
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
        ),
        child: Padding(
          padding: const EdgeInsets.all(8),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final item in SettingsSection.values)
                _SectionEntry(
                  key: ValueKey('atlas-settings-rail-${item.name}'),
                  section: item,
                  selected: item == section,
                  onSelect: onSelect,
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// One selectable section in the rail.
class const _SectionEntry({
  super.key,
  required final SettingsSection section,
  required final bool selected,
  required final ValueChanged<SettingsSection> onSelect,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 2),
      child: WorkspaceHoverSurface(
        // The selected section keeps the highlight while not hovered.
        color: selected ? colors.raised : null,
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: () => onSelect(section),
          child: Container(
            constraints: const BoxConstraints(minHeight: 30),
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
            child: Row(
              children: [
                Icon(
                  section.icon,
                  size: 14,
                  color: selected ? colors.textPrimary : colors.textSecondary,
                ),
                const SizedBox(width: 8),
                Flexible(
                  child: Text(
                    section.label(context),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      color: selected
                          ? colors.textPrimary
                          : colors.textSecondary,
                      fontSize: 12.5,
                      fontWeight: selected ? FontWeight.w500 : FontWeight.w400,
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// One pixel separator between the rail and the content pane.
class const _VerticalHairline() extends StatelessWidget {
  @override
  Widget build(BuildContext context) =>
      Container(width: 1, color: AtlasColors.of(context).divider);
}

/// Horizontal section strip used when the rail is hidden.
class const _SectionStrip({
  required final SettingsSection section,
  required final ValueChanged<SettingsSection> onSelect,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      child: Row(
        children: [
          for (final item in SettingsSection.values)
            Padding(
              padding: const EdgeInsets.only(right: 6),
              child: WorkspaceHoverSurface(
                key: ValueKey('atlas-settings-strip-${item.name}'),
                color: item == section ? colors.raised : null,
                borderRadius: BorderRadius.circular(AtlasRadii.control),
                child: GestureDetector(
                  behavior: HitTestBehavior.opaque,
                  onTap: () => onSelect(item),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 10,
                      vertical: 6,
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          item.icon,
                          size: 14,
                          color: item == section
                              ? colors.textPrimary
                              : colors.textSecondary,
                        ),
                        const SizedBox(width: 6),
                        Text(
                          item.label(context),
                          style: TextStyle(
                            color: item == section
                                ? colors.textPrimary
                                : colors.textSecondary,
                            fontSize: 12.5,
                            fontWeight: item == section
                                ? FontWeight.w500
                                : FontWeight.w400,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

/// Scrolling content pane holding the selected section.
///
/// Without the rail (compact layouts and phone screens) the same header line
/// carries the back button, so navigation never disappears.
class const _SettingsPane({
  required final SettingsSection section,
  required final ValueChanged<SettingsSection> onSelect,
  required final bool showRail,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final compact = !showRail;
    final showsControls = usesCaptionControls(
      desktop: Platform.environment['XDG_CURRENT_DESKTOP'] ?? '',
      sessionType: Platform.environment['XDG_SESSION_TYPE'] ?? '',
    );
    return ColoredBox(
      key: const ValueKey('atlas-settings-pane'),
      color: colors.canvas,
      child: Column(
        children: [
          SizedBox(
            height: compact
                ? WorkspaceMetrics.compactToolbarHeight
                : WorkspaceMetrics.desktopToolbarHeight,
            child: WorkspaceTitlebarDragArea(
              child: Row(
                children: [
                  if (!showRail) ...[
                    SizedBox(
                      width: WorkspaceMetrics.showsTrafficLights
                          ? WorkspaceMetrics.macOSTrafficLightInset
                          : 6,
                    ),
                    WorkspaceToolbarButton(
                      key: const ValueKey('atlas-settings-back'),
                      icon: LucideIcons.arrowLeft,
                      tooltip: context.l10n.back,
                      size: compact
                          ? 44
                          : WorkspaceMetrics.desktopToolbarButtonSize,
                      onPressed: () => context.pop(),
                    ),
                    const SizedBox(width: 8),
                    Text(
                      context.l10n.settings,
                      style: TextStyle(
                        color: colors.textPrimary,
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                  const Spacer(),
                  if (showsControls) ...[
                    const AtlasWindowControls(),
                    const SizedBox(width: 6),
                  ],
                ],
              ),
            ),
          ),
          const Divider(height: 1),
          if (!showRail) ...[
            _SectionStrip(section: section, onSelect: onSelect),
            const Divider(height: 1),
          ],
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(32, 28, 32, 40),
              child: Align(
                alignment: Alignment.topLeft,
                child: ConstrainedBox(
                  constraints: const BoxConstraints(
                    maxWidth: SettingsPage.columnWidth,
                  ),
                  child: switch (section) {
                    SettingsSection.appearance => const _AppearanceSection(),
                    SettingsSection.connections => const ConnectionsSettings(),
                  },
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// The appearance group: title, summary, and the theme row.
class const _AppearanceSection() extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final mode = ref.watch(themeModeProvider);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SettingsSectionHeader(
          title: context.l10n.appearance,
          description: context.l10n.appearanceDescription,
        ),
        const Divider(height: 1),
        _SettingRow(
          label: context.l10n.theme,
          description: context.l10n.themeDescription,
          control: _ThemeModeSelector(
            mode: mode,
            onChanged: (selected) => unawaited(
              ref.read(themeModeProvider.notifier).select(selected),
            ),
          ),
        ),
        const Divider(height: 1),
        _SettingRow(
          label: context.l10n.language,
          description: context.l10n.languageDescription,
          control: LanguageSelector(
            language: ref.watch(languageProvider),
            onChanged: (selected) =>
                unawaited(ref.read(languageProvider.notifier).select(selected)),
          ),
        ),
      ],
    );
  }
}

/// One labelled setting with its control on the trailing edge of the row.
class const _SettingRow({
  required final String label,
  required final String description,
  required final Widget control,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final text = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          label,
          style: TextStyle(
            color: colors.textPrimary,
            fontSize: 13,
            fontWeight: FontWeight.w500,
          ),
        ),
        const SizedBox(height: 3),
        Text(
          description,
          style: TextStyle(
            color: colors.textSecondary,
            fontSize: 12,
            height: 1.4,
          ),
        ),
      ],
    );

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 14),
      child: LayoutBuilder(
        builder: (context, constraints) {
          // Narrow panes stack the control under its label: the segmented
          // selector does not shrink below its three segments.
          if (constraints.maxWidth < 420) {
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [text, const SizedBox(height: 10), control],
            );
          }
          return Row(
            children: [
              Expanded(child: text),
              const SizedBox(width: 24),
              control,
            ],
          );
        },
      ),
    );
  }
}

/// Three-way selector for the client-local theme mode.
class const _ThemeModeSelector({
  required final ThemeMode mode,
  required final ValueChanged<ThemeMode> onChanged,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return SettingsSegmentedButton<ThemeMode>(
      segments: [
        ButtonSegment(
          value: ThemeMode.light,
          label: Text(context.l10n.light),
          tooltip: context.l10n.lightTooltip,
        ),
        ButtonSegment(
          value: ThemeMode.dark,
          label: Text(context.l10n.dark),
          tooltip: context.l10n.darkTooltip,
        ),
        ButtonSegment(
          value: ThemeMode.system,
          label: Text(context.l10n.system),
          tooltip: context.l10n.systemThemeTooltip,
        ),
      ],
      selected: mode,
      onChanged: onChanged,
    );
  }
}
