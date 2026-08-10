import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:nocterm/nocterm.dart';

import 'chat_controller.dart';
import 'input_bar.dart';
import 'message_list.dart';

/// The root Nocterm application for Atlas.
///
/// Owns a [ChatController] for the injected runtime and lays out the message
/// list above the input bar. It never constructs providers, tools, or storage.
final class AtlasTuiApp extends StatefulComponent {
  /// Creates the chat application.
  AtlasTuiApp({super.key, required this.runtime, this.workingDirectory});

  /// The runtime that executes turns.
  final AgentRuntime runtime;

  /// The working directory for tool execution, or the process directory.
  final String? workingDirectory;

  @override
  State<AtlasTuiApp> createState() => _AtlasTuiAppState();
}

final class _AtlasTuiAppState extends State<AtlasTuiApp> {
  late final ChatController _controller;
  late final TextEditingController _textController;

  @override
  void initState() {
    super.initState();
    _controller = ChatController(
      runtime: component.runtime,
      workingDirectory: component.workingDirectory ?? Directory.current.path,
    );
    _textController = TextEditingController();
    _controller.addListener(_refresh);
  }

  void _refresh() => setState(() {});

  void _submit(String text) {
    if (_controller.busy) {
      // Keep the draft in the input bar until the running turn finishes.
      return;
    }
    _textController.clear();
    _controller.send(text);
  }

  @override
  void dispose() {
    _controller.removeListener(_refresh);
    _textController.dispose();
    super.dispose();
  }

  @override
  Component build(BuildContext context) {
    return Column(
      children: [
        Expanded(child: MessageList(messages: _controller.messages)),
        const Divider(),
        InputBar(
          controller: _textController,
          busy: _controller.busy,
          onSubmitted: _submit,
        ),
      ],
    );
  }
}
