import 'dart:ui' show ImageByteFormat;

import 'package:atlas_flutter/shared/widgets/animated_caret.dart';
import 'package:atlas_flutter/shared/widgets/caret_animation.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

const _frame = Duration(milliseconds: 16);

final _captureKey = GlobalKey();

Rect _caret(double left) => Rect.fromLTWH(left, 100, 2, 20);

/// Counts pixels of [color] in a screenshot, optionally only inside
/// [within].
Future<int> _caretPixels(
  WidgetTester tester,
  Color color, {
  Rect? within,
}) async {
  final boundary = tester.renderObject<RenderRepaintBoundary>(
    find.byKey(_captureKey),
  );
  final image = await tester.runAsync(() => boundary.toImage());
  final data = await tester.runAsync(
    () => image!.toByteData(format: ImageByteFormat.rawRgba),
  );
  final size = image!.width * image.height;
  final target = _argb(color);
  var count = 0;
  for (var index = 0; index < size; index++) {
    final x = index % image.width;
    final y = index ~/ image.width;
    if (within != null &&
        !within.contains(Offset(x.toDouble(), y.toDouble()))) {
      continue;
    }
    if (data!.getUint32(index * 4) == target) {
      count++;
    }
  }
  return count;
}

int _argb(Color color) =>
    (color.r * 255).round() << 24 |
    (color.g * 255).round() << 16 |
    (color.b * 255).round() << 8 |
    (color.a * 255).round();

void main() {
  group('CaretAnimation', () {
    test('snaps on first sight and on geometry-only moves', () {
      final animation = CaretAnimation();
      expect(
        animation.update(
          rect: _caret(100),
          caretOffset: 0,
          timestamp: Duration.zero,
        ),
        isNull,
      );
      expect(animation.isActive, isFalse);

      // Same caret, new geometry (scrolling): a settled caret just follows.
      expect(
        animation.update(rect: _caret(140), caretOffset: 0, timestamp: _frame),
        isNull,
      );
      expect(animation.isActive, isFalse);
    });

    test('lets the leading edge arrive before the trailing edge', () {
      final animation = CaretAnimation();
      animation.update(
        rect: _caret(100),
        caretOffset: 0,
        timestamp: Duration.zero,
      );
      animation.update(rect: _caret(108), caretOffset: 1, timestamp: _frame);
      final quad = animation.update(
        rect: _caret(108),
        caretOffset: 1,
        timestamp: _frame * 2,
      );
      expect(quad, isNotNull);
      expect(animation.isActive, isTrue);

      // The quad is stretched while it travels: corners 1 and 2 lead the
      // movement, corners 0 and 3 trail it.
      expect(quad![2].dx - quad[0].dx, greaterThan(_caret(108).width));
    });

    test('converges onto the caret rectangle and stops being active', () {
      final animation = CaretAnimation();
      animation.update(
        rect: _caret(100),
        caretOffset: 0,
        timestamp: Duration.zero,
      );
      animation.update(rect: _caret(108), caretOffset: 1, timestamp: _frame);

      var frames = 1;
      List<Offset>? quad = animation.update(
        rect: _caret(108),
        caretOffset: 1,
        timestamp: _frame * 2,
      );
      while (animation.isActive && frames < 60) {
        frames++;
        quad = animation.update(
          rect: _caret(108),
          caretOffset: 1,
          timestamp: _frame * (frames + 1),
        );
      }

      expect(animation.isActive, isFalse);
      expect(quad, isNull);
      expect(frames, lessThan(20));
    });

    test('translates an in-flight quad instead of restarting it', () {
      final animation = CaretAnimation();
      animation.update(
        rect: _caret(100),
        caretOffset: 0,
        timestamp: Duration.zero,
      );
      final started = animation.update(
        rect: _caret(108),
        caretOffset: 1,
        timestamp: _frame,
      )!;

      // A scroll moves the caret rect without moving the logical caret.
      final scrolled = animation.update(
        rect: _caret(148),
        caretOffset: 1,
        timestamp: _frame,
      )!;

      expect(scrolled[0].dx - started[0].dx, closeTo(40, 0.001));
      expect(scrolled[2].dx - started[2].dx, closeTo(40, 0.001));
      expect(animation.isActive, isTrue);
    });

    test('reset forgets the caret so the next sample snaps', () {
      final animation = CaretAnimation();
      animation.update(
        rect: _caret(100),
        caretOffset: 0,
        timestamp: Duration.zero,
      );
      expect(
        animation.update(rect: _caret(108), caretOffset: 1, timestamp: _frame),
        isNotNull,
      );

      animation.reset();
      expect(animation.isActive, isFalse);
      expect(
        animation.update(
          rect: _caret(300),
          caretOffset: 4,
          timestamp: _frame * 2,
        ),
        isNull,
      );
    });
  });

  group('AnimatedCaret', () {
    late TextEditingController controller;

    setUp(() {
      EditableText.debugDeterministicCursor = true;
      controller = TextEditingController(text: 'hello world');
      addTearDown(() {
        EditableText.debugDeterministicCursor = false;
        controller.dispose();
      });
    });

    /// Focuses the field through the test input connection.
    Future<void> focusField(WidgetTester tester) async {
      await tester.showKeyboard(find.byType(TextField));
      await tester.pumpAndSettle();
    }

    /// Removes focus from the field.
    Future<void> blurField(WidgetTester tester) async {
      FocusManager.instance.primaryFocus?.unfocus();
      await tester.pumpAndSettle();
    }

    Future<void> pumpField(
      WidgetTester tester, {
      bool reduceMotion = false,
      Color? cursorColor,
      int minLines = 1,
      int maxLines = 1,
    }) {
      Widget field() => AnimatedCaret(
        controller: controller,
        child: TextField(
          showCursor: false,
          cursorColor: cursorColor,
          controller: controller,
          minLines: minLines,
          maxLines: maxLines,
        ),
      );
      return tester.pumpWidget(
        MaterialApp(
          home: RepaintBoundary(
            key: _captureKey,
            child: Scaffold(
              body: Center(
                child: SizedBox(
                  width: 220,
                  child: reduceMotion
                      ? Builder(
                          builder: (context) => MediaQuery(
                            data: MediaQuery.of(context)
                                .copyWith(disableAnimations: true),
                            child: field(),
                          ),
                        )
                      : field(),
                ),
              ),
            ),
          ),
        ),
      );
    }

    Finder overlayFinder() =>
        find.byKey(const ValueKey('animated-caret-overlay'));

    RenderEditable fieldOf(WidgetTester tester) => tester
        .state<EditableTextState>(find.byType(EditableText))
        .renderEditable;

    /// The caret rectangle the field laid out, in the overlay's coordinates.
    Rect expectedCaretRect(WidgetTester tester) {
      final field = fieldOf(tester);
      final rect = field.getLocalRectForCaret(
        TextPosition(offset: controller.selection.baseOffset),
      );
      final overlay = tester.renderObject<RenderBox>(overlayFinder());
      return rect.shift(
        overlay.globalToLocal(field.localToGlobal(rect.topLeft)) - rect.topLeft,
      );
    }

    void paintCaret(WidgetTester tester, Canvas canvas) {
      final paint = tester.widget<CustomPaint>(
        find.descendant(
          of: overlayFinder(),
          matching: find.byType(CustomPaint),
        ),
      );
      paint.painter!.paint(canvas, tester.getSize(overlayFinder()));
    }

    testWidgets('paints nothing until the field is focused', (tester) async {
      await pumpField(tester);
      await tester.pumpAndSettle();

      expect(overlayFinder(), findsOneWidget);
      expect((Canvas canvas) => paintCaret(tester, canvas), paintsNothing);
    });

    testWidgets('paints the caret of a focused field', (tester) async {
      await pumpField(tester);
      await focusField(tester);
      controller.selection = const TextSelection.collapsed(offset: 5);
      await tester.pumpAndSettle();

      expect(
        (Canvas canvas) => paintCaret(tester, canvas),
        paints..rect(rect: expectedCaretRect(tester)),
      );
    });

    testWidgets('animates the caret while it moves and settles after', (
      tester,
    ) async {
      await pumpField(tester);
      await focusField(tester);
      controller.selection = const TextSelection.collapsed(offset: 2);
      await tester.pumpAndSettle();

      controller.selection = const TextSelection.collapsed(offset: 4);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 16));
      expect((Canvas canvas) => paintCaret(tester, canvas), paints..path());

      await tester.pumpAndSettle();
      expect(
        (Canvas canvas) => paintCaret(tester, canvas),
        paints..rect(rect: expectedCaretRect(tester)),
      );
    });

    testWidgets('hides the caret when the field loses focus', (tester) async {
      await pumpField(tester);
      await focusField(tester);
      controller.selection = const TextSelection.collapsed(offset: 3);
      await tester.pumpAndSettle();
      expect(
        (Canvas canvas) => paintCaret(tester, canvas),
        paints..rect(rect: expectedCaretRect(tester)),
      );

      await blurField(tester);
      expect((Canvas canvas) => paintCaret(tester, canvas), paintsNothing);
    });

    testWidgets('snaps instead of animating when motion is reduced', (
      tester,
    ) async {
      await pumpField(tester, reduceMotion: true);
      await focusField(tester);
      controller.selection = const TextSelection.collapsed(offset: 2);
      await tester.pumpAndSettle();

      controller.selection = const TextSelection.collapsed(offset: 4);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 16));
      expect(
        (Canvas canvas) => paintCaret(tester, canvas),
        paints..rect(rect: expectedCaretRect(tester)),
      );
    });

    testWidgets('rejects a subtree with more than one field', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: AnimatedCaret(
              controller: controller,
              child: Column(
                children: [
                  TextField(showCursor: false, controller: controller),
                  const TextField(showCursor: false),
                ],
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(tester.takeException(), isA<AssertionError>());
    });

    testWidgets('follows the field while it scrolls', (tester) async {
      controller.text = 'line\n' * 40;
      await pumpField(tester, minLines: 4, maxLines: 4);
      await focusField(tester);
      controller.selection = TextSelection.collapsed(
        offset: controller.text.length - 1,
      );
      await tester.pumpAndSettle();
      expect(
        (Canvas canvas) => paintCaret(tester, canvas),
        paints..rect(rect: expectedCaretRect(tester)),
      );

      // A scroll moves the caret geometry without touching the controller or
      // focus; the painted caret must follow it.
      final pointer = TestPointer(1, PointerDeviceKind.mouse);
      pointer.hover(tester.getCenter(find.byType(TextField)));
      await tester.sendEventToBinding(pointer.scroll(const Offset(0, -80)));
      await tester.pumpAndSettle();

      expect(
        fieldOf(tester).offset.pixels,
        greaterThan(0),
        reason: 'the field must actually scroll for this test to mean',
      );
      expect(
        (Canvas canvas) => paintCaret(tester, canvas),
        paints..rect(rect: expectedCaretRect(tester)),
      );
    });

    testWidgets('draws the caret on screen at the field caret position', (
      tester,
    ) async {
      // The caret color is asserted against the color configured at the call
      // site, not against the field's own cursor color: that is what the
      // widget reads, so it cannot also be the expectation.
      const caretColor = Color(0xFFFF00FF);
      await pumpField(tester, cursorColor: caretColor);
      await focusField(tester);
      controller.selection = const TextSelection.collapsed(offset: 5);
      await tester.pumpAndSettle();

      // Hiding the native caret leaves the field's cursor color transparent.
      expect(fieldOf(tester).cursorColor?.a, 0);
      expect(await _caretPixels(tester, caretColor), greaterThan(0));

      // The painted pixels cover the caret rectangle the field laid out
      // (edges are antialiased, so only roughly the full area matches).
      final rect = expectedCaretRect(tester);
      final overlay = tester.renderObject<RenderBox>(overlayFinder());
      final painted = await _caretPixels(
        tester,
        caretColor,
        within: rect.shift(overlay.localToGlobal(Offset.zero)),
      );
      // At least one full caret column is painted in the exact caret color
      // (the two edge columns land on fractional pixels and blend).
      expect(painted, greaterThanOrEqualTo(rect.height));

      await blurField(tester);
      expect(await _caretPixels(tester, caretColor), 0);
    });
  });
}
