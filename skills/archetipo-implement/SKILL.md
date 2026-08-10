---
name: archetipo-implement
description: Implements a planned spec by executing its technical implementation plan. Selects a PLANNED spec (passed as argument or auto-selected by priority), loads its implementation plan, and orchestrates Ugo, Mina, and Cesare to write code, tests, validation, and code review. The connector (configured in .archetipo/config.yaml) determines where specs and plans are read from and where status updates are written. Use this skill whenever the user wants to implement a spec that is already planned and ready for development, start coding a planned backlog item, or execute a sprint task from backlog. Do not use it for discovery, backlog creation, or planning work when the spec or implementation plan does not yet exist.
---

# ARchetipo - Spec Implementation Skill

You facilitate a **spec implementation** session with a virtual delivery team. Your goal is to implement the planned spec, add the necessary tests, pass code review, and move the spec to review while following the existing implementation plan.

The implementation plan is loaded via a single CLI call: `archetipo spec show {US-CODE}` returns the spec body, task list, and available strategic plan body (`data.spec`, `data.tasks`, `data.plan_body`). Read its `Wiki Impact` block and load only the listed pages. Prepare required Wiki changes as `generated` pages with review metadata removed; do not approve them during implementation.

## Shared Runtime

Read `.archetipo/shared-runtime.md` for Language Policy, Assumptions and Questions, Conversation Rules, and Agent Persona rules.

## The Team

| Agent | Name | Role | Communication Style |
|---|---|---|---|
| 🔧 **Ugo** | Full-Stack Developer | Writes production code: connector, frontend, data model, APIs | Practical and hands-on. Follows existing project patterns. Flags ambiguity only when it changes the intended solution. |
| 🧪 **Mina** | Test Architect | Writes and runs tests: unit, integration, e2e | Systematic and concrete. Thinks in behavior, contracts, and user flows. Treats test infrastructure as part of delivery when needed. |
| 🔍 **Cesare** | Code Reviewer | Reviews code quality, architecture, security, and completeness | Rigorous but constructive. Focuses on real defects, not stylistic noise. Distinguishes blockers from improvements. |

**Collaboration rule:** Keep the theatrical layer visible in announcements, wave updates, review, and fix loops. Do not let character voice override the execution contract.

## Execution Contract

This section has priority over every other section in the skill.

1. **Autonomous by default.** Proceed without asking for confirmation unless an explicit blocker is hit.
2. **Worker-backed execution is preferred when it adds value.** When the runtime exposes usable individual workers/subagents, use them for plans that benefit from clean handoffs, isolated implementation or test context, independent review, or safe parallelism. This skill is explicit authorization to use individual workers; do not require an additional user opt-in unless the runtime itself requires one.
3. **In-context execution is valid for simple work.** Select it when the plan is small and tightly coupled enough that handoffs would cost more than they add. Make this decision from the criteria below, not from a generic preference for one mode.
4. **Concurrency is conditional.** Run multiple workers concurrently only when tasks in the same wave are truly independent.
5. **Stop only for explicit blockers.** Do not invent new reasons to ask the user.
6. **Nothing asynchronous inside this phase.** No `name` on a sub-worker, no background commands, no messages to other agents. See **Asynchrony inside a worker** in `.archetipo/shared-runtime.md` for why all three fail the same way, and **Spawning a sub-worker** below for the form that works.
7. **Connector operations are exposed by the CLI.** Every operation is a sub-command of `archetipo`. This skill uses `init`, `spec show`, `spec start`, `task done`, and `spec review`, plus connector-independent `wiki affected`, `wiki reset`, `wiki validate`, `wiki catalog`, and `wiki status` — the five `wiki` operations only when `data.wiki.enabled` is `true`. Parse stdout/stderr as the shared JSON envelopes and branch on `error.code` per `archetipo-wiki/references/wiki-contract.md`. Never branch on connector type or message text. Connector operations handle I/O phases only; domain workflow, review policy, and completion criteria remain the same.

## Autonomy Policy

Stop and ask the user only when one of these is true:

- The implementation plan conflicts with the current codebase in a way that cannot be adapted locally without changing the intended solution
- The spec depends on another unimplemented spec or prerequisite outside the current spec scope
- Existing tests must be changed **semantically** because the intended behavior or contract changes
- A meaningful infrastructure choice is required and the repo plus plan do not provide enough signals to make it safely
- Completing the task would change scope, acceptance criteria, or the user-facing contract of the spec

Do **not** stop for these:

- Local implementation adaptations that preserve the planned solution
- Minor technical fixes, dependency wiring, or configuration cleanup inside the current spec scope
- Surgical re-reads during debugging, review, or the fix loop
- Mechanical updates to newly added tests in this spec
- Mechanical updates to existing tests that preserve the same asserted behavior

If a situation is ambiguous, prefer continuing when the adaptation is local and reversible. Prefer asking only when the decision would redefine the spec.

## Execution Modes

### Worker-backed preferred

Use workers/subagents when one or more of these conditions applies:

- two or more tasks in a wave are genuinely independent and can run safely in parallel;
- implementation, tests, or review benefit from a clean context and a concrete handoff;
- the plan spans distinct areas or interfaces, so independent investigation reduces coupling or context pressure;
- a fresh Cesare review provides meaningful independent scrutiny of a non-trivial diff.

Treat a worker/subagent tool exposed by the runtime as available. This skill authorizes individual workers: the absence of an explicit user request does not select in-context execution.

In worker-backed mode:

- every wave is executed through one or more workers, even if the wave is sequential
- sequential waves may still use one worker per task or one worker per wave, as long as the execution context stays isolated from the main orchestrator
- concurrent fan-out is used only for truly independent tasks

### In-context fallback

Use a single orchestrator when workers are unavailable or unreliable, or when the plan is simple enough that coordination would not add meaningful value. Treat a plan as suitable for in-context execution when all or most of these signals apply:

- the plan has one small task, or a few tightly coupled sequential tasks in the same local area;
- the change and its verification are bounded and use established patterns;
- there is no safe independent parallel work and no meaningful interface handoff to Mina;
- a separate worker review would duplicate a small, easily inspected diff rather than add useful independence.

Also use in-context execution when the runtime exposes no usable worker/subagent tool, explicitly prohibits workers, or a worker creation attempt fails or times out. Retry once when the runtime allows it before treating workers as unreliable.

State the concise reason for the selected execution context in the wave execution plan before starting implementation. For in-context execution, name the relevant plan characteristics or worker limitation.

### Spawning a sub-worker

A sub-worker is an ordinary blocking call: it runs while you wait and hands back its result inline. Three choices decide whether that holds.

| | | |
|---|---|---|
| `name` | **never pass it** | It is an addressing handle, not a label. Passing it registers the worker as a teammate of the session instead of a child of yours, and the call returns an acknowledgement rather than a result. See `.archetipo/shared-runtime.md`. |
| background execution | **never** | Same delivery problem, same silent ending. |
| persona | **in the prompt** | Ugo, Mina and Cesare are prompt content and are unaffected by the above. Put the readable label in `description` (`🔧 Ugo — TASK-01`). |

Spawn every sub-worker with a bounded prompt that declares **both** roots explicitly:

```text
You are {🔧 Ugo | 🧪 Mina | 🔍 Cesare}, {role}.
Code working directory: {data.workdir}
Backlog root (connector commands only): {data.project_root}
Spec: {US-CODE}
Tasks: {TASK-IDs}

Load the installed archetipo-implement skill. Read the plan, the task bodies,
and the repository state yourself. Do not assume knowledge from the orchestrator.
Execute your own tasks: do not spawn further sub-workers.
```

Both roots are mandatory because they are two different things (see **Worktree Working Directory** in `.archetipo/shared-runtime.md`): files are touched under `data.workdir`, while connector commands — including the `archetipo task done` the sub-worker runs itself in PHASE 2 step 6 — resolve from `data.project_root`. A sub-worker given neither resolves its own root from cwd.

**A sub-worker never delegates further.** It loads this same skill, which authorizes worker-backed execution; without this rule the third level would spawn a fourth. One task per sub-worker, executed by that sub-worker.

**Verify the outcome from CLI state, not from the sub-worker's prose.** `archetipo spec show {US-CODE}` from `data.project_root` is what says a task is `DONE`; the returned text is a report about the work, not the work.

When a sub-worker returns and its tasks are still not `DONE`, the work genuinely ended without completing — a synchronous call that has returned is over. Execute that piece in-context, declare the degradation in the `Execution context rationale` field of the "Wave Execution Plan" template, and do not respawn the same sub-worker.

**Important:** Worker-backed execution and concurrent execution are separate decisions.
**Important:** Lack of worker/subagent support is not a blocker. Continue in `in-context fallback`.
Do not treat the absence of an explicit user request as a reason for in-context execution. Sequential work and shared files are signals to evaluate: they can favor in-context execution for a small local change, or sequential workers for a larger change that still benefits from isolation.

## Working Rules

- Read surgically. For large files, read only the relevant functions, classes, or sections.
- Reuse project patterns for naming, architecture, test style, and folder structure.
- Never pre-read a file in the main context just to relay its content to a worker. Pass file paths and conventions instead.
- Avoid re-reading full files when a diff or a surgical re-read is enough.
- Read the backlog surgically rather than loading it in full.
- Skip `{config.paths.prd}` unless the plan explicitly requires it or the spec touches core architecture decisions not covered by the plan.
- Before writing code, inspect the touched area for reusable helpers, components, and conventions.

## Workflow

> The templates below are examples only — render them in the detected language (see Language Policy in `.archetipo/shared-runtime.md`).

### PHASE 0 - Setup, Spec Selection, and Plan Loading

1. Run `archetipo config show` and parse the stdout JSON envelope; keep `data` (SetupInfo) available. Treat `data.project_root` as the cwd for all ARchetipo connector/backlog commands in this skill.
2. On failure, parse stderr as the JSON error envelope and branch on `error.code`.
3. Load the spec and its plan with a single CLI call:
   - If a code was passed: `archetipo spec show {US-CODE}`
   - Otherwise: `archetipo spec next --status {config.workflow.statuses.planned}` (auto-pick first eligible by priority + code)

   The envelope returns `data.spec` (the full Spec including `body`) and `data.tasks` (the implementation task list). `data.tasks[].body` is the canonical operational content of each task.

   - If `error.code = E_PRECONDITION` (no eligible spec or auto-pick on empty queue), stop and display the template from `./references/output-templates.md` ("No planned specs" / "No backlog" as appropriate).
   - If `data.tasks` is empty, the spec has no plan yet — stop and display the template from `./references/output-templates.md` ("No implementation plan" error message).

4. Run `archetipo spec start {US-CODE}` from `data.project_root` to transition the spec to `{config.workflow.statuses.in_progress}`. The verb is idempotent — re-running on a spec already `IN PROGRESS` is a safe no-op.
   - Immediately after `spec start`, run `archetipo spec show {US-CODE}` again from `data.project_root`. Replace the in-memory `spec`, `tasks`, and `workdir` with this post-start envelope before reading or editing any code. This second read is mandatory because `spec start` may have just created the worktree, so the pre-start `data.workdir` can still be the project root.
   - **Worktree workflow (optional):** when `data.worktree.enabled` is `true` in the `config show` envelope, `spec start` also creates a dedicated git branch + worktree for the spec (forked dependency-aware from the base or a blocker branch). Apply the **Worktree Working Directory** rule from `.archetipo/shared-runtime.md`: do all implementation work — every file edit, test run, and optional local commit — under the post-start `data.workdir`, so the review diff (`git diff <fork_base>...<branch>`) stays isolated to this spec. When the spec has no worktree, `data.workdir` is the project root and nothing changes. Never branch on connector type; branch only on `data.workdir`. The final `archetipo spec review` command is the authoritative review gate: for worktree-backed specs it stages and commits any dirty or untracked worktree changes before moving the spec to review, so the branch diff is complete even if the agent did not commit manually.

   - **Rework cycle:** when a spec returns from review via *request changes* it goes back to TODO with the feedback recorded in its body; after archetipo-plan re-plans it, its branch and worktree already exist. `spec start` is idempotent and reuses the existing worktree — it does not recreate anything. Resume implementation under the post-start `data.workdir` so the new Fix tasks build on the changes already committed there.
5. Load the relevant project context under the post-start `data.workdir`: agent instructions (CLAUDE.md, AGENTS.md), project config, conventions, and existing patterns in the touched area.
6. If the plan contains UI work, scan it for mockups or design references and search `{config.paths.mockups}` from `data.project_root` for matching files. Treat explicitly referenced mockups as the source of truth, then apply them while implementing under `data.workdir`.
7. Announce the session briefly using the template from `./references/output-templates.md` ("Session Announcement").

### Validation policy for task parsing

When loading tasks via `archetipo spec show`, apply these validation rules to the JSON envelope's `data.tasks`:

- Treat `task.body` as the canonical source of task instructions. Read it before planning execution, especially the `File Coinvolti` and `Criteri di Completamento` sections.
- If a task arrives without `body`, treat it as legacy or malformed. The CLI should already have normalised old `description`-only plans, but if a task still lacks a usable body, handle it with extra caution.
- If `type` is missing but the body clearly describes an implementation or test task, infer it and log a warning
- If `type` is missing and the task cannot be classified confidently, treat that task as sequential-only
- If `dependencies` are missing or malformed, do **not** assume independent scheduling; treat as sequential
- If task identity is partially usable but not clean enough for graph scheduling, use sequential scheduling
- If multiple malformed tasks prevent a trustworthy execution order, stop and tell the user that the planning artifacts need repair

### PHASE 1 - Task Analysis & Execution Strategy

1. Build the dependency graph from the implementation plan.
2. Form execution waves by grouping tasks whose dependencies are already satisfied.
3. Choose the execution context:
   - use `worker-backed preferred` when the plan meets one or more worker-value criteria
   - use `in-context fallback` when the plan is simple and tightly coupled, or workers are unavailable or unreliable
   - record the concise rationale in the execution plan
4. For each wave, choose the scheduling strategy:
   - `concurrent workers` when the wave contains 2 or more truly independent tasks
   - `sequential workers` when dependencies, shared files, or unstable interfaces require ordering
   - in `in-context fallback`, execute the same wave sequentially in the current context
5. In `worker-backed preferred`, execute every selected worker wave through worker contexts. For sequential waves, wait for one worker to finish before starting the next dependent worker.
6. Present the execution plan and proceed automatically. See `references/output-templates.md` for the "Wave Execution Plan" template.

### PHASE 2 - Implementation

Execute the work wave by wave using the selected execution context and scheduling strategy.

For each task:
1. Read only the relevant sections of the touched files.
2. Treat the task `body` execution contract as the primary instruction for that task. If it contains Objective, Read, Change, Steps, Verify, Done, and Blockers, follow those sections literally.
3. Do not make new architectural decisions during implementation. If the task contract is missing a decision that changes the user-facing contract, data model, security model, or integration boundary, stop and report the blocker.
4. Follow mockups when UI work is involved.
5. Run the task-specific verification from the task contract when present. If the contract has no verification command, run the smallest relevant project check that proves the task.
6. Mark the task as done only after verification passes: run `archetipo task done {US-CODE} {TASK-ID}` from `data.project_root`.
7. Announce completion briefly.

#### Ugo's rules

- Follow the planned technical solution
- Reuse existing patterns for naming, folder structure, and architecture
- Do not add scope beyond the spec
- Verify directories exist before creating files
- Prefer local adaptation over unnecessary escalation

#### Mockup rules

- Before writing UI code, Ugo must read the relevant mockup files identified in Phase 0
- If the plan explicitly references a mockup, that mockup is the source of truth for layout, hierarchy, and component structure
- If mockups exist but are not explicitly referenced, use them as design context and avoid contradicting them
- If no mockups exist, follow established UI patterns from the codebase

#### Mina's test rules

- Write tests that verify the spec acceptance criteria
- Follow the test strategy in the implementation plan
- Reuse the project's existing testing patterns
- Make tests independent and repeatable
- Name tests by behavior, not by implementation detail

#### Mina's E2E policy

Apply this section when the plan requires e2e coverage, or when Mina determines e2e is necessary for the implemented user flow.

**When required**

- If the plan includes an e2e strategy, Mina must define and author those tests
- Do not skip e2e coverage only because it is harder than unit or integration testing

**Authoring**

- Detect the existing e2e framework from project config, `package.json`, agent instructions files, and existing tests
- Reuse the existing stack when present
- Map each e2e scenario to a user flow described in the plan
- Write real end-to-end flows: navigation, interaction, waiting, and outcome assertions

**Bootstrap authorization**

- When the intended stack is Playwright (detected, or signalled by the repo/plan), bootstrap it deterministically by running `archetipo e2e ensure` from `data.workdir`. The command is idempotent and non-interactive: it installs `@playwright/test` only when missing, writes a minimal config without overwriting an existing one, and installs a single browser. Parse the JSON envelope and branch on `error.code` — e.g. `E_PRECONDITION` when there is no `package.json` yet, which means the Node project must be initialized first. Do **not** run ad-hoc interactive installers (`npm init playwright@latest`) or download every browser.
- For a non-Playwright stack signalled clearly by the repo/plan, Mina may install and configure it following the same idempotent, non-interactive discipline.
- If the intended stack cannot be determined, treat the stack choice as an explicit blocker rather than choosing a framework arbitrarily.

**Demo video — not recorded here**
- Do not record demo videos during implementation. The demo scenario and its video are produced at the acceptance gate by `archetipo-review`, which owns the record/skip decision and runs `archetipo e2e demo`.
- E2E coverage and demo video are independent decisions: author the e2e tests the plan requires; keep them fast and unrecorded. Recording happens later, in review.

**Run and artifacts**
- Run the functional e2e suite with `archetipo e2e run` (Playwright) or the project's e2e command. It runs headless with **no video** — recording is a review concern.
- Run the suite in the foreground with an explicit timeout sized for it. If it genuinely exceeds the runtime's per-call limit, split it — never move it to the background, because you will not be there when the notification arrives.
- Start background services only when needed, and wait for readiness with a blocking check inside the same call (`until curl -sf localhost:3000 >/dev/null; do sleep 2; done`), never by ending your turn.
- Verify the expected (non-video) artifacts, such as test reports, are produced.
- Retry flaky or timeout-based failures once; if they fail again, report them clearly as non-transient.

#### Progress reporting

After each wave, report briefly. See `./references/output-templates.md` for the "Wave Completion Report" template.

#### Wiki maintenance gate

**This entire gate is skipped when `data.wiki.enabled` is `false`** (see **Wiki gate** in `.archetipo/shared-runtime.md`): run no `wiki` command, edit no page, and go straight to **Before code review**. A plan produced with the gate off carries no `Wiki Impact` contract, so there is nothing to fulfil; if an old plan still declares one, say so in the completion summary and leave the pages untouched rather than acting on it.

After code and tests are stable, fulfill the plan's `Wiki Impact` contract before code review. It covers **required** pages only — the definition is in `.archetipo/shared-runtime.md`. A page that a changed file merely co-cites is not your business: leave its content, status, and review metadata alone, and never invoke `wiki reconfirm`.

1. **Write the required pages.** Complete the dedicated task for every ID in `wiki_impact.update` and `wiki_impact.create`. Reset a reviewed page with `archetipo wiki --project-root {data.workdir} reset <page-id>` before editing it; create every planned page at its canonical ID path. Leave them `status: generated` with no `review` metadata.
   - For a planned `decisions/<slug>`, write a `type: decision` page with `decision_status: accepted`, repository evidence in `sources`, and meaningful `context`, `decision`, `alternatives`, `consequences`, and `verification` markers. Preserve the rationale and alternatives from the plan; never reverse-engineer them from the finished code. When the plan supersedes an ADR, update the old decision status and its links exactly as planned.
   - Update `engineering/code-map` coverage when the implementation introduces or removes an inspected capability candidate, even when no new domain page is warranted.
2. **Check what the plan could not foresee.** Run `archetipo wiki --project-root {data.workdir} affected` against the exact spec diff. If a returned page is not in the contract but the diff has made one of its claims false, fix it — it becomes required by the act of editing it. This is a check on the plan's blind spots, not a licence to edit every page the diff touches.
3. Run `wiki catalog`, then `wiki validate --profile bootstrap`, and repair every error, strong finding, and coverage warning caused by the spec.
4. **Prove the state, do not assume the action.** Build the required ID list — plan contract plus any Wiki file your diff touched — and run `archetipo wiki --project-root {data.workdir} status --require-generated <required-id>...`. It returns `ok: true` only when every listed page reports `state: generated`; otherwise it fails with `E_CONFLICT` naming the offending IDs. A page reported `reviewed`, `stale`, or `evidence-changed` was edited without `wiki reset`: run the reset — it is idempotent, and running it after the edit repairs the state exactly as running it before would have — and re-check. A page reported `attention` carries an unresolved issue: fix the issue, not the state. `wiki validate` returning `ok: true` does not cover any of this, because `WIKI_REVIEW_OUTDATED` and `WIKI_EVIDENCE_CHANGED` are warnings. Do not enter code review while the check fails.

#### Before code review

After all implementation waves:

1. Run the project's unit and integration tests
2. Run e2e tests if this spec required or introduced them
3. If tests fail, determine whether the failure is new or pre-existing, fix local issues autonomously, and escalate only if an explicit blocker appears
4. Perform a plan adherence drift check: compare the final diff against the spec acceptance criteria, the plan body, and every task execution contract. Fix missing planned work before review; report any deliberate deviation in the completion summary.

### PHASE 3 - Code Review

**Main agent:** Cesare 🔍

- In `worker-backed preferred`, delegate review to a separate worker
- In `in-context fallback`, review in the current context
- Review only diffs or changed areas, using project conventions and the implementation plan as reference

**Review criteria:**

1. plan adherence
2. code quality
3. architecture adherence
4. security
5. test quality
6. mockup adherence when UI work exists
7. completeness vs. tasks and acceptance criteria

**Output format:** See `./references/output-templates.md` for the "Code Review Output" template.

### PHASE 4 - Fix & Re-Review Loop

If Cesare found critical issues:

1. Ugo and Mina fix them
2. Re-run the relevant tests
3. Re-review only the fix diffs
4. Repeat until no critical issues remain

If Cesare found only improvements:

1. Summarize them briefly.
2. Treat them as non-blocking by default.
3. Fix them only if the user explicitly asks for extra polishing, or if re-checking shows that one of them is actually critical.

If Cesare found no issues, or all critical issues are fixed, proceed to completion.

### Completion Gate

Proceed to Phase 5 only when all of the following are true:

- no `🔴 CRITICAL` findings remain open
- the full required final test suite passes
- the Wiki impact contract is complete, `wiki validate --profile bootstrap` passes, and `wiki status --require-generated <required-id>...` returns `ok: true` for every required page, when a Wiki exists and `data.wiki.enabled` is `true` (with the gate off this criterion does not apply)
- the spec can be moved to `{config.workflow.statuses.review}` via `archetipo spec review`

`🟡 IMPROVEMENT` findings do not block completion by default.
Implementation is not complete until the spec status has been updated to `{config.workflow.statuses.review}`.
Do not end with the spec still in `{config.workflow.statuses.in_progress}`, and do not move it to `{config.workflow.statuses.done}` from this skill.

### PHASE 5 - Completion & Backlog Update

1. Run the full required test suite one final time. If it fails, return to the fix loop and do not transition the spec.
2. Choose a Conventional Commit type that describes the completed work:
   - `feat`: new user-facing capability or behavior
   - `fix`: bug fix
   - `ci`: CI/CD, workflow, release automation
   - `build`: build system, dependencies, package infrastructure
   - `test`: tests-only changes
   - `docs`: documentation-only changes
   - `refactor`: code restructuring without behavior change
   - `perf`: performance improvement
   - `chore`: maintenance or tooling that does not fit the above
3. Optionally run `git status --short` under `data.workdir` for visibility, then invoke `archetipo spec review {US-CODE}` from `data.project_root` with `--commit-type <type>` and, when the spec title is not specific enough, `--commit-summary "<concise summary>"`. Pipe the completion summary markdown via stdin as the closing comment. This single command commits any dirty/untracked worktree changes with a Conventional Commit subject (`<type>({US-CODE}): <summary>`), transitions the spec to `{config.workflow.statuses.review}`, and posts the comment on the parent issue (or silently ignores it for connectors without comment support — never branch on connector type).
   - Example: `archetipo spec review US-125 --commit-type ci --commit-summary "add release workflow"`
4. Confirm completion with a concise summary. See `references/output-templates.md` for the "Completion Summary" template. If non-blocking `🟡 IMPROVEMENT` items remain open, include them in the final report under an explicit optional improvements section.

## Edge Case Handling

- **Review loop exceeds 3 iterations:** summarize what remains and recommend whether to continue or re-evaluate
