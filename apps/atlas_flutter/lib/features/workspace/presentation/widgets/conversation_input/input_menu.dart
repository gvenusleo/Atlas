/// The floating picker menus attached to the composer toolbar.
enum InputMenu { model, effort, mode }

/// Tracks which [InputMenu] is open and each menu's keyboard highlight.
///
/// Pure state holder: callers wrap mutations in [setState] and drive the
/// overlay portal themselves, keeping Flutter state in the owning widget.
final class InputMenuTracker {
  /// Creates a tracker with every menu closed.
  InputMenuTracker()
    : _highlights = {for (final menu in InputMenu.values) menu: 0};

  InputMenu? _open;
  final Map<InputMenu, int> _highlights;

  /// The currently open menu, or null when all are closed.
  InputMenu? get open => _open;

  /// Whether [menu] is the open menu.
  bool isOpen(InputMenu menu) => _open == menu;

  /// The keyboard highlight of [menu]; only meaningful while it is open.
  int highlight(InputMenu menu) => _highlights[menu] ?? 0;

  /// Opens [menu] (closing the others) or closes it when already open.
  ///
  /// Returns whether [menu] ended up open. A freshly opened menu starts at
  /// [initialHighlight], clamped to at least zero.
  bool toggle(InputMenu menu, {required int initialHighlight}) {
    if (_open == menu) {
      _open = null;
      return false;
    }
    _open = menu;
    _highlights[menu] = initialHighlight < 0 ? 0 : initialHighlight;
    return true;
  }

  /// Closes every menu.
  void close() {
    _open = null;
  }

  /// Moves the highlight of [menu] by [delta], wrapping within [count].
  void moveHighlight(InputMenu menu, int delta, int count) {
    if (count <= 0) {
      return;
    }
    _highlights[menu] = (_highlights[menu]! + delta + count) % count;
  }

  /// Pins the highlight of [menu], clamped to at least zero.
  void setHighlight(InputMenu menu, int index) {
    _highlights[menu] = index < 0 ? 0 : index;
  }
}
