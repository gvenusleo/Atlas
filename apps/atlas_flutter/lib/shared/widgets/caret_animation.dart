import 'dart:math' as math;
import 'dart:ui' show Offset, Rect;

/// Animated caret geometry, ported from Zed's cursor movement animation.
///
/// Zed's `cursor_animation` setting animates the caret as a quad: four
/// corners, each driven by its own critically damped spring. Corner durations
/// differ by how much the corner leads the movement, so the leading edge
/// arrives first and the trailing edge catches up — the caret stretches
/// while it travels instead of sliding rigidly.
///
/// Constants, corner ranking, and frame clamps mirror
/// `crates/editor/src/cursor_animation.rs` of zed-industries/zed (GPL-3.0).
/// The physics there is adapted from the MIT licensed `vscode-neovide-cursor`
/// extension.
class CaretAnimation {
  /// Longest frame time fed to the springs; a stalled frame must not make the
  /// caret jump.
  static const maxFrameDuration = Duration(milliseconds: 33);

  /// Duration of a move across rows or over longer distances, in seconds.
  static const _animationLength = 0.125;

  /// Duration of a short horizontal move, in seconds.
  static const _shortAnimationLength = 0.05;

  /// Duration for corners that lead the movement, in seconds.
  static const _leadingSnapLength = 0.02;

  /// Difference between leading and trailing corner durations; 0 moves the
  /// quad rigidly.
  static const _trailSize = 1.0;

  /// Duration factors by corner rank, from most trailing to most leading.
  static const _rankTrailFactors = <double>[1.0, 0.9, 0.5, 0.3];

  /// Horizontal travel, in caret widths, that still counts as a short move.
  static const _shortMoveWidths = 8.0;

  /// Alignment above which a corner leads the movement.
  static const _leadingAlignment = 0.5;

  /// Moves whose duration exceeds this drop the accumulated velocity.
  static const _velocityResetLength = 0.075;

  /// Displacements below these pixel values are treated as settled.
  static const _springResetEpsilon = 0.001;
  static const _springActiveEpsilon = 0.01;
  static const _cornerActiveDistance = 0.5;

  /// Differences below this pixel value are ignored when comparing geometry.
  static const _geometryEpsilon = 0.01;

  /// Upper bound for a corner displacement, in caret sizes.
  static const _maxTrailDistanceFactor = 100.0;

  final _corners = <_Corner>[
    _Corner(const Offset(-0.5, -0.5)),
    _Corner(const Offset(0.5, -0.5)),
    _Corner(const Offset(0.5, 0.5)),
    _Corner(const Offset(-0.5, 0.5)),
  ];

  Rect? _targetRect;
  int? _lastCaretOffset;
  Duration? _lastTimestamp;
  var _active = false;

  /// Whether the springs still move the caret.
  bool get isActive => _active;

  /// Forgets the caret, so the next [update] snaps instead of animating.
  void reset() {
    _targetRect = null;
    _lastCaretOffset = null;
    _lastTimestamp = null;
    _active = false;
    for (final corner in _corners) {
      corner.reset();
    }
  }

  /// Feeds one layout sample of the caret.
  ///
  /// [rect] is the caret exactly as the field laid it out, in the field's own
  /// coordinates; [caretOffset] identifies the logical caret position in the
  /// text and [timestamp] the monotonic frame time. A changed [caretOffset]
  /// retargets the springs; a geometry-only change translates an in-flight
  /// quad (scrolling must not restart the animation) and leaves a settled
  /// caret alone. Returns the animated quad, or null while the caret should
  /// be drawn as a plain [rect].
  List<Offset>? update({
    required Rect rect,
    required int caretOffset,
    required Duration timestamp,
  }) {
    if (!rect.isFinite || rect.width <= 0 || rect.height <= 0) {
      reset();
      return null;
    }
    final lastRect = _targetRect;
    final lastTimestamp = _lastTimestamp;
    if (lastRect == null || lastTimestamp == null) {
      _snap(rect, caretOffset, timestamp);
      return null;
    }
    if (_lastCaretOffset != caretOffset) {
      final elapsed = _active
          ? _elapsedSince(timestamp, lastTimestamp)
          : Duration.zero;
      _retarget(rect);
      _advance(elapsed);
    } else if (!_nearlySameRect(lastRect, rect)) {
      if (_active) {
        _translate(rect.topLeft - lastRect.topLeft);
      } else {
        _snap(rect, caretOffset, timestamp);
        return null;
      }
    } else if (_active) {
      _advance(_elapsedSince(timestamp, lastTimestamp));
    }
    _lastCaretOffset = caretOffset;
    _lastTimestamp = timestamp;
    if (!_active) {
      _snap(rect, caretOffset, timestamp);
      return null;
    }
    return [for (final corner in _corners) corner.current];
  }

  void _snap(Rect rect, int caretOffset, Duration timestamp) {
    for (final corner in _corners) {
      corner.snap(rect);
    }
    _targetRect = rect;
    _lastCaretOffset = caretOffset;
    _lastTimestamp = timestamp;
    _active = false;
  }

  /// Aims every corner at [rect], with durations ordered by corner rank.
  void _retarget(Rect rect) {
    final alignments = [
      for (final corner in _corners) corner.directionAlignment(rect),
    ];
    final order = [0, 1, 2, 3]
      ..sort((a, b) {
        final result = alignments[a].compareTo(alignments[b]);
        return result != 0 ? result : a.compareTo(b);
      });
    final ranks = List.filled(4, 0);
    for (var rank = 0; rank < 4; rank++) {
      ranks[order[rank]] = rank;
    }
    for (var index = 0; index < 4; index++) {
      _corners[index].retarget(rect, ranks[index]);
    }
    _targetRect = rect;
    _active = _corners.any((corner) => corner.displacement != Offset.zero);
  }

  /// Moves the whole quad, keeping the springs' velocities.
  void _translate(Offset delta) {
    if (delta == Offset.zero) {
      return;
    }
    for (final corner in _corners) {
      corner.translate(delta);
    }
    _targetRect = _targetRect?.shift(delta);
  }

  void _advance(Duration elapsed) {
    final rect = _targetRect;
    if (rect == null) {
      return;
    }
    final seconds = elapsed.inMicroseconds / Duration.microsecondsPerSecond;
    final maxTrail =
        math.max(rect.width, rect.height) * _maxTrailDistanceFactor;
    var active = false;
    for (final corner in _corners) {
      active = corner.update(seconds, maxTrail) || active;
    }
    if (!active) {
      for (final corner in _corners) {
        corner.snap(rect);
      }
    }
    _active = active;
  }

  Duration _elapsedSince(Duration now, Duration then) {
    final elapsed = now - then;
    if (elapsed.isNegative) {
      return Duration.zero;
    }
    return elapsed > maxFrameDuration ? maxFrameDuration : elapsed;
  }

  static bool _nearlySameRect(Rect left, Rect right) =>
      _nearlyEqual(left.left, right.left) &&
      _nearlyEqual(left.top, right.top) &&
      _nearlyEqual(left.width, right.width) &&
      _nearlyEqual(left.height, right.height);

  static bool _nearlyEqual(double left, double right) =>
      (left - right).abs() <= _geometryEpsilon;
}

/// One corner of the caret quad, spring driven along each axis.
class _Corner {
  _Corner(this.relativePosition);

  final Offset relativePosition;
  final _horizontal = _Spring();
  final _vertical = _Spring();
  var current = Offset.zero;
  var target = Offset.zero;
  var animationLength = 0.0;

  /// Current offset from the target, in pixels.
  Offset get displacement => Offset(_horizontal.position, _vertical.position);

  /// Where this corner sits for a caret laid out as [rect].
  Offset destination(Rect rect) =>
      rect.center +
      Offset(
        relativePosition.dx * rect.width,
        relativePosition.dy * rect.height,
      );

  /// How much this corner leads the travel toward [rect], as a dot product.
  double directionAlignment(Rect rect) {
    final travel = _normalized(destination(rect) - current);
    final direction = _normalized(relativePosition);
    return travel.dx * direction.dx + travel.dy * direction.dy;
  }

  void snap(Rect rect) {
    final position = destination(rect);
    current = position;
    target = position;
    _horizontal.reset();
    _vertical.reset();
  }

  void translate(Offset delta) {
    current += delta;
    target += delta;
  }

  void retarget(Rect rect, int rank) {
    final dest = destination(rect);
    final horizontalJump = (dest.dx - target.dx) / math.max(rect.width, 1e-6);
    final verticalJump = (dest.dy - target.dy) / math.max(rect.height, 1e-6);
    final jump = _normalized(Offset(horizontalJump, verticalJump));
    final direction = _normalized(relativePosition);
    final leadingAlignment = jump.dx * direction.dx + jump.dy * direction.dy;
    final isShortJump =
        horizontalJump.abs() <= CaretAnimation._shortMoveWidths &&
        verticalJump.abs() <= CaretAnimation._springResetEpsilon;

    final base = isShortJump
        ? math.min(
            CaretAnimation._animationLength,
            CaretAnimation._shortAnimationLength,
          )
        : CaretAnimation._animationLength;
    final reference = leadingAlignment > CaretAnimation._leadingAlignment
        ? CaretAnimation._leadingSnapLength
        : base * CaretAnimation._rankTrailFactors[math.min(rank, 3)];
    animationLength = base + (reference - base) * CaretAnimation._trailSize;

    if (animationLength > CaretAnimation._velocityResetLength) {
      _horizontal.reset();
      _vertical.reset();
    }
    target = dest;
    _horizontal.position = dest.dx - current.dx;
    _vertical.position = dest.dy - current.dy;
  }

  /// Advances the springs and returns whether the corner is still moving.
  bool update(double elapsedSeconds, double maxTrailDistance) {
    _horizontal.update(elapsedSeconds, animationLength);
    _vertical.update(elapsedSeconds, animationLength);
    _horizontal.clampTo(maxTrailDistance);
    _vertical.clampTo(maxTrailDistance);
    current = target - displacement;
    return _horizontal.position.abs() > CaretAnimation._cornerActiveDistance ||
        _vertical.position.abs() > CaretAnimation._cornerActiveDistance;
  }

  void reset() {
    current = Offset.zero;
    target = Offset.zero;
    animationLength = 0;
    _horizontal.reset();
    _vertical.reset();
  }

  static Offset _normalized(Offset value) {
    final length = value.distance;
    if (length == 0 || !length.isFinite) {
      return Offset.zero;
    }
    return value / length;
  }
}

/// Critically damped spring, solved in closed form for one axis.
class _Spring {
  var position = 0.0;
  var velocity = 0.0;

  /// Advances the spring and returns whether it is still moving.
  bool update(double elapsedSeconds, double animationLength) {
    if (!elapsedSeconds.isFinite ||
        elapsedSeconds < 0 ||
        !animationLength.isFinite ||
        animationLength <= elapsedSeconds ||
        position.abs() < CaretAnimation._springResetEpsilon) {
      reset();
      return false;
    }
    if (elapsedSeconds == 0) {
      return position.abs() >= CaretAnimation._springActiveEpsilon;
    }

    final angularFrequency = 4.0 / animationLength;
    final initialPosition = position;
    final combinedVelocity = position * angularFrequency + velocity;
    final decay = math.exp(-angularFrequency * elapsedSeconds);
    position = (initialPosition + combinedVelocity * elapsedSeconds) * decay;
    velocity =
        decay *
        (-initialPosition * angularFrequency -
            combinedVelocity * elapsedSeconds * angularFrequency +
            combinedVelocity);
    if (!position.isFinite ||
        !velocity.isFinite ||
        position.abs() < CaretAnimation._springResetEpsilon) {
      reset();
      return false;
    }
    return position.abs() >= CaretAnimation._springActiveEpsilon;
  }

  void clampTo(double limit) {
    position = position.clamp(-limit, limit);
  }

  void reset() {
    position = 0;
    velocity = 0;
  }
}
