import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:mcp_dart/mcp_dart.dart' as mcp;

/// Limits text at a UTF-8 boundary, including its truncation marker.
({String text, int bytes, bool truncated}) boundedText(
  String text,
  int limit, {
  String marker = '\n[MCP output truncated]',
}) {
  final bytes = utf8.encode(text);
  if (bytes.length <= limit) {
    return (text: text, bytes: bytes.length, truncated: false);
  }
  var end = limit - utf8.encode(marker).length;
  while (end > 0 && (bytes[end] & 0xc0) == 0x80) {
    end--;
  }
  return (
    text: '${utf8.decode(bytes.sublist(0, end))}$marker',
    bytes: bytes.length,
    truncated: true,
  );
}

/// Converts MCP content into Atlas's bounded text-only tool result contract.
ToolResult convertResult(
  mcp.CallToolResult result,
  String server,
  String tool,
) {
  final parts = <String>[];
  var supported = result.hasStructuredContent;
  var unsupported = false;
  for (final content in result.content) {
    switch (content) {
      case mcp.TextContent(:final text):
        parts.add(text);
        supported = true;
      case mcp.ResourceLink(:final uri):
        parts.add(uri);
        supported = true;
      case mcp.EmbeddedResource(
        resource: mcp.TextResourceContents(:final uri, :final text),
      ):
        parts.add('$uri\n$text');
        supported = true;
      case mcp.ImageContent() ||
          mcp.AudioContent() ||
          mcp.EmbeddedResource() ||
          mcp.UnknownContent():
        parts.add('[Unsupported MCP ${content.type} content omitted]');
        unsupported = true;
    }
  }
  if (result.hasStructuredContent) {
    parts.add(jsonEncode(result.structuredContentJson!.toJson()));
  }
  final metadata = <String, Object?>{
    'mcp_server': boundedText(server, 512).text,
    'mcp_tool': boundedText(tool, 512).text,
  };
  final unsupportedOnly = unsupported && !supported && !result.isError;
  if (unsupportedOnly) {
    parts.add('Atlas currently supports text and JSON MCP tool results.');
    metadata['failure_kind'] = 'unsupported_content';
  }
  // Reserve metadata space within the aggregate 50 KiB persisted result budget.
  final budget = 50 * 1024 - utf8.encode(jsonEncode(metadata)).length - 256;
  final output = boundedText(parts.join('\n'), budget);
  metadata['truncated'] = output.truncated;
  metadata['total_bytes'] = output.bytes;
  return ToolResult(
    content: output.text,
    isError: result.isError || unsupportedOnly,
    metadata: metadata,
  );
}
