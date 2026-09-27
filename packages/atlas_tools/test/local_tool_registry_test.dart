import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

import 'tool_test_utils.dart';

Tool _named(String name) => _FakeTool(name);

final class _FakeTool(final String name) implements Tool {
  @override
  ToolDescriptor get descriptor => ToolDescriptor(
    name: name,
    description: 'fake tool $name',
    inputSchema: const {'type': 'object'},
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async =>
      ToolResult(content: '$name executed');
}

void main() {
  test('returns an error for an unknown tool', () async {
    final dir = await tempDir();
    final registry = LocalToolRegistry([_named('read')]);

    final result = await registry.execute(
      toolContext(dir),
      ToolCall(id: ToolCallId('call-1'), name: 'nope', arguments: const {}),
    );

    expect(result.isError, isTrue);
    expect(result.content, contains('unknown tool'));
  });

  test('rejects duplicate tool names', () {
    expect(
      () => LocalToolRegistry([_named('read'), _named('read')]),
      throwsArgumentError,
    );
  });
}
