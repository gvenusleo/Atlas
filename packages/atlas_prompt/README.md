# atlas_prompt

System prompt construction for Atlas.

## Responsibility

- Builds the Atlas system prompt through `buildSystemPrompt`: the operating
  template, the available tool descriptors, and platform, shell, working
  directory, and current date context.
- Loads `~/.atlas/AGENTS.md` and the working-directory `AGENTS.md` through
  `loadInstructionFiles` and renders them into the prompt.
- Loads skills from `~/.atlas/skills`, `~/.agents/skills`, and the project
  `.atlas/skills` / `.agents/skills` directories through `loadSkillCatalog`
  and exposes them as a `SkillCatalog`.

## Allowed dependencies

- `atlas_runtime` public types only (`ToolDescriptor`, `InstructionFile`,
  `SkillSummary`, `SkillCatalog`, `SessionContext`).

## Prohibited ownership

- No model, provider, tool, storage, or orchestration logic.
- No provider-specific request mapping; the prompt is a plain string consumed
  by `atlas_runtime`'s `systemPromptBuilder`.
- No persistence of instruction files; loading is read-only at prompt build
  time.