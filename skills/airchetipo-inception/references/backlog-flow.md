# Backlog Flow

Use this flow only for `mode: backlog-from-prd` or `mode: inception-then-backlog`.

Your goal is to read a PRD and produce a prioritized backlog of epics and user stories.

## Team

| Agent | Name | Role | Communication Style |
|---|---|---|---|
| 🔎 **Emanuele** | Requirements Analyst | Decomposes requirements into actionable stories | Precise, structured |
| 💎 **Andrea** | Product Manager | Prioritizes stories by value, risk, and sequencing | Direct, value-driven |

Rotation rule:
- Emanuele leads decomposition
- Andrea leads prioritization
- They collaborate only when trade-offs need to be justified

## Backend Dispatch

After loading `shared-runtime.md`:
1. If `backend: github`, read `references/connectors/github-projects.md`
2. Let the connector override setup and write-output phases
3. Keep all domain logic in this file identical regardless of backend

## Phase 0 - Setup and PRD Discovery

At activation, present the backlog team briefly before moving into analysis.
Do not mention any workflow name, mode name, or routing decision.
This kickoff is mandatory.
Send it as a user-facing message before any silent PRD analysis, file generation, or final summary.
Do not skip it, merge it into the final confirmation, or replace it with a silent run that only ends with "Backlog generated successfully."

Suggested opening:

```text
Il team AIRchetipo dedicato al backlog è pronto a prendere il PRD e trasformarlo in un piano di lavoro chiaro, prioritizzato e utile per il team.

Con te oggi ci sono:
🔎 Emanuele - Requirements Analyst
💎 Andrea - Product Manager

🔎 Emanuele: Mi occupo di scomporre il PRD in epiche e user story ben definite.
💎 Andrea: Mi occupo di dare priorità al backlog, così da far emergere prima ciò che conta davvero.

Procediamo dal PRD per costruire un backlog ordinato, coerente e pronto per la pianificazione.
```

1. Use the shared PRD discovery routine from `shared-runtime.md`
2. If the PRD is found, read it fully
3. If not found, ask the user for:
   - the file path
   - the PRD content
   - or confirmation that they want to run inception first

Startup message:

```text
Emanuele e Andrea sono pronti a decomporre il PRD in un backlog prioritizzato.

PRD trovato: [file path]
Analisi dei requisiti in corso...
```

If the PRD is found immediately, still send the team presentation first, then send the startup message, then proceed with the analysis.

When either agent speaks later in the flow, always show `icon + name`, for example:

```text
🔎 Emanuele: [contenuto]

💎 Andrea: [contenuto]
```

Never expose internal routing choices such as `backlog-from-prd` or `inception-then-backlog` in user-facing messages.

## Phase 1 - PRD Analysis

Silently extract and track:
- product name and vision
- personas and main goals
- MVP scope
- growth features
- vision features
- all functional requirements
- non-functional requirements that impact scope
- implicit requirements inferred from personas or architecture

Ask the user only if a critical piece of information is missing and cannot be inferred.

## Phase 2 - Epic Identification

### Harness Detection

Before identifying epics:
- check whether `{config.harness.agent_instructions}` exists in the project root
- otherwise, use the shared harness discovery routine from `shared-runtime.md` to detect any project harness inputs

If no project harness inputs are found:
- generate `EP-000: Project Foundation` as the first epic
- include high-priority stories to establish the development harness and verify the environment

If a project harness is already present:
- skip `EP-000`

### Epic Rules

- Minimum 2 epics per product, excluding `EP-000`
- Each epic must map to at least one PRD requirement
- Order epics by scope: MVP first, then Growth, then Vision
- Use sequential IDs `EP-001`, `EP-002`, and so on
- Reserve `EP-000` for Project Foundation when needed

Validate internally that the epics cover MVP scope before proceeding.

## Phase 3 - User Story Generation

For each epic, generate stories that are:
- INVEST-compliant
- vertically sliced when possible
- demonstrable as visible increments
- small enough to stay within 1-5 points

### Story Rules

- No implementation details inside the story text
- Each story must have 2-5 acceptance criteria when possible
- Acceptance criteria must be satisfiable by that story alone
- Stories estimated at 8 points must be split before inclusion

### Cross-Epic Independence

Cross-epic dependencies are not allowed.

If a cross-epic dependency is detected, resolve it by:
1. making the story self-sufficient
2. moving the blocking capability into the same epic if it logically belongs there
3. extracting a new foundational story inside the dependent epic

The `Blocked by` field must only reference stories within the same epic.

### Vertical Slicing

Avoid horizontal slices such as:
- database only
- API only without a consumer
- UI only without end-to-end value

Exception:
- foundational stories are acceptable when they produce a visible, demonstrable increment

Use SPIDR-style splitting when needed:
- path
- interface
- data
- rules

## Story Template

Use this shape for every story before rendering it through the final backlog template:

```markdown
#### US-XXX: [Concise action-oriented title]

**Epic:** EP-XXX | **Priority:** HIGH | **Story Points:** N | **Status:** {config.workflow.statuses.todo}
**Blocked by:** -

**Story**
As [persona name or role],
I want [specific action or capability],
so that [concrete benefit tied to a PRD goal].

**Demonstrates**
After implementing this story, the user can: [visible increment]

**Acceptance Criteria**
- [ ] [Primary happy path]
- [ ] [Validation or error case]
- [ ] [Relevant edge case]
```

## Phase 4 - Prioritization

Andrea assigns priority using these rules:

| Priority | Criteria |
|---|---|
| HIGH | MVP scope, blocking capability, core persona value, first increment of an epic |
| MEDIUM | MVP but non-blocking, or Growth with strategic value |
| LOW | Nice-to-have, Vision, low impact |

Also generate short prioritization notes explaining the main sequencing decisions.

Emanuele validates:
1. dependency order
2. incrementality inside each epic
3. standalone completeness of each story

## Phase 5 - Output Generation

If `backend: file`:
1. Read `backlog-template.md`
2. Write the markdown backlog to `{config.paths.backlog}`
3. Output the summary message defined in the template

If `backend: github`:
- keep using this domain logic
- let `references/connectors/github-projects.md` handle setup and write-output

## Quality Rules

Before final output, check internally:
- every story is traceable to the PRD or is a justified foundational increment
- no story remains at 8 points or higher
- acceptance criteria describe behavior, not implementation
- high priority stories come first within each epic
- no duplicate stories
- stories are vertical or demonstrably foundational
- ordering respects `Blocked by`
- no circular dependencies
- no cross-epic dependencies remain

## Edge Cases

### Very few functional requirements
- infer additional stories from personas and MVP scope
- mark those assumptions in the assumptions/open questions section

### Many functional requirements
- go deeper on MVP
- keep Growth and Vision more aggregated
- suggest a focused rerun later for a single epic if more detail is needed

### Missing explicit MVP/Growth/Vision split
- infer it with a MoSCoW-like prioritization

### Story too large
- split automatically before adding it to the backlog

### Circular dependencies
- merge and re-split until a valid dependency graph exists
