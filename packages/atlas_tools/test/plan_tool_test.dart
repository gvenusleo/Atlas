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

  test('accepts an empty plan', () async {
    final result = await tool.execute(context, {'plan': []});

    expect(result.isError, isFalse);
    expect(result.content, 'Plan updated');
  });

  test('rejects a missing plan argument', () async {
    final result = await tool.execute(context, {});

    expect(result.isError, isTrue);
    expect(result.content, 'plan is required');

    final nullPlan = await tool.execute(context, {'plan': null});
    expect(nullPlan.isError, isTrue);
    expect(nullPlan.content, 'plan is required');
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

  test('rejects blank steps', () async {
    final result = await tool.execute(context, {
      'plan': [
        {'step': '   ', 'status': 'pending'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, 'plan step is required');
  });

  test('rejects steps that are too long', () async {
    final result = await tool.execute(context, {
      'plan': [
        {'step': 'x' * (maxPlanStepChars + 1), 'status': 'pending'},
      ],
    });

    expect(result.isError, isTrue);
    expect(
      result.content,
      'plan step must contain at most $maxPlanStepChars characters',
    );
  });

  test('rejects steps with control characters', () async {
    final result = await tool.execute(context, {
      'plan': [
        {'step': 'step\u0007', 'status': 'pending'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, 'plan step must not contain control characters');
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

  test('rejects non-object plan entries', () async {
    final result = await tool.execute(context, {
      'plan': ['not an object'],
    });

    expect(result.isError, isTrue);
    expect(result.content, 'plan step must be an object with step and status');
  });

  test('describes the tool for the model', () {
    final descriptor = tool.descriptor;
    expect(descriptor.name, 'plan');
    expect(
      descriptor.description,
      contains('Each call replaces the entire plan'),
    );
    final schema = descriptor.inputSchema;
    final plan = (schema['properties'] as Map)['plan'] as Map;
    expect(plan['maxItems'], maxPlanSteps);
    final item = plan['items'] as Map;
    final status = (item['properties'] as Map)['status'] as Map;
    expect(status['enum'], ['pending', 'in_progress', 'completed']);
  });
}
