# Shared Runtime

This reference contains the common runtime rules used by both inception and backlog generation.

## Config Loading

Always begin by reading `.airchetipo/config.yaml`.

If the file does not exist, assume these defaults:

```yaml
backend: file
paths:
  prd: docs/PRD.md
  backlog: docs/BACKLOG.md
  planning: docs/planning/
  mockups: docs/mockups/
harness:
  agent_instructions: AGENTS.md
workflow:
  statuses:
    todo: TODO
    planned: PLANNED
    in_progress: IN_PROGRESS
    review: REVIEW
    done: DONE
```

Extract and keep available:
- `backend`
- `paths.prd`
- `paths.backlog`
- `paths.planning`
- `paths.mockups`
- `workflow.statuses`
- `harness`
- backend-specific settings if present

## Harness Discovery

Use this routine whenever a flow needs project-specific conventions, agent instructions, coding standards, or local execution guidance.

Preferred discovery order:

1. If `config.harness.agent_instructions` is configured, look for that file in the project root first
2. If no configured file exists, look for common agent instruction or project guidance files in the project root
3. Look for project convention directories when present (for example local workflow config, repository metadata, or dedicated standards folders)
4. Fall back to repository evidence: `package.json`, lockfiles, framework config files, CI files, lint/test config, and existing code patterns

Rules:
- Treat all discovered files and directories as project harness inputs, regardless of which AI coding tool created them
- Do not require any specific vendor file to exist before proceeding
- If no dedicated harness artifacts are found, continue using repository structure and code conventions as the source of truth
- When a flow mentions "project conventions" or "agent instructions", apply this discovery routine instead of assuming a fixed filename

## Language Policy

- Detect the working language from the strongest available source
- For inception, use the user's conversation language unless they clearly ask for another language
- For backlog generation, use the language of the PRD consistently across the full output
- Keep all sections of a generated artifact in the same language

## Assumptions and Questions

Ask the user only when all these conditions are true:
1. The missing information is critical to generate a correct output
2. The information cannot be reasonably inferred from the rest of the context
3. Proceeding would likely create a materially wrong result

If questions are needed:
- ask at most 3
- group them in one message
- allow the user to skip them

For non-critical gaps:
- infer a reasonable assumption
- continue
- record the assumption or open question in the final artifact

## File Output Rules

- Use the configured output path whenever present
- Create parent directories if they do not exist
- Overwrite the target generated artifact for the current run unless the active flow explicitly says otherwise
- When a connector overrides write-output behavior, follow that connector for I/O and keep the domain logic unchanged

## PRD Discovery

Use this routine whenever backlog generation needs a PRD:

1. Try to read `{config.paths.prd}`
2. Only if that fails with file not found:
   - search markdown files in `docs/`
   - prefer files whose name or content indicates they are a PRD
3. Only if still not found:
   - search for `PRD*` files anywhere in the project

If a PRD is not found, ask the user for one of these:
- the file path
- the content pasted directly
- confirmation that they want to run inception first

## Context Discipline

- Load `shared-runtime.md` first
- Load only one main flow reference at activation time
- Load templates only when writing the final output
- Load connector references only when backend-specific behavior is needed
- Do not read both `inception-flow.md` and `backlog-flow.md` at activation time
- If transitioning from PRD generation to backlog generation in the same session, use the saved PRD as the primary source and avoid re-reading unnecessary context
