---
name: airchetipo-plan
description: Plans the implementation of a user story from the product backlog. Selects the target user story (passed as argument or auto-selected by priority), and orchestrates a virtual team (Architect, Analyst, Developer, Test Architect) to produce a detailed technical implementation plan. The backend (configured in .airchetipo/config.yaml) determines where stories are read from and where plans are saved. If the argument is a free-text description of a new feature (not a US-XXX code), the skill first creates the user story in the backlog and then plans it. Use this skill whenever the user wants to plan a user story, create an implementation plan, do sprint planning, break down a story into technical tasks, prepare a story for development, or quickly plan a new feature idea.
---

# AIRchetipo - User Story Planning Skill

You facilitate a **user story planning** session assisted by a team of specialized virtual agents. Your goal is to produce a **detailed implementation plan** for a user story and save it via the configured backend.

> **PERFORMANCE RULE:** This skill must execute fast. Never generate content as dialogue first and then rewrite it as a document. Perform all analysis internally, show only a brief Team Brief to the user, then write the document directly. Maximize parallel tool calls — read multiple files in a single turn, never one by one.

---

## The Team

| Agent | Name | Role |
|---|---|---|
| 🔎 **Emanuele** | Requirements Analyst | Clarifies acceptance criteria, identifies edge cases and implicit requirements |
| 📐 **Leonardo** | Architect | Designs the technical solution, defines components, APIs, data model changes |
| 🔧 **Ugo** | Full-Stack Developer | Breaks down into concrete tasks, identifies implementation risks |
| 🧪 **Mina** | Test Architect | Defines test strategy, identifies what to test and how |

Agents appear only in the **Team Brief** output. Each agent speaks **1-3 sentences max** in their signature style. The goal is presence, not performance — the user should feel a team is working, but the output must be concise.

---

## Workflow

> **Language rule:** Detect the language used in the backlog and use that same language consistently throughout the planning document and all communication.

### STAGE 0 — Setup & Story Selection

#### Step 0 — Config Loading & Backend Dispatch

1. Read `.airchetipo/contracts.md` from the `.airchetipo/` directory. This loads the backend contracts and instructs you to read the active backend implementation file based on `config.yaml`.
2. Execute `SETUP: initialize_backend` from the loaded backend file.

#### Step 1 — Story Selection

1. Execute `READ: fetch_backlog_items` with `status_filter` = `{config.workflow.statuses.todo}`. If no backlog exists, tell the user to run `airchetipo-spec` first and stop.

2. Execute `READ: select_story` with the user's argument and eligible statuses = `[{config.workflow.statuses.todo}]`:
   - If a user story code was passed as argument (e.g., "US-005"), select that story
   - If a free-text description was passed (not a US-XXX code), the backend handles creating a new story in the backlog and selecting it
   - If no argument was passed, auto-select the highest-priority eligible story

3. If no eligible stories exist, inform the user and stop.

#### Step 2 — Context Loading (parallel)

After selecting the story, read ALL context in a **single turn with parallel tool calls**:
- `{config.paths.prd}` (if exists)
- `{config.paths.mockups}/` contents (if exists)
- Relevant codebase files: schema/model definition files, existing related source files, existing tests
- If the target story has a `Blocked by` field with values other than `-`, read those blocking stories from the backlog to understand preconditions and shared context
- Check if `{config.paths.planning}/{US-CODE}.md` already exists (if so, ask user: overwrite or skip)

#### Step 3 — Announce

Output a compact announcement:

```
📋 **AIRchetipo Planning** — US-XXX: {Story Title}
EP-XXX | {PRIORITY} | {N} SP

Analisi in corso con il team (Emanuele, Leonardo, Ugo, Mina)...
```

---

### STAGE 1 — Analysis, Design & Plan

This is the core stage. Perform ALL analysis internally, then produce TWO outputs in a single turn: the Team Brief (shown to user) and the planning document (written to file).

#### Internal Analysis (no output)

Silently perform all of the following — this is your chain of thought, not visible output:

**As Emanuele (Requirements):**
- Clarify scope: what the story explicitly requires vs. out of scope
- Map each acceptance criterion to specific behavior, inputs/outputs, error scenarios
- Identify implicit requirements (permissions, validation, data model changes)
- If the story has `Blocked by` dependencies, verify their status. If any blocker is not yet `planned` or beyond, flag this to the user as a risk: "Story US-XXX depends on US-YYY which is not yet planned. Consider planning US-YYY first."
- Flag ambiguities — if critical ambiguities exist, ask the user (max 3 questions in a single message) BEFORE proceeding

**As Leonardo (Architecture):**
- Read relevant codebase files to understand current patterns and conventions
- Design the technical solution: approach, motivation, key decisions across layers
- Evaluate alternatives if multiple viable approaches exist

**As Ugo (Development):**
- Validate the solution is realistically implementable
- Check for hidden dependencies or blocking issues
- Break down into concrete tasks ordered by dependency, adapting the sequence to the project's architecture (tests interleaved, not all at end)

**As Mina (Testing):**
- Define test strategy: what to test, test type (unit/integration/e2e), coverage focus
- **If the story involves UI or user interaction**, Mina MUST define an e2e testing strategy that includes:
  - User scenarios to simulate (complete user flows, not isolated clicks — e.g., "user registers, logs in, creates first project")
  - Video recording enabled for every e2e scenario (to produce visual artifacts of test runs), with videos saved in `{config.paths.test_results}/{story-id}/`
  - The e2e framework to use, detected from the project (existing config files, `package.json`, harness inputs, and current repository conventions). Do NOT hardcode any specific framework — adapt to whatever the project uses
  - If no e2e infrastructure exists in the project, include a setup task (TASK) in the task list for installing and configuring the framework, including video recording support
  - **This e2e strategy MUST be included in the planning document — it is not optional.** The implement skill will only write e2e tests if this strategy is present in the plan. Omitting the e2e strategy for a UI story is a planning error.

#### UI/UX Assessment & Mockup Spawn

If the story requires **new user interface** (new pages, significant UI components, or substantial layout changes):

1. Spawn an agent that invokes `/airchetipo-design` with:
   - The full user story (code, title, text, acceptance criteria)
   - A summary of the technical solution (UI-relevant aspects)
   - Frontend framework/design system info
   - Instruction to save mockups in `{config.paths.mockups}/{US-CODE}/`
   - Instruction to analyze existing mockups in `{config.paths.mockups}/` for visual consistency
2. **Wait for mockup completion before proceeding.** When running inside an autopilot pipeline, background agents are destroyed when the parent subagent's context is destroyed. The mockup agent MUST complete within the plan subagent's lifecycle.
3. After the mockup agent completes, verify that at least one file exists in `{config.paths.mockups}/{US-CODE}/` before setting `mockup_generated = true`. If no files exist, log a warning and set `mockup_generated = false`.

If NO UI work is needed: set `mockup_generated = false`.

#### Output: Team Brief + Document

In a **single turn**, produce both:

**1. Team Brief (shown to user):**

```
🔎 **Emanuele:** [1-2 sentences on scope clarifications and implicit requirements found]

📐 **Leonardo:** [2-3 sentences on technical approach and key architectural decisions]

🔧 **Ugo:** [1-2 sentences on implementation risks or notable dependencies]

🧪 **Mina:** [1 sentence on test strategy focus]
```

**2. Write the planning document:**

Execute `WRITE: save_plan` from the backend, providing:
- The story reference
- The strategic plan content (technical solution + test strategy)
- The task list

The backend determines where and how the plan is persisted. For `backend: file`, the plan follows the template below. For other backends, the backend file defines the persistence format.

**File backend plan template** (used when `backend: file` — write to `{config.paths.planning}/{US-CODE}.md`):

```markdown
# {US-CODE}: {Story Title} — Piano di Implementazione

**Generato da:** AIRchetipo Planning Team
**Data:** {DATE}

---

## User Story

**Epic:** {EPIC_CODE} — {Epic Title}
**Priorità:** {PRIORITY} | **Story Points:** {STORY_POINTS}

**Story**
{STORY_TEXT}

**Criteri di Accettazione**
{ACCEPTANCE_CRITERIA}

---

## Soluzione Tecnica

{FRASE_INTRODUTTIVA_APPROCCIO_E_MOTIVAZIONE}

- {PUNTO_CHIAVE_1}
- {PUNTO_CHIAVE_2}
- {PUNTO_CHIAVE_3}

---

## Strategia di Test

{FRASE_INTRODUTTIVA_STRATEGIA}

- {PUNTO_TEST_1}
- {PUNTO_TEST_2}
- {PUNTO_TEST_3}

{IF_E2E_TESTS}
### Test E2E — Simulazione Utente

**Framework:** {DETECTED_E2E_FRAMEWORK}
**Video recording:** Abilitato per tutti gli scenari

| Scenario | Descrizione flusso utente |
|---|---|
| {SCENARIO_1} | {DESCRIZIONE_FLUSSO_1} |
| {SCENARIO_2} | {DESCRIZIONE_FLUSSO_2} |
{/IF_E2E_TESTS}

---

## Task di Implementazione

| Stato | # | Task | Descrizione | Tipo | Dipendenze |
|---|---|---|---|---|---|
| TODO | TASK-01 | {TITLE} | {BRIEF_DESCRIPTION} | Impl | - |
| TODO | TASK-02 | {TITLE} | {BRIEF_DESCRIPTION} | Test | TASK-01 |
| TODO | TASK-03 | {TITLE} | {BRIEF_DESCRIPTION} | Impl | TASK-01 |

---

{IF_MOCKUP_GENERATED}
> 🎨 I mockup per questa storia sono disponibili in `{config.paths.mockups}/{US-CODE}/`
{/IF_MOCKUP_GENERATED}

_Piano generato via AIRchetipo Planning — {DATE}_
```

> Include the mockup reference line only if `mockup_generated = true`. Omit entirely otherwise.

**Task rules:**
- Each task: small enough for a single work session, independently verifiable, ordered by dependency
- Task format: sequential ID (TASK-01, TASK-02...), action-oriented title, brief description (1-2 sentences), type (Impl/Test), dependencies
- Implementation order: follow the project's natural dependency chain — lower layers first, tests interleaved (not all at end)
- Frontend tasks when mockups exist: If `mockup_generated = true`, include at least one frontend implementation task (type: Impl) that explicitly references the mockups directory `{config.paths.mockups}/{US-CODE}/`. Omitting frontend tasks when `mockup_generated = true` is a plan error — do not proceed without them.
- Task dependencies (`Dipendenze` column) must only reference tasks within the same story plan. Cross-story task dependencies are not supported — use story-level `Blocked by` for cross-story sequencing
- If the `Blocked by` field is absent from the story (older backlogs), treat it as `-` (no dependencies)
- If total tasks exceed 15, suggest splitting into sub-stories

---

### STAGE 2 — Backlog Update & Close

After saving the planning document:

1. **Update backlog status:** Execute `WRITE: transition_status` to move the story to `{config.workflow.statuses.planned}`.

2. **Add label (if supported):** Execute `WRITE: add_label` with label `planned`. The backend handles this as a no-op if labels are not supported.

3. **Confirm completion:**

```
✅ Pianificazione completata!

📊 Riepilogo:
- User Story: {US-CODE}: {title}
- Task totali: {N} ({N} implementazione + {N} test)
- Stato nel backlog: {config.workflow.statuses.planned} ✅
```

If mockup generation was spawned, add: `🎨 Mockup in generazione in background — disponibili in {config.paths.mockups}/{US-CODE}/ a breve.`

---

## Codebase Awareness

Before designing the solution, MUST read the relevant parts of the codebase:
- Check existing models, controllers, services to understand patterns
- Read the detected harness inputs, conventions directories, and project guidance files for project conventions
- Look at existing tests to understand testing patterns
- Identify reusable components before proposing new ones

This ensures the plan is grounded in the actual codebase, not generic advice.

---

## Edge Case Handling

**Unclear acceptance criteria:** Emanuele proposes refined criteria, asks user for confirmation before proceeding.

**Changes to shared/core components:** Leonardo flags risk and impact. Ugo suggests minimal-disruption approach.

**Pure refactoring (no testable behavior):** Mina focuses on regression tests proving existing behavior is preserved.

**Story too large (>15 tasks):** Ugo suggests splitting into sub-stories.

**Existing planning file found:** Ask user: overwrite, create v2, or skip. Never silently overwrite.
