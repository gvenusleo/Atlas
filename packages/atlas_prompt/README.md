# atlas_prompt

System prompt construction for Atlas.

## Responsibility

- Builds the Atlas system prompt through `buildSystemPrompt`: the operating
  template, the available tool descriptors, and platform, shell, working
  directory, and current date context.
- Loads `~/.atlas/AGENTS.md` and the working-directory `AGENTS.md` through
  `loadInstructionFiles` and renders them into the prompt.

## Allowed dependencies

- `atlas_runtime` public types only (`ToolDescriptor`, `InstructionFile`).

## Prohibited ownership

- No model, provider, tool, storage, or orchestration logic.
- No provider-specific request mapping; the prompt is a plain string consumed
  by `atlas_runtime`'s `systemPromptBuilder`.
- No persistence of instruction files; loading is read-only at prompt build
  time.