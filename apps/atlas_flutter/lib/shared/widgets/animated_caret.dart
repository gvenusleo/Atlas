import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/scheduler.dart';
import 'package:material_ui/material_ui.dart';

import 'caret_animation.dart';

/// Draws an animated caret over a text field.
///
/// Wraps exactly one field, and that field must not paint a caret of its own —
/// build it with `showCursor: false`. The field's own [RenderEditable]
/// supplies the caret geometry and color, so the animated caret follows the
/// field's padding, wrapping, scrolling, and error state without duplicating
/// layout.
///
/// The caret is a quad whose four corners spring toward their new positions
/// (see [CaretAnimation]), so the leading edge arrives before the trailing
/// edge. The animation is skipped when the platform requests reduced motion,
/// and the caret hides while [TickerMode] is disabled. While the field is
/// unfocused or a range selection covers text the caret hides and the springs
/// reset, so it reappears in place rather than sliding in.
class const AnimatedCaret({
  super.key,

  /// Controller of the wrapped field; its selection drives the caret.
  required final TextEditingController controller,

  /// The text field to decorate, built with `showCursor: false`.
  required final Widget child,
}) extends StatefulWidget {
  /// Wraps [child] with an animated caret driven by [controller].
  this;

  @override
  State<AnimatedCaret> createState() => _AnimatedCaretState();
}

class _AnimatedCaretState extends State<AnimatedCaret>
    with SingleTickerProviderStateMixin {
  /// Cursor blink half period, matching the framework's own caret.
  static const _blinkHalfPeriod = Duration(milliseconds: 500);

  final _animation = CaretAnimation();
  final _paint = ValueNotifier(const _CaretPaint.hidden());
  final _overlayKey = GlobalKey();
  late final Ticker _ticker;

  RenderEditable? _field;
  Timer? _blinkTimer;
  var _hasFocus = false;
  var _blinkOn = true;
  var _reduceMotion = false;
  var _syncScheduled = false;

  @override
  void initState() {
    super.initState();
    assert(
      widget.child is! TextField ||
          (widget.child as TextField).showCursor != true,
      'Build the field wrapped by AnimatedCaret with showCursor: false.',
    );
    _ticker = createTicker((_) => _sync());
    widget.controller.addListener(_handleControllerChanged);
    FocusManager.instance.addListener(_handleFocusChanged);
  }

  @override
  void didUpdateWidget(covariant AnimatedCaret oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.controller != widget.controller) {
      oldWidget.controller.removeListener(_handleControllerChanged);
      widget.controller.addListener(_handleControllerChanged);
    }
    _scheduleSync();
  }

  @override
  void dispose() {
    widget.controller.removeListener(_handleControllerChanged);
    FocusManager.instance.removeListener(_handleFocusChanged);
    _field?.offset.removeListener(_handleFieldScroll);
    _stopBlink();
    _ticker.dispose();
    _paint.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    _reduceMotion = MediaQuery.disableAnimationsOf(context);
    if (!TickerMode.valuesOf(context).enabled) {
      _animation.reset();
    }
    // Rebuilds (resizes, theme and style changes) move the caret without
    // touching the controller. The builder deliberately avoids LayoutBuilder:
    // dialogs measure their content with intrinsics, which LayoutBuilder
    // cannot answer.
    _scheduleSync();
    return Stack(
      fit: StackFit.passthrough,
      children: [
        widget.child,
        Positioned.fill(
          child: IgnorePointer(
            key: const ValueKey('animated-caret-overlay'),
            child: ClipRect(
              child: CustomPaint(
                key: _overlayKey,
                painter: _CaretPainter(_paint),
              ),
            ),
          ),
        ),
      ],
    );
  }

  void _handleControllerChanged() {
    // The caret that just moved starts out visible, like the framework's own.
    _blinkOn = true;
    _stopBlink();
    _scheduleSync();
  }

  void _handleFocusChanged() {
    final hasFocus = _ownsFocus();
    if (hasFocus == _hasFocus) {
      return;
    }
    _hasFocus = hasFocus;
    _blinkOn = true;
    _scheduleSync();
  }

  /// Whether the primary focus of the app sits inside this wrapper.
  bool _ownsFocus() {
    final focused = FocusManager.instance.primaryFocus?.context;
    if (focused == null) {
      return false;
    }
    var owns = false;
    focused.visitAncestorElements((ancestor) {
      if (ancestor == context) {
        owns = true;
        return false;
      }
      return true;
    });
    return owns;
  }

  /// Samples the field after the current frame, when its layout is complete.
  void _scheduleSync() {
    if (_syncScheduled) {
      return;
    }
    _syncScheduled = true;
    SchedulerBinding.instance.addPostFrameCallback((_) {
      _syncScheduled = false;
      _sync();
    });
    SchedulerBinding.instance.scheduleFrame();
  }

  void _sync() {
    if (!mounted) {
      return;
    }
    final field = _resolveField();
    Rect? rect;
    List<Offset>? quad;
    Color? color;
    if (field != null && _hasFocus && TickerMode.valuesOf(context).enabled) {
      final selection = field.selection;
      if (selection != null && selection.isCollapsed) {
        rect = field.getLocalRectForCaret(
          TextPosition(offset: selection.baseOffset),
        );
        color = _caretColor(field);
        if (_reduceMotion) {
          _animation.reset();
        } else {
          quad = _animation.update(
            rect: rect,
            caretOffset: selection.baseOffset,
            timestamp: SchedulerBinding.instance.currentFrameTimeStamp,
          );
        }
      }
    }
    if (rect == null) {
      _animation.reset();
      _blinkOn = true;
      _stopBlink();
      _apply(const _CaretPaint.hidden());
    } else {
      _startBlink();
      final delta = _overlayDelta(rect);
      _apply(
        _CaretPaint(
          rect: rect.shift(delta),
          quad: quad == null
              ? null
              : [for (final corner in quad) corner + delta],
          color: color!,
          visible: _blinkOn,
        ),
      );
    }
    _syncTicker();
  }

  /// Resolves the wrapped field's render object, walking the render tree when
  /// it has not been found yet or was rebuilt.
  RenderEditable? _resolveField() {
    final cached = _field;
    if (cached != null && cached.attached && cached.hasSize) {
      return cached;
    }
    RenderEditable? found;
    var candidates = 0;
    void visit(RenderObject node) {
      if (node is RenderEditable) {
        found ??= node;
        candidates++;
        return;
      }
      node.visitChildren(visit);
    }

    final root = context.findRenderObject();
    if (root != null) {
      visit(root);
    }
    assert(
      candidates <= 1,
      'AnimatedCaret decorates one field, but this subtree holds '
      '$candidates.',
    );
    return _bindField(found);
  }

  /// Binds [field] to the caret, replacing the previously bound field.
  RenderEditable? _bindField(RenderEditable? field) {
    final previous = _field;
    if (previous == field) {
      return field;
    }
    previous?.offset.removeListener(_handleFieldScroll);
    _field = field;
    field?.offset.addListener(_handleFieldScroll);
    return field;
  }

  /// A scroll moves the caret geometry without touching controller or focus.
  void _handleFieldScroll() => _scheduleSync();

  /// Resolves the color to paint the caret with.
  ///
  /// Hiding the native caret (`showCursor: false`) leaves the field's cursor
  /// color transparent, but its channels still carry the color the framework
  /// resolved for this field, including the error color of a failing
  /// decoration.
  Color _caretColor(RenderEditable field) {
    final color = field.cursorColor ?? const Color(0xFF000000);
    return color.a == 0 ? color.withValues(alpha: 1) : color;
  }

  /// Offset that moves a caret rect from the field's coordinates into the
  /// overlay's.
  Offset _overlayDelta(Rect rect) {
    final field = _field;
    final overlay =
        _overlayKey.currentContext?.findRenderObject() as RenderBox?;
    if (field == null || overlay == null || !overlay.attached) {
      return Offset.zero;
    }
    return overlay.globalToLocal(field.localToGlobal(rect.topLeft)) -
        rect.topLeft;
  }

  void _apply(_CaretPaint paint) {
    if (paint == _paint.value) {
      return;
    }
    _paint.value = paint;
  }

  void _syncTicker() {
    if (_animation.isActive) {
      if (!_ticker.isActive) {
        _ticker.start();
      }
    } else if (_ticker.isActive) {
      _ticker.stop();
    }
  }

  void _startBlink() {
    // Tests that need a deterministic caret do not blink, like the
    // framework's own caret.
    if (EditableText.debugDeterministicCursor) {
      _blinkOn = true;
      return;
    }
    _blinkTimer ??= Timer.periodic(_blinkHalfPeriod, (_) {
      _blinkOn = !_blinkOn;
      _scheduleSync();
    });
  }

  void _stopBlink() {
    _blinkTimer?.cancel();
    _blinkTimer = null;
  }
}

/// One frame's worth of caret drawing state.
@immutable
class _CaretPaint {
  const new({
    required this.rect,
    required this.quad,
    required this.color,
    required this.visible,
  });

  const new hidden()
    : rect = null,
      quad = null,
      color = const Color(0x00000000),
      visible = false;

  /// Caret rectangle in the overlay's coordinates.
  final Rect? rect;

  /// Animated quad in the overlay's coordinates; null draws [rect].
  final List<Offset>? quad;

  /// Resolved caret color of the wrapped field.
  final Color color;

  /// Whether the blink phase currently shows the caret.
  final bool visible;

  @override
  bool operator ==(Object other) =>
      other is _CaretPaint &&
      other.rect == rect &&
      other.color == color &&
      other.visible == visible &&
      listEquals(other.quad, quad);

  @override
  int get hashCode => Object.hash(rect, color, visible, quad?.length);
}

/// Paints the caret quad, or the plain caret rectangle while it settles.
class _CaretPainter(final ValueListenable<_CaretPaint> caret)
    extends CustomPainter {
  this : super(repaint: caret);

  @override
  void paint(Canvas canvas, Size size) {
    final snapshot = caret.value;
    final rect = snapshot.rect;
    if (!snapshot.visible || rect == null) {
      return;
    }
    final brush = Paint()..color = snapshot.color;
    final quad = snapshot.quad;
    if (quad == null) {
      canvas.drawRect(rect, brush);
      return;
    }
    canvas.drawPath(Path()..addPolygon(quad, true), brush);
  }

  // Repaints come from the caret notifier, not from new painter instances.
  @override
  bool shouldRepaint(covariant _CaretPainter oldDelegate) => false;

  @override
  bool shouldRebuildSemantics(covariant _CaretPainter oldDelegate) => false;
}
