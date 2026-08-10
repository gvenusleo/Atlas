import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  final tools = [
    const ToolDescriptor(
      name: 'read',
      description: 'Read a file.',
      inputSchema: {'type': 'object'},
    ),
    const ToolDescriptor(
      name: 'shell',
      description: 'Run a command.',
      inputSchema: {'type': 'object'},
    ),
  ];

  test('renders the dynamic context into the header', () {
    final prompt = buildSystemPrompt(
      workingDirectory: '/work/project',
      tools: tools,
      platform: 'macos',
      shell: '/bin/sh',
      now: DateTime.utc(2026, 8, 10),
    );

    expect(prompt, contains('Operating context: 2026-08-10'));
    expect(prompt, contains('Platform: macos'));
    expect(prompt, contains('Default shell: /bin/sh'));
    expect(prompt, contains('Working directory: /work/project'));
  });

  test('lists available tools when tools are provided', () {
    final prompt = buildSystemPrompt(
      workingDirectory: '/work',
      tools: tools,
      platform: 'macos',
      shell: '/bin/sh',
    );

    expect(prompt, contains('Available tools: read, shell.'));
  });

  test('omits the tool list when no tools are provided', () {
    final prompt = buildSystemPrompt(
      workingDirectory: '/work',
      tools: const [],
      platform: 'macos',
      shell: '/bin/sh',
    );

    expect(prompt, isNot(contains('Available tools:')));
  });

  test('injects loaded instruction files with a wrapper', () {
    final prompt = buildSystemPrompt(
      workingDirectory: '/work',
      tools: tools,
      instructions: [
        const InstructionFile(
          path: '/home/u/.atlas/AGENTS.md',
          content: 'be concise',
        ),
        const InstructionFile(path: '/work/AGENTS.md', content: 'dart only'),
      ],
      platform: 'macos',
      shell: '/bin/sh',
    );

    expect(prompt, contains('## Loaded Instructions'));
    expect(
      prompt,
      contains('<instruction_file path="/home/u/.atlas/AGENTS.md">'),
    );
    expect(prompt, contains('be concise'));
    expect(prompt, contains('dart only'));
  });

  test('omits the instructions section when none are loaded', () {
    final prompt = buildSystemPrompt(
      workingDirectory: '/work',
      tools: tools,
      platform: 'macos',
      shell: '/bin/sh',
    );

    expect(prompt, isNot(contains('Loaded Instructions')));
  });

  test('falls back to unknown working directory markers', () {
    final prompt = buildSystemPrompt(
      workingDirectory: '',
      tools: const [],
      platform: 'macos',
      shell: '/bin/sh',
    );

    expect(prompt, contains('Working directory: (unknown)'));
  });
}
