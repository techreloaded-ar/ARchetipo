---
name: airchetipo-spec
description: Creates the initial product backlog from a PRD, or appends new user stories to an existing one. Use whenever the user asks for a backlog, epics, user stories, or wants to add a single feature — even if the backlog does not yet exist.
---

# AIRchetipo - Spec Skill

You are the public entry point for AIRchetipo backlog and user-story work.

Your job is to understand whether the user needs to create the first backlog or extend an existing one, load only the references that matter for that case, and execute the correct flow without making the user choose between overlapping skills.

Treat routing as an internal implementation detail.

## Shared Runtime

Read `.airchetipo/shared-runtime.md` for Language Policy, Assumptions and Questions, Conversation Rules, and File Output Rules.

## Core Principle

Keep the working context lean:
- Load this file first
- Load exactly one main flow reference at activation time
- The connector is loaded once via contracts — no need for connector references

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

## Config Loading & Connector Dispatch

1. Read `.airchetipo/contracts.md` from the `.airchetipo/` directory. This loads the connector contracts and instructs you to read the active connector implementation file based on `config.yaml`.
2. Execute `SETUP: initialize_connector` from the loaded connector file.

Extract and keep available:
- `connector`
- `paths.prd`
- `paths.backlog`
- `paths.planning`
- `paths.mockups`
- `workflow.statuses`

## Backlog Discovery

Use this routine whenever the skill must decide whether it is extending an existing backlog or creating the first one.

Execute `READ: read_existing_backlog` from the connector. This operation:
- For `connector: file`: reads `{config.paths.backlog}` and searches for backlog files if not found at the configured path
- For other connectors: queries the connector service for existing backlog items

If existing stories are found, use them as the source of truth for backlog extension.
If none are found, treat the project as backlog-less and route to initial backlog creation.

**File connector fallback search** (only when `{config.paths.backlog}` is not found):
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

## Runtime Rules

- Ask clarifying questions only when critical information is missing and cannot be inferred responsibly
- Group clarifying questions in a single message when possible

## Output Boundaries

- Initial backlog creation belongs to this skill, not to `airchetipo-inception`

## Story Template

Use this shape for every story, in both initial backlog generation and story extension:

```markdown
#### US-XXX: [Concise action-oriented title]

**Epic:** EP-XXX | **Priority:** HIGH | **Story Points:** N | **Status:** {config.workflow.statuses.todo}
**Blocked by:** -

**Story**
As [persona name or role],
I want [specific action or capability],
so that [concrete benefit tied to a PRD goal].

**Demonstrates**
After implementing this story, [describe what can be concretely observed or verified — e.g. for a feature: "the user can open the reports page, click Export, and download a CSV"; for a foundational story: "a developer can run the test suite and see all checks pass"]

**Acceptance Criteria**
- [ ] [Primary happy path]
- [ ] [Validation or error case]
- [ ] [Relevant edge case]
```
