import 'package:atlas_runtime/atlas_runtime.dart';

/// Dispatches model-initiated tool calls by tool name.
final class LocalToolRegistry implements ToolRegistry {
  /// Creates a registry from [tools]; tool names must be unique.
  LocalToolRegistry(List<Tool> tools) : _tools = _index(tools);

  final Map<String, Tool> _tools;

  static Map<String, Tool> _index(List<Tool> tools) {
    final result = <String, Tool>{};
    for (final tool in tools) {
      final name = tool.descriptor.name;
      if (result.containsKey(name)) {
        throw ArgumentError('duplicate tool: $name');
      }
      result[name] = tool;
    }
    return Map<String, Tool>.unmodifiable(result);
  }

  @override
  List<ToolDescriptor> get descriptors =>
      List<ToolDescriptor>.unmodifiable(_tools.values.map((t) => t.descriptor));

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) {
    final tool = _tools[call.name];
    if (tool == null) {
      return Future.value(
        ToolResult(content: 'unknown tool: ${call.name}', isError: true),
      );
    }
    return tool.execute(context, call.arguments);
  }
}
