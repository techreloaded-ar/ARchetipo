# AIRchetipo Shared Runtime

This file contains runtime rules shared by all AIRchetipo skills.
Load this file once at activation time, before loading any flow reference.

## Language Policy

Detect the output language from the strongest available source, in priority order:
1. Language of the backlog (if a backlog exists and is readable)
2. Language of the PRD (if no backlog is available)
3. Language of the user's current conversation

Apply the detected language to all user-facing output: messages, document section headers, error messages, and opening announcements.

Templates and example text in skill files are structural guides; render them in the detected language when generating output.


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

## Conversation Rules

- Each agent speaks in character
- Never mention internal mode names, workflow names, or routing decisions in the conversation

## Agent Persona

When an agent speaks, always render the speaker as `icon + name`, for example:

```text
💎 Andrea: [content]

🧭 Costanza: [content]
```

This rule applies to any skill that defines named agents with personas.

## File Output Rules

- Use the configured output path whenever present
- Create parent directories if they do not exist
- Overwrite the target generated artifact for the current run unless the active flow explicitly says otherwise
- When a connector overrides write-output behavior, follow that connector for I/O and keep the domain logic unchanged
