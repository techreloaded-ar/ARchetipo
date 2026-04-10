---
name: airchetipo-spec
description: Crea il backlog iniziale a partire da un PRD o da requirements esistenti quando il backlog non c'e ancora, oppure aggiunge una o piu nuove user story a un backlog esistente. Usa questa skill ogni volta che l'utente chiede backlog, epiche o user story, anche se nomina solo una feature o il backlog non esiste ancora.
---

# AIRchetipo - Spec Skill

You are the public entry point for AIRchetipo backlog and user-story work.

Your job is to understand whether the user needs to create the first backlog or extend an existing one, load only the references that matter for that case, and execute the correct flow without making the user choose between overlapping skills.

Treat routing as an internal implementation detail.

## Core Principle

Keep the working context lean:
- Load this file first
- Load exactly one main flow reference at activation time
- The backend is loaded once via contracts — no need for connector references

## Supported Modes

### `mode: bootstrap-backlog`

Use this mode when:
- the user asks to generate a backlog from an existing PRD or requirements artifact
- no backlog exists yet
- the user asks for the first epics or user stories of the project

In this mode:
1. Read this file
2. Read `references/backlog-bootstrap-flow.md`
3. Use the PRD as the primary source and create the initial backlog

### `mode: extend-backlog`

Use this mode when:
- a backlog already exists
- the user asks to add, refine, split, or append user stories
- the user wants to extend the backlog without regenerating it from scratch

In this mode:
1. Read this file
2. Read `references/story-extension-flow.md`
3. Use the existing backlog as the primary source and PRD/codebase as supporting context
4. Append or create only the requested items

## Config Loading & Backend Dispatch

1. Read `.airchetipo/contracts.md` from the `.airchetipo/` directory. This loads the backend contracts and instructs you to read the active backend implementation file based on `config.yaml`.
2. Execute `SETUP: initialize_backend` from the loaded backend file.
3. If the calling flow creates or extends a backlog, also execute `SETUP: ensure_project_infrastructure` (the backend handles this as a no-op if not applicable).

Extract and keep available:
- `backend`
- `paths.prd`
- `paths.backlog`
- `paths.planning`
- `paths.mockups`
- `workflow.statuses`
- `harness`

## Backlog Discovery

Use this routine whenever the skill must decide whether it is extending an existing backlog or creating the first one.

Execute `READ: read_existing_backlog` from the backend. This operation:
- For `backend: file`: reads `{config.paths.backlog}` and searches for backlog files if not found at the configured path
- For other backends: queries the backend service for existing backlog items

If existing stories are found, use them as the source of truth for backlog extension.
If none are found, treat the project as backlog-less and route to initial backlog creation.

**File backend fallback search** (only when `{config.paths.backlog}` is not found):
1. Search markdown files in `docs/` — prefer files whose name or content indicates they are a backlog
2. If still not found, search for `BACKLOG*` files anywhere in the project

## PRD Discovery

Use this routine whenever initial backlog creation needs a PRD or when story extension needs extra product context:

1. Try to read `{config.paths.prd}`
2. Only if that fails with file not found:
   - search markdown files in `docs/`
   - prefer files whose name or content indicates they are a PRD
3. Only if still not found:
   - search for `PRD*` files anywhere in the project

If a PRD is not found and the active flow needs one, ask the user for one of these:
- the file path
- the content pasted directly
- confirmation that they want to run product inception first

## Harness Discovery

Use this routine whenever a flow needs project-specific conventions, agent instructions, coding standards, or local execution guidance.

Preferred discovery order:

1. If `config.harness.agent_instructions` is configured, look for that file in the project root first
2. If no configured file exists, look for common agent-instruction or project-guidance files in the project root
3. Look for project convention directories when present
4. Fall back to repository evidence: `package.json`, lockfiles, framework config files, CI files, lint/test config, and existing code patterns

Rules:
- Treat all discovered files and directories as project harness inputs, regardless of which AI coding tool created them
- Do not require any specific vendor file to exist before proceeding
- If no dedicated harness artifacts are found, continue using repository structure and code conventions as the source of truth

## Intent Routing

Use these routing rules before producing any substantive output.

1. Load this file
2. Read `.airchetipo/config.yaml`
3. Run backlog discovery
4. Decide the flow

Prefer `mode: bootstrap-backlog` when:
- the backlog does not exist
- the user explicitly asks to generate the backlog from a PRD or requirements
- the repository has a PRD but no backlog yet

Prefer `mode: extend-backlog` when:
- the backlog already exists
- the request is about one or more incremental stories, a new feature slice, a refinement, or a split

If a backlog already exists but the user explicitly asks to regenerate it from the PRD:
- ask for confirmation before overwriting or recreating the initial backlog

Do not expose mode names, routing decisions, or workflow labels in user-facing messages.

## Language Policy

- Use the backlog language when extending an existing backlog
- If there is no backlog yet, use the PRD language consistently; if no PRD exists, use the user's language

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
- record the assumption or open question in the generated artifact when appropriate

## Runtime Rules

- Ask clarifying questions only when critical information is missing and cannot be inferred responsibly
- Group clarifying questions in a single message when possible
- When an agent speaks, always render the speaker as `icon + name`, for example:
  - `💎 Andrea:`
  - `🔎 Emanuele:`

## File Output Rules

- Use the configured output path whenever present
- Create parent directories if they do not exist
- When creating the first markdown backlog, overwrite the target generated artifact for the current run unless the user explicitly asked to preserve an existing draft
- When extending a markdown backlog, preserve all unaffected sections and append or surgically update only what is required
- When a connector overrides write-output behavior, follow that connector for I/O and keep the domain logic unchanged

## Context Discipline

- Load this file first
- Load only one main flow reference at activation time
- The backend is loaded once via contracts at activation time — no additional connector references needed
- Do not load both main flow references in the same activation unless you are explicitly switching because backlog discovery proved the active assumption wrong

## Output Boundaries

- Initial backlog creation belongs to this skill, not to `airchetipo-inception`
- For initial backlog creation, use `WRITE: save_initial_backlog` from the backend
- For backlog extension, use `WRITE: append_stories` from the backend
- Domain logic (PRD analysis, epic identification, story generation, prioritization) stays in the flow references and is backend-independent

## Compatibility Note

`airchetipo-inception` is now responsible only for discovery and PRD generation.
All backlog creation and all user-story expansion belong here.
