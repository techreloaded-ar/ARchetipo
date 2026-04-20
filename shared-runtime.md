# AIRchetipo Shared Runtime

This file contains runtime rules shared by all AIRchetipo skills.
Load this file once at activation time, before loading any flow reference.

## Language Policy

Detect the output language from the strongest available source, in priority order:
1. Language of the backlog (if a backlog exists and is readable)
2. Language of the PRD (if no backlog is available)
3. Language of the user's current conversation

Apply the detected language to all user-facing output: messages, document section headers, error messages, and opening announcements.

Skill instructions (the text you are reading now) are always in English — they are internal, not user-facing. Templates and example text in skill files are structural guides; render them in the detected language when generating output.

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
