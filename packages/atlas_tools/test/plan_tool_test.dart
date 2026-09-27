import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

import 'tool_test_utils.dart';

void main() {
  final tool = PlanTool();
  late ToolContext context;

  setUpAll(() async {
    context = toolContext(await tempDir());
  });

  test('accepts a full plan and reports the update', () async {
    final result = await tool.execute(context, {
      'plan': [
        {'step': 'Read main.go', 'status': 'in_progress'},
        {'step': 'Fix nil pointer in handler', 'status': 'pending'},
        {'step': 'Run tests', 'status': 'completed'},
      ],
    });

    expect(result.isError, isFalse);
    expect(result.content, 'Plan updated');
  });

  test('rejects plans with too many steps', () async {
    final result = await tool.execute(context, {
      'plan': [
        for (var i = 0; i < maxPlanSteps + 1; i++)
          {'step': 'step $i', 'status': 'pending'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, 'plan must contain at most $maxPlanSteps steps');
  });

  test('rejects invalid status values', () async {
    final result = await tool.execute(context, {
      'plan': [
        {'step': 'step one', 'status': 'done'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, 'invalid status "done" for plan step "step one"');
  });

  test('rejects more than one in_progress step', () async {
    final result = await tool.execute(context, {
      'plan': [
        {'step': 'first', 'status': 'in_progress'},
        {'step': 'second', 'status': 'in_progress'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, 'plan must contain at most one in_progress step');
  });
}
