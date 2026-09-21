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
final class const PermissionOption({
  /// The option identifier sent back to the agent.
  required final String optionId,

  /// The reply kind this option maps to.
  required final PermissionReply kind,

  /// Human-readable option label.
  required final String name,
}) {
  /// Creates a permission option.
  this;
}

/// A tool permission request raised by an agent, awaiting a user decision.
final class const PermissionRequest({
  /// The session the tool call belongs to.
  required final SessionId sessionId,

  /// Opaque correlation id used to respond to this request.
  required final Object requestId,

  /// The tool call identifier reported by the agent.
  required final String toolCallId,

  /// The tool name requesting permission.
  required final String toolName,

  /// Human-readable description of the requested action.
  required final String title,

  /// The raw tool arguments, for display.
  required final Map<String, Object?> input,

  /// The reply options offered by the agent.
  required final List<PermissionOption> options,
}) {
  /// Creates a permission request.
  this;
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
