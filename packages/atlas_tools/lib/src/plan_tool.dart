import 'package:atlas_runtime/atlas_runtime.dart';

/// The maximum number of plan steps in one plan.
const maxPlanSteps = 50;

/// The maximum number of characters in one plan step.
const maxPlanStepChars = 500;

/// Manages the complete task plan for multi-step work.
///
/// Each call replaces the entire plan; the tool is stateless and the model
/// re-sends the full list on every update. The status of a step is one of
/// `pending`, `in_progress`, or `completed`, with at most one step
/// `in_progress` at a time.
final class PlanTool implements Tool {
  /// Matches Unicode control characters (Cc category), like Go's
  /// `unicode.IsControl`.
  static final RegExp _controlCharacters = RegExp(r'[\x00-\x1F\x7F-\x9F]');

  @override
  ToolDescriptor get descriptor => const ToolDescriptor(
    name: 'plan',
    description:
        'Update the complete task plan for multi-step work. '
        'Each call replaces the entire plan. '
        'Use pending, in_progress, or completed for status. '
        'Keep at most one step in_progress at a time. '
        'Skip for simple or single-step tasks.\n\n'
        'When to use:\n'
        '- Multi-step tasks that span several tool calls\n'
        '- Planning a sequence of edits before making them\n'
        '- After receiving new multi-step instructions, capture the '
        'requirements as plan steps\n'
        '- Before starting a tracked step, mark exactly one step as '
        'in_progress\n'
        '- Immediately after finishing a tracked step, mark it completed\n\n'
        'When NOT to use:\n'
        '- Single-shot answers that complete in one or two tool calls\n'
        '- Trivial requests where tracking adds no clarity\n\n'
        'Avoid churn:\n'
        '- Do not re-call this tool when nothing meaningful has changed '
        'since the last call\n'
        '- Update the plan only after real progress, not after every tool '
        'call\n'
        '- Keep steps short and actionable (e.g., "Read main.go", "Fix nil '
        'pointer in handler")',
    inputSchema: {
      'type': 'object',
      'properties': {
        'plan': {
          'type': 'array',
          'maxItems': maxPlanSteps,
          'items': {
            'type': 'object',
            'properties': {
              'step': {
                'type': 'string',
                'description': 'Task step text',
                'maxLength': maxPlanStepChars,
              },
              'status': {
                'type': 'string',
                'enum': ['pending', 'in_progress', 'completed'],
                'description': 'Current step status',
              },
            },
            'required': ['step', 'status'],
          },
          'description': 'The updated task plan',
        },
      },
      'required': ['plan'],
    },
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    final error = _validatePlan(arguments['plan']);
    if (error != null) {
      return ToolResult(content: error, isError: true);
    }
    return const ToolResult(content: 'Plan updated');
  }

  /// Validates the plan argument, returning a model-visible error message or
  /// `null` when the plan is well-formed.
  static String? _validatePlan(Object? raw) {
    if (raw is! List) {
      return 'plan is required';
    }
    if (raw.length > maxPlanSteps) {
      return 'plan must contain at most $maxPlanSteps steps';
    }
    var inProgress = 0;
    for (final item in raw) {
      if (item is! Map) {
        return 'plan step must be an object with step and status';
      }
      final step = item['step'];
      if (step is! String || step.trim().isEmpty) {
        return 'plan step is required';
      }
      if (step.runes.length > maxPlanStepChars) {
        return 'plan step must contain at most $maxPlanStepChars characters';
      }
      // Matches the Unicode control category, like Go's unicode.IsControl.
      if (_controlCharacters.hasMatch(step)) {
        return 'plan step must not contain control characters';
      }
      final status = item['status'];
      if (status != 'pending' &&
          status != 'in_progress' &&
          status != 'completed') {
        return 'invalid status "$status" for plan step "${step.trim()}"';
      }
      if (status == 'in_progress') {
        inProgress++;
        if (inProgress > 1) {
          return 'plan must contain at most one in_progress step';
        }
      }
    }
    return null;
  }
}
