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
