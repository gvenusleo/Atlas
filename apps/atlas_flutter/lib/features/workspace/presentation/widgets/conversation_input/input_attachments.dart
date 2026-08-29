import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../../shared/theme/atlas_theme.dart';
import '../../../data/image_attachment.dart';
import '../workspace_controls.dart';

/// Toolbar trigger that opens the image picker.
class AttachImageButton extends StatelessWidget {
  /// Creates an image attachment trigger.
  const AttachImageButton({
    super.key,
    required this.enabled,
    required this.attaching,
    required this.onPressed,
  });

  final bool enabled;
  final bool attaching;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Tooltip(
      message: enabled
          ? 'Attach image'
          : 'Current model does not support images',
      child: WorkspaceHoverSurface(
        enabled: enabled && !attaching,
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        child: IconButton(
          key: const ValueKey('atlas-attach-image'),
          padding: EdgeInsets.zero,
          constraints: const BoxConstraints.tightFor(width: 28, height: 28),
          style: IconButton.styleFrom(overlayColor: Colors.transparent),
          onPressed: enabled && !attaching ? onPressed : null,
          icon: Icon(
            LucideIcons.paperclip,
            size: 14,
            color: enabled ? colors.textSecondary : colors.divider,
          ),
        ),
      ),
    );
  }
}

/// Row of images staged for the next message.
class PendingImageStrip extends StatelessWidget {
  /// Creates a pending image strip.
  const PendingImageStrip({
    super.key,
    required this.images,
    required this.onRemove,
  });

  final List<PendingImage> images;
  final ValueChanged<int> onRemove;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(6, 4, 6, 4),
      child: Align(
        alignment: Alignment.centerLeft,
        child: Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final (index, image) in images.indexed)
              PendingImageChip(image: image, onRemove: () => onRemove(index)),
          ],
        ),
      ),
    );
  }
}

/// A single staged image with a remove affordance.
class PendingImageChip extends StatelessWidget {
  /// Creates a pending image chip.
  const PendingImageChip({
    super.key,
    required this.image,
    required this.onRemove,
  });

  final PendingImage image;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return SizedBox(
      width: 56,
      height: 56,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          Positioned.fill(
            child: ClipRRect(
              borderRadius: BorderRadius.circular(AtlasRadii.control),
              child: Image.memory(
                image.bytes,
                fit: BoxFit.cover,
                gaplessPlayback: true,
              ),
            ),
          ),
          Positioned(
            top: -4,
            right: -4,
            child: Tooltip(
              message: 'Remove image',
              child: Material(
                color: colors.canvas,
                shape: const CircleBorder(),
                child: InkWell(
                  customBorder: const CircleBorder(),
                  onTap: onRemove,
                  child: SizedBox.square(
                    dimension: 18,
                    child: Icon(
                      LucideIcons.x,
                      size: 11,
                      color: colors.textSecondary,
                    ),
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
