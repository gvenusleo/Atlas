import 'package:atlas_flutter/features/workspace/application/workspace_state.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final model = ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('model')),
  );
  final first = SessionWorkspace(
    workingDirectory: '/tmp',
    activeModel: model,
    sessionId: SessionId('first'),
  );
  final second = SessionWorkspace(
    workingDirectory: '/tmp',
    activeModel: model,
    sessionId: SessionId('second'),
  );
  final state = WorkspaceState(
    activeKey: 'first',
    workspaces: {'first': first, 'second': second},
    sessions: const [],
  );

  test('copies share the collections that did not change', () {
    // A streaming turn rewrites one session while the others stay untouched.
    final after = state.copyWith(
      workspaces: {'first': first.copyWith(busy: true), 'second': second},
    );

    expect(after.runningSessionIds, {SessionId('first')});
    expect(
      identical(after.workspaceKeys, state.workspaceKeys),
      isTrue,
      reason: 'unchanged cache keys keep their instance',
    );
    expect(
      identical(after.sessions, state.sessions),
      isTrue,
      reason: 'an untouched list keeps its instance',
    );
    expect(
      identical(after.completedSessionIds, state.completedSessionIds),
      isTrue,
      reason: 'unchanged activity flags keep their instance',
    );
  });

  test('copies replace the collections whose content changed', () {
    final after = state.copyWith(
      workspaces: {'first': first, 'second': second.copyWith(busy: true)},
    );

    expect(after.runningSessionIds, {SessionId('second')});
    expect(
      identical(after.runningSessionIds, state.runningSessionIds),
      isFalse,
    );

    final dropped = after.copyWith(workspaces: {'first': first});
    expect(dropped.workspaceKeys, ['first']);
    expect(identical(dropped.workspaceKeys, after.workspaceKeys), isFalse);
  });
}
