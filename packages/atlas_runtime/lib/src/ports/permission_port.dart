import 'dart:async';

import '../domain/ids.dart';

/// A user decision for a pending permission request.
enum PermissionReply {
  /// Allow this single tool invocation.
  allowOnce,

  /// Allow this tool for the rest of the session.
  allowAlways,

  /// Reject this tool invocation.
  reject,
}

/// One selectable option offered by an agent for a permission request.
final class PermissionOption {
  /// Creates a permission option.
  const PermissionOption({
    required this.optionId,
    required this.kind,
    required this.name,
  });

  /// The option identifier sent back to the agent.
  final String optionId;

  /// The reply kind this option maps to.
  final PermissionReply kind;

  /// Human-readable option label.
  final String name;
}

/// A tool permission request raised by an agent, awaiting a user decision.
final class PermissionRequest {
  /// Creates a permission request.
  const PermissionRequest({
    required this.sessionId,
    required this.requestId,
    required this.toolCallId,
    required this.toolName,
    required this.title,
    required this.input,
    required this.options,
  });

  /// The session the tool call belongs to.
  final SessionId sessionId;

  /// Opaque correlation id used to respond to this request.
  final Object requestId;

  /// The tool call identifier reported by the agent.
  final String toolCallId;

  /// The tool name requesting permission.
  final String toolName;

  /// Human-readable description of the requested action.
  final String title;

  /// The raw tool arguments, for display.
  final Map<String, Object?> input;

  /// The reply options offered by the agent.
  final List<PermissionOption> options;
}

/// An optional runtime capability for surfacing agent permission requests.
///
/// Local runtimes execute tools directly and do not implement this port;
/// remote agents (such as ACP servers) raise requests that presentation code
/// must forward to the user before replying.
abstract interface class PermissionPort {
  /// Requests awaiting a user decision, in arrival order.
  Stream<PermissionRequest> get permissionRequests;

  /// Replies to [requestId] with [reply].
  Future<void> respondPermission(Object requestId, PermissionReply reply);
}
