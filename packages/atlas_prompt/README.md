# atlas_prompt

System prompt construction for Atlas.

## Responsibility

- Builds the Atlas system prompt through `buildSystemPrompt`: the operating
  template, the available tool descriptors, and platform, shell, working
  directory, and current date context.
- Loads `~/.atlas/AGENTS.md` and the working-directory `AGENTS.md` through
  `loadInstructionFiles` and renders them into the prompt.
- Loads skills from the user-level `~/.agents/skills` and `~/.atlas/skills`
  roots, then the project `.agents/skills` / `.atlas/skills` roots under the
  working directory through `loadSkillCatalog` (pass `workingDirectory` so
  project skills resolve against the session cwd); later roots override
  earlier ones, so `.atlas/skills` wins over `.agents/skills` within a level
  and project skills win over user skills.
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