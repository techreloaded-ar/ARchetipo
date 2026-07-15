---
name: archetipo-plan
description: Creates a detailed technical implementation plan for a spec. Use this skill whenever the user wants to plan a spec, break down a feature into technical tasks, create an implementation plan, do sprint planning, prepare a spec for development or estimate a feature. Also triggers on requests like "plan this", "break this down", "what are the tasks for this spec", or "how would we build this". The spec can be passed by code (e.g., US-005) or as a free-text description — the skill handles both automatically.
---

## Subagents capability

This skill uses isolated subagents for optimal context management.
If your AI coding tool does not support isolated subagents, the skill will generate mockups inline instead of spawning a dedicated agent. Planning output quality is unchanged.

# ARchetipo - Spec Planning Skill

You facilitate a **spec planning** session assisted by a team of specialized virtual agents. Your goal is to produce a **detailed implementation plan** for a spec (whose body is a user story) and save it via the configured connector.

> **PERFORMANCE RULE:** This skill must execute fast. Never generate content as dialogue first and then rewrite it as a document. Perform all analysis internally, show only a brief Team Brief to the user, then write the document directly. Maximize parallel tool calls — read multiple files in a single turn, never one by one.

---

## Shared Runtime

Read `.archetipo/shared-runtime.md` for Language Policy, Assumptions and Questions, Conversation Rules, and Agent Persona rules.

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

> **Language:** Use the detected language from `.archetipo/shared-runtime.md` throughout the planning document and all communication.

### STAGE 0 — Setup & Spec Selection

#### Step 0 — Config Loading & Connector Dispatch

1. Run `archetipo config show` and parse the stdout JSON envelope; keep the `data` (SetupInfo) available. Treat `data.project_root` as the cwd for all ARchetipo connector/backlog commands in this skill.
2. On failure, parse stderr as the JSON error envelope and branch on `error.code`.
3. This skill uses only these CLI operations:
   - `archetipo config show`
   - `archetipo spec show {US-CODE}`
   - `archetipo spec next --status {config.workflow.statuses.todo}`
   - `archetipo validate plan {US-CODE} --file <path>`
   - `archetipo spec plan {US-CODE} --file <path>`
   - `archetipo wiki search [query]`

#### Step 1 — Spec Selection

Pick one of the two mutually exclusive forms:

- If a spec code was passed (e.g. "US-005"): `archetipo spec show US-005`
- If no argument was passed: `archetipo spec next --status {config.workflow.statuses.todo}` (auto-select first eligible by priority + code)

Free-text descriptions are not supported as spec selectors. If the user passes free text, route to `archetipo-spec` to add the spec first.

The envelope returns `data: {spec, tasks}`. If a plan already exists `data.tasks` is populated — see Step 2 below for the overwrite handling.

If `error.code = E_PRECONDITION` (no eligible specs) or `error.code = E_NOT_FOUND` (spec code not in the backlog), inform the user and stop.

#### Step 2 — Context Loading (parallel)

After selecting the spec, read ALL context in a **single turn with parallel tool calls**:

- `{config.paths.prd}` (if exists)
- `{config.paths.mockups}/` contents (if exists)
- Relevant codebase files: schema/model definition files, existing related source files, existing tests
- `{config.paths.wiki}/index.md` and only the pages selected from the spec's `Wiki context` IDs or `archetipo wiki search`
- If the target spec has a `Blocked by` field with values other than `-`, read those blocking specs from the backlog to understand preconditions and shared context
- If `data.tasks` from Step 1 was non-empty, a plan already exists. In **Rework mode** (see below) do NOT ask — preserve the existing tasks and append. Otherwise ask the user: overwrite, create a new revision, or skip. Never silently overwrite.

**Worktree awareness.** Apply the **Worktree Working Directory** rule from `.archetipo/shared-runtime.md`: run `config show`, `spec show`/`next`, and `spec plan` from `data.project_root`, but do ALL codebase reading and analysis (including the Rework Feedback `file:line` lookups) under `data.workdir` returned by the `spec show`/`next` call in Step 1. That directory is the spec's worktree when one exists — holding the changes already made for this spec, so the plan reflects the real current state — and the project root otherwise. Branch only on `data.workdir`, never on connector type.

**Rework mode.** A spec is "in rework" when `data.spec.rework` is `true` or `data.spec.body` contains a `## Rework Feedback` section. It means the spec was sent back from review via *request changes*, with the reviewer's inline comments recorded as bullets (each anchored to a `file:line`). In this mode the feedback is the primary planning input — see the task-construction rule in STAGE 1.

#### Baseline and verification discovery

Before designing tasks, discover the checks the repository actually trusts from agent instructions, CI configuration, and project scripts. Run the smallest relevant baseline commands under `data.workdir` and record the command plus result in `plan_body` under a `Baseline` section.

- Include the focused tests for the behavior being changed and the compiler/type-checker, linter, build, or schema check that can catch defects in the files likely to be touched.
- Never write `Expect: passes` for a command that already fails. If a baseline command fails, record the exact pre-existing failure boundary and design verification that proves the task adds no failures.
- A file changed by the plan must not be left with compiler or linter diagnostics. If that file already has local diagnostics, either include their bounded cleanup in the task or choose a different seam; do not silently add more diagnostics to a degraded file.
- Detect environment prerequisites separately from code failures. Name deterministic setup commands when available, but do not make the implementer rediscover package-manager, generated-client, database, or service requirements.

#### Step 3 — Announce

Output a compact announcement:

```
📋 **ARchetipo Planning** — {US-CODE}: {Spec Title}
{EP-CODE} | {PRIORITY} | {N} SP

[Detected language: brief status message that analysis is starting with the team]
```

---

### STAGE 1 — Analysis, Design & Plan

This is the core stage. Perform ALL analysis internally, then produce TWO outputs in a single turn: the Team Brief (shown to user) and the planning document (written to file).

#### Internal Analysis (no output)

Silently perform all of the following — this is your chain of thought, not visible output:

**As Emanuele (Requirements):**

- Clarify scope: what the spec explicitly requires vs. out of scope
- Map each acceptance criterion to specific behavior, inputs/outputs, error scenarios
- Identify implicit requirements (permissions, validation, data model changes)
- If the spec has `Blocked by` dependencies, verify their status. If any blocker is not yet `planned` or beyond, flag this to the user as a risk: "Spec US-XXX depends on US-YYY which is not yet planned. Consider planning US-YYY first."
- Flag ambiguities — if critical ambiguities exist, ask the user (max 3 questions in a single message) BEFORE proceeding

**As Leonardo (Architecture):**

- Read relevant codebase files to understand current patterns and conventions
- Design the technical solution: approach, motivation, key decisions across layers
- Evaluate alternatives if multiple viable approaches exist
- Evaluate whether the solution crosses the ADR threshold below. Search existing pages with `archetipo wiki search --type decision` before declaring a new decision ID.

**As Ugo (Development):**

- Validate the solution is realistically implementable
- Check for hidden dependencies or blocking issues
- Break down into concrete tasks ordered by dependency, adapting the sequence to the project's architecture (tests interleaved, not all at end)
- For every task, write `body` as an execution contract for smaller implementation models. Use the exact canonical headings defined under **Task execution contract**; do not substitute the older `Descrizione` / `File Coinvolti` / `Criteri di Completamento` shape.
- Add one final `Test` gate task that depends on every implementation and acceptance-test task. It runs the exact focused checks plus the relevant regression/build/type/lint checks discovered in the baseline; the implementer must not infer the final suite from project conventions. Its `Done` section must also require a post-completion `archetipo spec show {US-CODE}` check from `data.project_root` proving every task is `DONE` before `archetipo spec review` is allowed.

**As Mina (Testing):**

- Define test strategy: what to test, test type (unit/integration/e2e), coverage focus
- Build an acceptance evidence map before creating tasks. For every `AC-N`, name the test layer, exact observable oracle/assertion, fixture or starting state, and task that will produce the evidence. Include this map in `plan_body`; it is an acceptance map, not a duplicate task summary.
- Test the specified outcome, not a proxy. Absence of cards does not prove that an empty-state message is visible; a successful mock call does not prove the returned user contract. If a test double hides the component or boundary responsible for an acceptance outcome, either remove that mock for the scenario or add a direct test of the real responsible component.
- **If the spec involves UI or user interaction**, classify browser e2e risk explicitly:
  - Browser e2e is required when the acceptance path crosses a route/server boundary, authentication or authorization, persistence, external integration, navigation across screens, or browser-only behavior; also use it when lower layers cannot prove the observable outcome.
  - A local interaction within one already-tested client component may use deterministic component integration tests instead. Record the browser-e2e waiver in `plan_body`, including why the chosen layer proves every affected `AC-N`; do not bootstrap a browser framework solely for a local state/filter/presentation change.
  - When browser e2e is required, define complete user flows, detect and reuse the repository's framework, and include an idempotent setup task if infrastructure is absent — for Playwright, use `archetipo e2e ensure`.
  - Note whether `Demonstrates` is filmable. Planning flags the expectation only; `archetipo-review` owns video recording and the final record/skip decision.

#### Architectural Decision Record threshold

Create or update an ADR when the plan chooses between at least two viable alternatives and the choice materially affects one or more of: persistent data model, security model, integration boundary, deployment topology, consistency or failure semantics, or a cross-cutting technical policy used by multiple capabilities. Routine implementation details, a local bug fix, following an existing established pattern, or a reversible refactor do not warrant an ADR.

For every qualifying choice:

1. Search existing `type: decision` pages and reuse the stable ID when the decision already exists. Put that ID in `wiki_impact.update_after_acceptance` when the choice clarifies, extends, or supersedes it.
2. Otherwise choose a specific `decisions.<slug>` ID and put it in `wiki_impact.create`. Never omit the page because the Wiki previously lacked an ADR type: `type: decision` is part of the Wiki contract.
3. Include context, chosen option, viable alternatives, tradeoffs, and verification intent in the technical solution. Add an implementation task whose completion criteria require the generated ADR page, its repository evidence, and all decision section markers.
4. If a new choice replaces an existing ADR, update the old page to `decision_status: superseded`, link it to the new accepted decision, and include both IDs in the Wiki impact contract.

If no choice crosses this threshold, keep decision IDs out of `wiki_impact.create` and state the reason briefly in the technical solution. Do not manufacture ADRs merely to grow the Wiki.

#### UI/UX Assessment & Mockup Spawn

Decide whether the spec needs mockups using these explicit triggers. The spec needs mockups when **at least one** holds:

- It introduces a **new page, screen, or route** that does not exist yet
- It introduces a **new user-facing component** with its own layout (form, wizard, dashboard widget, modal flow — not a single field or button added to an existing form)
- It **restructures the layout** of an existing page (sections added/removed/rearranged), as opposed to changing copy, colors, or styling of what is already there

The spec does NOT need mockups when it only: changes text/labels, adds a field to an existing form, tweaks styling within the current layout, or has no user-facing surface at all. When in doubt between "new component" and "small addition", prefer no mockup and note the call in the Team Brief so the user can override.

If the spec requires mockups per the triggers above:

**If subagent/worker support is available:**

1. Spawn an agent that invokes `/archetipo-design` with:
   - The full spec (code, title, user-story body, acceptance criteria)
   - A summary of the technical solution (UI-relevant aspects)
   - Frontend framework/design system info
   - Instruction to save mockups in `{config.paths.mockups}/{US-CODE}/`
   - Instruction to analyze existing mockups in `{config.paths.mockups}/` for visual consistency
2. **Wait for mockup completion before proceeding.** When running inside an autopilot pipeline, background agents are destroyed when the parent subagent's context is destroyed. The mockup agent MUST complete within the plan subagent's lifecycle.
3. After the mockup agent completes, verify that at least one file exists in `{config.paths.mockups}/{US-CODE}/` before setting `mockup_generated = true`. If no files exist, log a warning and set `mockup_generated = false`.

**If subagent/worker support is NOT available:**

1. Load `skills/archetipo-design/SKILL.md` and apply its workflow inline — design rules, aesthetic guidelines, and output constraints live there and must not be duplicated here.
2. Save mockup files to `{config.paths.mockups}/{US-CODE}/` as instructed by the design skill.
3. After generation, verify at least one file exists: set `mockup_generated = true` on success, or `mockup_generated = false` with a warning if the directory is empty.

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

**2. Save the plan and transition the spec:**

Construct the full JSON payload string in your own context (not via shell heredoc or inline script). Choose a unique temp filename using the spec code (e.g. `tmp-payload-US-005-plan.json`). Write the file to `.archetipo/` under `data.project_root` using your file-writing tool. Then invoke `archetipo validate plan {US-CODE} --file <path>` from `data.project_root` before saving the plan.

If validation returns `kind: "validation_result"` with `data.ok: false`, do not call `archetipo spec plan`. Read `data.findings`, repair every `severity: "error"` in the payload, and rerun validation. Treat warnings as quality feedback; fix them when straightforward, but they do not block persistence.

Only after validation passes, invoke `archetipo spec plan {US-CODE} --file <path>` from `data.project_root`. After the CLI exits, delete the temp file.

> **⚠️ Cross-platform warning:** Do NOT pipe the JSON through stdin via shell (`--file -` with shell pipe). Shell pipes are OS-dependent and can corrupt JSON that contains markdown with special characters (`` ` ``, `$`, `{`, line breaks, Unicode). Use your file-writing tool to write the JSON file first, then pass its path to `--file`.
>
> **Temp file:** Use `.archetipo/tmp-payload-{US-CODE}-plan.json`. The code is known to you already. After the CLI command exits, delete it with `rm .archetipo/tmp-payload-{US-CODE}-plan.json` (works in both bash and PowerShell). Always clean up, regardless of CLI success or failure.

```json
{"plan_body":"<technical solution + baseline + acceptance evidence map + test strategy as markdown — do NOT include a task summary>","tasks":[{"id":"TASK-01","title":"...","body":"## Objective\n<one outcome>\n\n## Read\n- path/to/file — symbol or behavior to inspect\n\n## Change\n- path/to/file — exact allowed change\n\n## Steps\n1. <ordered action>\n\n## Verify\n- Run: `<exact command>`\n- Expect: <observable result or explicit no-new-failures boundary from baseline>\n\n## Done\n- [ ] AC-1 — <acceptance-linked criterion>\n\n## Blockers\nNone.","type":"Impl|Test","status":"TODO","dependencies":[]}]}
```

> **Payload field contracts:** `plan_body` contains ONLY the technical solution, baseline results, acceptance evidence map, test strategy, and context notes as markdown. The task list lives exclusively in the `tasks` array — do NOT duplicate it inside `plan_body` (no task summary table or bullet list). `status` uses the CLI's canonical values (`TODO`, `DONE`) — these are part of the envelope contract and are **not** the display labels from `config.workflow.statuses`. `type` is one of `Impl`, `Test`, or `Fix` (Fix only in rework mode). `dependencies` lists ids of tasks defined in the same payload; the CLI rejects references to unknown task ids. Each task must use `body` as the only produced content field and follow the complete contract below. Use concrete file paths when they are known; when they are not, stay conservative and do not invent files.

#### Task execution contract

Use these seven headings literally and in this order in every `task.body`. Keep the headings in English as stable machine-readable labels; write their content in the detected project language.

1. `## Objective` — one observable outcome for this task.
2. `## Read` — exact existing paths plus the symbols, tests, or behavior to inspect before editing. State `None — new file` only when appropriate.
3. `## Change` — exact paths and allowed modifications, including important non-goals that prevent scope drift.
4. `## Steps` — ordered, atomic actions with all local technical decisions already made. Do not leave architecture choices to the implementer.
5. `## Verify` — exact runnable command(s), the working directory when non-obvious, and the expected evidence. Prefer a focused check plus the smallest relevant regression suite. The expectation must agree with the recorded baseline; never promise green for a known-red command.
6. `## Done` — checklist items tied to the task outcome and named spec acceptance ids (`AC-N`). Test tasks state the actual observable oracle, not only that a test file exists or a mock was called.
7. `## Blockers` — prerequisites or decisions that genuinely block execution; write `None.` when there are none.

Before validation, audit every task body against all seven headings. Missing sections are a planning defect, including on `Test` and `Fix` tasks. Ensure dependencies reference earlier tasks and that a smaller implementer can execute each task without rediscovering scope, paths, commands, or intended behavior. Also verify that every `AC-N` appears in the acceptance evidence map and in at least one task's `Done`, every observable oracle remains visible through the chosen test doubles, and the last task is a final `Test` gate depending on all work it verifies. The gate must state the workflow order explicitly: run checks, mark the gate task `DONE`, reload the spec, confirm no task remains `TODO`, then and only then request review.

**Rework mode task construction.** When the spec is in rework (see Step 2), build the `tasks` array like this instead of planning from scratch:

- **Preserve every existing task** from `data.tasks` with its current `status` (tasks already `DONE` stay `DONE`). The payload replaces the whole task list, so omitting them would lose history.
- For **each bullet** in the `## Rework Feedback` section, read the referenced `file:line` **under `data.workdir`** (see Worktree awareness in Step 2) to understand the real code, then append one task with `"type":"Fix"`, `"status":"TODO"`, a concrete `title`, and a `body` that follows the complete seven-heading task execution contract, states what to change and why, and references the reviewer's comment and anchor. Continue the existing `TASK-NN` numbering.
- Add interleaved `Test` tasks for the fixes when the change warrants verification.
- Append a new final `Test` gate for the rework even if the original plan already has a completed gate. It must verify the fix tasks, rerun the acceptance evidence affected by feedback, check that modified files add no compiler/linter diagnostics relative to the recorded baseline, and prove through `archetipo spec show` that all preserved and appended tasks are `DONE` before review.
- Set `plan_body` to the existing plan body augmented with a short "Rework" note summarising the feedback being addressed; do not discard the original technical solution.

This single command saves the plan AND transitions the spec to `{config.workflow.statuses.planned}` atomically (and clears the rework marker) — no separate `status set` step is needed. The CLI persists according to the active connector (file: writes `{paths.planning}/{US-CODE}-plan.yaml`; github: appends to the parent issue body and creates one sub-issue per task). For the file connector, follow the template in `./references/plan-template.md` to compose `plan_body` (technical solution + baseline + acceptance evidence map + test strategy — no task summary table).

Re-running the command on a spec already in `PLANNED` upserts the plan body without erroring.

### STAGE 2 — Close

After saving the plan:

1. **Confirm completion:**

```
[Detected language: adapt this block]
✅ Planning complete!

📊 Summary:
- Spec: {US-CODE}: {title}
- Total tasks: {N} ({N} implementation + {N} test)
- Backlog status: {config.workflow.statuses.planned} ✅
```

If mockup generation was spawned, add: `🎨 Mockups generating in background — available in {config.paths.mockups}/{US-CODE}/ shortly.`

---

## Edge Case Handling

**Unclear acceptance criteria:** Emanuele proposes refined criteria, asks user for confirmation before proceeding.

**Changes to shared/core components:** Leonardo flags risk and impact. Ugo suggests minimal-disruption approach.

**Pure refactoring (no testable behavior):** Mina focuses on regression tests proving existing behavior is preserved.

**Spec too large (>15 tasks):** Ugo suggests splitting into sub-specs.

**Existing planning file found:** Ask user: overwrite, create v2, or skip. Never silently overwrite.
