---
name: archetipo-autopilot
description: Runs the ARchetipo plan-to-implementation pipeline autonomously until the selected backlog specs reach REVIEW. Accepts no argument for every eligible spec, one EP-XXX code for an epic, or one US-XXX code for a single spec. Requires a fresh isolated worker for every planning and implementation phase. Use when the user asks to run everything, implement the backlog, process an epic autonomously, or autopilot one or more specs end to end.
---

# ARchetipo Autopilot

Act as a lightweight controller. Reconcile each selected spec from its current workflow state to `REVIEW` by running `archetipo-plan` and `archetipo-implement` in separate fresh workers. Never read source code, plans, PRDs, or Wiki pages in the controller context.

## Shared runtime

Read `.archetipo/shared-runtime.md` exactly once at activation time. Apply its JSON envelope, project-root, language, and error rules throughout this run.

## Execution contract

1. Require a foreground worker/subagent mechanism that creates a fresh isolated context for every invocation and lets the controller wait for its result.
2. Stop before creating state or mutating the project when that capability is unavailable or cannot be established. Do not execute either phase in the controller context.
3. Run one phase at a time. Never run two spec-writing workers concurrently.
4. Treat CLI state as authoritative. A worker summary is telemetry, never proof of success.
5. Stop at the first failed worker, unresolved dependency, invalid transition, or failed verification. Do not skip and do not retry automatically.
6. Target `{config.workflow.statuses.review}`. Final human acceptance through `archetipo-review` remains outside this skill.
7. Do not start, detect, or depend on a vendor-native goal, loop, or autopilot mode.

## Input

Accept exactly one of these forms:

```text
/archetipo-autopilot
/archetipo-autopilot EP-002
/archetipo-autopilot US-017
```

Interpret them as follows:

- no argument: select every eligible spec;
- one `EP-[0-9]{3}` token: select eligible specs whose `epic.code` matches it;
- one `US-[0-9]{3}` token: select that exact spec.

Reject flags, free-form conditions, malformed identifiers, extra tokens, and multiple identifiers before any mutation. Do not normalize lowercase or infer a partial code.

## Direct CLI operations

Invoke only these CLI operations in the controller:

- `archetipo config show`
- `archetipo spec list`
- `archetipo spec show <US-CODE>`

Run them from `data.project_root`, parse stdout and stderr as the shared JSON envelopes, and branch on `error.code`. Phase workers invoke the additional commands declared by their own skills.

## Isolated worker contract

A compatible worker mechanism must:

- start each invocation with a fresh context that does not contain prior phase conversation or tool output;
- receive a bounded prompt and the absolute project root;
- load and execute the installed phase skill;
- have the tools and permissions needed by that skill;
- run in the foreground so the controller can wait;
- terminate after returning a concise result.

Nested workers are not required. The phase worker may complete its entire phase itself when its skill provides an inline execution path.

If the contract is unavailable, stop with a message equivalent to:

```text
ARchetipo Autopilot is unavailable because this runtime cannot create a fresh
isolated worker for every planning and implementation phase. Run each phase in
a separate agent session or use a compatible runtime.
```

## Run state

Persist an active run in:

```text
.archetipo/autopilot-state-{UTC_TIMESTAMP}-{SHORT_SUFFIX}.yaml
```

Use this minimal shape:

```yaml
autopilot:
  id: "20260722T103000Z-a1b2"
  scope: ALL       # ALL | EP-XXX | US-XXX
  selected_specs: [US-001, US-002]
  current_spec: US-001
  status: running  # running | error
  started_at: "2026-07-22T10:30:00Z"
  updated_at: "2026-07-22T10:35:00Z"
  last_error: null
```

Write state updates atomically through a sibling temporary file followed by replacement when the available file tools support it. Never store phase summaries, duplicated workflow states, plans, task bodies, or source-code observations.

Before creating a run:

1. Find active `autopilot-state-*.yaml` files with `status: running` or `status: error`.
2. If exactly one file exists and its `scope` equals the requested scope, resume it automatically from its frozen `selected_specs`; set `status: running` and clear `last_error`.
3. If an active file has a different scope, or multiple active files exist, stop without mutation and report the conflicting paths.
4. Never silently discard an active run.

Delete the state file only after every selected spec is verified at `REVIEW` or `DONE`. Retain it after errors or interruption so a later invocation with the same scope can resume.

## Workflow

### Phase 0 — Initialize

1. Validate the input.
2. Verify the isolated worker contract.
3. Run `archetipo config show` and retain `data.project_root`, configured workflow labels, and paths.
4. Detect and resume a matching active run when present.
5. Otherwise run `archetipo spec list` and freeze the selection:
   - `ALL`: specs in configured `TODO`, `PLANNED`, or `IN PROGRESS` states;
   - `EP-XXX`: the same eligible states within that epic;
   - `US-XXX`: the exact spec, including `REVIEW` or `DONE` so an already-satisfied invocation can finish as a no-op.
6. For an unknown `EP-XXX` or `US-XXX`, stop before creating state and report the missing scope.
7. If `ALL` or an epic contains no eligible specs, report that the scope already has no work requiring Autopilot and stop successfully without creating state.
8. Validate dependencies and order the frozen selection as described below.
9. Create the state file and announce the frozen queue.

### Dependency ordering

Use `blocked_by` from `archetipo spec list` to build a dependency graph.

- Place every selected blocker before its dependent spec.
- Among currently dependency-ready specs, sort by priority `HIGH`, `MEDIUM`, `LOW`, then by code.
- Treat a dependency outside the frozen selection as satisfied only when its current status is configured `REVIEW` or `DONE`.
- Treat a missing blocker, a dependency cycle, or an external blocker below `REVIEW` as a blocking error. Do not widen an explicit epic or spec scope automatically.

### Phase 1 — Reconcile specs

For each frozen spec code:

1. Set `current_spec` and update `updated_at` in the state file.
2. Run `archetipo spec show <US-CODE>`.
3. Choose exactly one action from the observed state:
   - configured `TODO`: run the planning phase;
   - configured `PLANNED` or `IN PROGRESS`: run the implementation phase;
   - configured `REVIEW` or `DONE`: mark the spec satisfied and continue;
   - any other state: fail the run.
4. After a successful planning phase, observe the spec again and continue reconciliation of the same spec; do not advance directly from the worker summary.
5. After a successful implementation phase, observe and verify the spec, then advance to the next code.

#### Planning phase

Spawn one fresh worker with a prompt containing only:

```text
Working directory: {data.project_root}
Spec: {US-CODE}
Phase skill: archetipo-plan
Expected terminal state: {config.workflow.statuses.planned}

Load the installed archetipo-plan skill and execute it for {US-CODE}. Read the
project instructions and current repository state yourself. Do not assume any
knowledge from earlier Autopilot phases. Complete the whole planning phase in
this worker and return a concise outcome.
```

After the worker terminates, run `archetipo spec show <US-CODE>` regardless of its textual result. Accept the phase only when:

- `data.spec.status` equals configured `PLANNED`; and
- `data.tasks` is non-empty.

Otherwise fail the run with the observed status and task count.

#### Implementation phase

Spawn a different fresh worker with a prompt containing only:

```text
Working directory: {data.project_root}
Spec: {US-CODE}
Phase skill: archetipo-implement
Expected terminal state: {config.workflow.statuses.review}

Load the installed archetipo-implement skill and execute it for {US-CODE}.
Read the persisted plan, project instructions, and current repository state
yourself. Do not assume any knowledge from planning or earlier specs. Complete
the whole implementation phase in this worker and return a concise outcome.
```

After the worker terminates, run `archetipo spec show <US-CODE>` regardless of its textual result. Accept the phase only when:

- `data.spec.status` equals configured `REVIEW`; and
- `data.tasks` is non-empty; and
- every `data.tasks[].status` is canonical `DONE`.

Otherwise fail the run with the observed status and remaining task IDs.

Mockups, E2E coverage, task execution, fix loops, Wiki maintenance, and code review belong entirely to the phase skills. Do not duplicate or infer those responsibilities from worker summaries.

### Phase 2 — Complete

After the queue is exhausted:

1. Run `archetipo spec show` once more for every frozen code.
2. Require every spec status to be configured `REVIEW` or `DONE`.
3. Delete the state file.
4. Report the scope, verified spec codes, final statuses, and that human acceptance remains pending for specs in `REVIEW`.

## Failure handling

On any worker failure, CLI error, unresolved dependency, unexpected state, or verification failure:

1. Re-read the current spec with `archetipo spec show` when possible.
2. When the state file already exists, set `status: error`, update `updated_at`, and record a concise `last_error` containing the spec, phase, observed state, and stable `error.code` when available. Failures detected before state creation remain mutation-free.
3. Stop immediately. Do not retry, skip, start another worker, delete the state file, or continue to another spec.
4. If state was created, report its retained path and explain that invoking Autopilot again with the same scope resumes from authoritative CLI state. Otherwise report that no run state was created.

## User-facing progress

Keep controller output compact:

- opening: scope and frozen queue;
- after each verified phase: spec, phase, observed transition;
- after each satisfied spec: progress count;
- closure: verified final states or the first blocking error.

Render all messages in the language selected by the shared runtime.
