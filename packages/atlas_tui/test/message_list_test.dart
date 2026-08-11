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
        expect('${tester.terminalState}', isNot(contains('message 0')));
      });
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
