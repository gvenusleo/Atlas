import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  group('MessageList', () {
    test('pins the newest message at the bottom of the viewport', () async {
      await testNocterm('message list pins to bottom', (tester) async {
        final messages = [
          for (var i = 0; i < 30; i++)
            ChatMessage(kind: ChatMessageKind.assistant, text: 'message $i'),
        ];
        await tester.pumpComponent(
          SizedBox(height: 10, child: MessageList(messages: messages)),
        );

        // The newest message is visible; the oldest is scrolled off the top.
        expect(tester.terminalState, containsText('message 29'));
        expect(tester.terminalState.getText(), isNot(contains('message 0')));
      });
    });

    test(
      'keeps the bullet at column 0 when a reply arrives after a user message',
      () async {
        await testNocterm('bullet stays at column 0', (tester) async {
          final messages = <ChatMessage>[
            ChatMessage(kind: ChatMessageKind.user, text: '你好？'),
          ];
          final holder = _MessageListHolder(messages);
          await tester.pumpComponent(
            SizedBox(
              width: 60,
              height: 10,
              child: _RebuildableMessageList(holder: holder),
            ),
          );
          await tester.pump();

          // A new reply arrives: the list now grows at the tail, and the
          // reversed ListView must re-render the new top row in place.
          holder.messages.add(
            ChatMessage(
              kind: ChatMessageKind.reasoning,
              text: '用户用中文打招呼，需要直接用中文回复。',
            ),
          );
          holder.notify();
          await tester.pump();

          final state = tester.terminalState;
          // Locate the reply row by its content, then verify its bullet
          // starts at column 0 rather than drifting to the line end.
          final match = state
              .findText('用户用中文打招呼')
              .firstWhere((hit) => hit.x == 2);
          expect(state, hasTextAt(0, match.y, '•'));
        });
      },
    );
  });

  group('tailLines', () {
    test('keeps short text unchanged', () {
      expect(tailLines('a\nb', 3), 'a\nb');
      expect(tailLines('', 3), '');
    });

    test('keeps the trailing lines and elides the head', () {
      expect(tailLines('1\n2\n3\n4\n5', 3), '...\n4\n5');
      expect(tailLines('1\n2\n3\n4', 3), '...\n3\n4');
    });

    test('keeps exactly maxLines when the text has that many lines', () {
      expect(tailLines('1\n2\n3', 3), '1\n2\n3');
    });

    test('ignores a trailing newline in the line count', () {
      expect(tailLines('1\n2\n3\n4\n5\n', 3), '...\n4\n5');
      expect(tailLines('a\nb\n', 3), 'a\nb');
    });
  });

  group('headWindow', () {
    test('keeps the full text when it fits', () {
      expect(headWindow('short', 10), 'short');
    });

    test('drops the tail, keeps the head, and marks the elision', () {
      expect(headWindow('abcdef', 5), 'ab...');
    });

    test('counts wide characters as two columns', () {
      // 窗口 7 列 = 4 列内容（2 个汉字）+ 3 列省略号。
      expect(headWindow('汉字汉字', 7), '汉字...');
    });

    test('keeps whole code points from the head', () {
      // 😀 是代理对；窗口 6 列 = 3 列内容 + 3 列省略号，头部保留了
      // 完整的 😀 + b，而非被拆坏的半个代理对。
      expect(headWindow('😀b😀c😀', 6), '😀b...');
    });

    test('handles empty text and zero width', () {
      expect(headWindow('', 10), '');
      expect(headWindow('abc', 0), '');
    });
  });

  group('tailWindow', () {
    test('keeps the full text when it fits', () {
      expect(tailWindow('short', 10), 'short');
    });

    test('drops the head, keeps the tail, and marks the elision', () {
      expect(tailWindow('abcdef', 5), '...ef');
    });

    test('counts wide characters as two columns', () {
      // 四个汉字 = 8 列；窗口 7 列 = 省略号 3 列 + 4 列内容（2 个汉字）。
      expect(tailWindow('汉字汉字', 7), '...汉字');
    });

    test('keeps whole code points from the tail', () {
      // 😀 是代理对；窗口 6 列 = 省略号 3 列 + 3 列内容，尾部保留了
      // 完整的 c + 😀，而非被拆坏的半个代理对。
      expect(tailWindow('a😀b😀c', 6), '...😀c');
    });

    test('handles empty text and zero width', () {
      expect(tailWindow('', 10), '');
      expect(tailWindow('abc', 0), '');
    });
  });
}

/// Mutable message list holder that rebuilds its listener on every change.
class _MessageListHolder {
  _MessageListHolder(this.messages);

  final List<ChatMessage> messages;
  void Function()? _listener;

  void notify() => _listener?.call();
}

/// Rebuilds a [MessageList] whenever the holder changes, mirroring how the
/// app streams new content into the transcript across frames.
class _RebuildableMessageList extends StatefulComponent {
  const _RebuildableMessageList({required this.holder});

  final _MessageListHolder holder;

  @override
  State<_RebuildableMessageList> createState() =>
      _RebuildableMessageListState();
}

class _RebuildableMessageListState extends State<_RebuildableMessageList> {
  @override
  void initState() {
    super.initState();
    component.holder._listener = () => setState(() {});
  }

  @override
  Component build(BuildContext context) {
    return MessageList(messages: List.of(component.holder.messages));
  }
}
