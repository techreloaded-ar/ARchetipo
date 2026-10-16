---
name: archetipo-autopilot
description: Runs the ARchetipo pipeline autonomously end to end, from planning through implementation to autonomous acceptance, until the selected backlog specs reach DONE. Applies a bounded fix loop of at most three rework cycles per spec; a spec that is still not acceptable stays in REVIEW for a human while the run continues with the remaining specs. Accepts no argument for every eligible spec, or any combination of EP-XXX and US-XXX selectors whose eligible specs are unioned, plus an optional --max-specs N flag that caps how many specs the run freezes and completes. Uses a compact in-context path for one bounded local spec and fresh phase workers for larger work; acceptance always runs in a fresh independent worker. Use when the user asks to run everything, implement the backlog, process an epic autonomously, or autopilot one or more specs end to end.
---

# ARchetipo Autopilot

Act as a lightweight controller. Reconcile each selected spec from its current workflow state to `{config.workflow.statuses.done}` by invoking the existing planning, implementation and acceptance skills. Choose the compact or isolated context below. Between phases read only CLI control state; source/plan/PRD/Wiki work belongs to the active phase skill, whether it runs inline or in a worker. Acceptance always remains independent.

## Shared runtime

Read `.archetipo/shared-runtime.md` exactly once at activation time. Apply its JSON envelope, project-root, language, and error rules throughout this run.

Read `./references/acceptance-loop.md` exactly once at activation time as well. It carries the acceptance worker prompt, the CLI-authoritative cross-check table, and the rework budget accounting. Acceptance rules themselves belong to `archetipo-review`: never restate, summarize, or anticipate them here.

## Execution contract

1. Require a worker/subagent mechanism that creates a fresh independent acceptance context and lets the controller await its terminal result. In isolated mode use it for planning and implementation as well.
2. Stop before creating state or mutating the project when that capability is unavailable or cannot be established. Do not execute any phase in the controller context.
3. Run one phase at a time. Never run two spec-writing workers concurrently.
4. Treat CLI state as authoritative. A worker summary — including an acceptance verdict block — is telemetry, never proof of success.
5. **Recover bounded failures without weakening acceptance.** Apply *Failure handling* to a terminal worker that did not finish its phase. Preserve completed work and approvals; stop for an unresolved external dependency, changed product scope, uncertain ownership or exhausted recovery budget.
6. Target `{config.workflow.statuses.done}` through autonomous acceptance. A spec that is still not acceptable after `max_review_iterations` rework cycles stays in `{config.workflow.statuses.review}` for human review through `/archetipo-review`.
7. Do not start, detect, or depend on a vendor-native goal, loop, or autopilot mode.

## Input

The argument list is a sequence of **selectors** plus optional **flags**, in any order:

```text
/archetipo-autopilot
/archetipo-autopilot EP-002
/archetipo-autopilot US-017
/archetipo-autopilot EP-002 US-011 US-030
/archetipo-autopilot --max-specs 3
/archetipo-autopilot EP-002 --max-specs 2
```

Interpret selectors as follows:

- no selector: select every eligible spec;
- an `EP-[0-9]{3}` selector: select eligible specs whose `epic.code` matches it;
- a `US-[0-9]{3}` selector: select that exact spec;
- several selectors: select the **union** of what each one contributes. A spec named explicitly and also reached through its epic collapses into a single entry.

Interpret `--max-specs N` as the maximum number of specs this run freezes and completes. `N` is an integer `>= 1`. The cap truncates the ordered queue in Phase 0, so the announced queue is already the definitive one; excluded specs never enter the CLI checkpoint and are never touched. A spec already in configured `{config.workflow.statuses.done}` at freeze time does not consume the cap, because it needs no worker.

Reject unknown flags, free-form conditions, malformed identifiers, and a malformed `--max-specs` value — missing, non-integer, `<= 0`, or the flag repeated — before any mutation. Do not normalize lowercase or infer a partial code.

## Direct CLI operations

Use these read-only workflow operations plus the checkpoint operations below:

- `archetipo config show`
- `archetipo spec list`
- `archetipo spec show <US-CODE>`

Run them from `data.project_root`, parse stdout and stderr as the shared JSON envelopes, and branch on `error.code`. Artifact mutations belong to the invoked phase skill and are declared by its own contract. In isolated mode only the phase worker performs them; in compact mode perform them only while executing that base skill inline. Do not reimplement any phase inside this controller.

## Isolated worker contract

A compatible worker mechanism must:

- start each invocation with a fresh context that does not contain prior phase conversation or tool output;
- receive a bounded prompt and the absolute project root;
- load and execute the installed phase skill;
- have the tools and permissions needed by that skill;
- let the controller wait for completion and retrieve the terminal result before starting another phase;
- terminate after returning a concise result.

Choose the invocation from the runtime's documented tool schema, not from the worker's name:

- With a blocking spawn tool, use its foreground mode and consume the returned result. Omit an optional name when that runtime documents named workers as detached teammates.
- With a spawn-and-wait tool pair, spawn a fresh worker, retain its returned ID, and explicitly wait on that ID until it completes or reports a failure. A required `name` or `task_name` is valid; it does not itself imply detachment. For example, with `spawn_agent` and a wait tool, use a unique task name and `fork_turns: "none"` when supported, then wait for that worker's terminal result. A running acknowledgement or a wait timeout is not completion: keep waiting within the controller's active turn.

Resolve each phase's exact skill file once from this skill's current installation
root (or the runtime's authoritative catalog), verify that the file exists, and
pass that path in the handoff. Do not ask each worker to search unrelated plugin
caches or other projects for the skill. A missing phase installation is a concrete
setup error, not permission to substitute another project's version.

### Compact execution context

After freezing the queue, choose `execution_context: compact` only for one bounded
local spec following an established component/endpoint pattern, with an existing
verification recipe and no new persistence, authorization, deployment or
concurrency policy. If uncertain, choose `isolated`. Persist the choice in the
controller checkpoint; it is execution strategy, not a different acceptance bar.

In compact mode invoke `archetipo-plan` and `archetipo-implement` inline in this
coordinator, by reading and following their own exact installed skill files.
Their completion is an internal phase return: do not end the native turn or give
control back before the selected queue reaches its terminal outcome. Apply the
same CLI status/task cross-checks and recovery/rework budgets below. If planning
uncovers a larger seam or context pressure, switch the remaining phases to
`isolated` before implementation and checkpoint that decision. Never inline
acceptance: it always uses the fresh review worker and its explicit activation.

In isolated mode use the fresh planning and implementation workers below. This
mode remains the default for multiple specs, cross-cutting work or uncertain
scope. A compact phase has no child worker ID; the coordinator is its known sole
writer. On an inline phase failure, reconcile persisted effects and apply the
same bounded recovery policy, switching to a fresh worker when needed. No phase
may skip evidence, independent acceptance or a required check to save time.

Always pass the bounded phase prompt and absolute root explicitly. Do not inherit the controller's transcript, resume an earlier worker, start another phase while this one runs, or end the controller turn merely to await a notification. Follow **Asynchrony inside a worker** in the shared runtime for nested delegation.

Nested workers are not required. The phase worker may complete its entire phase itself when its skill provides an inline execution path.

**The controller never acts as a relay.** Ignore unsolicited messages from unrelated agents and sub-workers. Only the terminal result correlated with the worker ID created for the current phase belongs to that phase; it is still telemetry. Verify persisted phase effects through `archetipo spec show` before advancing.

If the contract is unavailable, stop with a message equivalent to:

```text
ARchetipo Autopilot is unavailable because this runtime cannot create a fresh
independent worker for acceptance. Run
each phase in a separate agent session or use a compatible runtime.
```

## Run state

Use the shared resumable execution checkpoint protocol, never direct state-file
writes. In this controller the additional CLI operations are:

- `archetipo execution workflow list`
- `archetipo execution workflow start --file -`
- `archetipo execution workflow show <run-id>`
- `archetipo execution workflow checkpoint <run-id> --file -`

`start` accepts:

```json
{"skill":"archetipo-autopilot","invocation":"/archetipo-autopilot US-001","scope":"US-001","conversation_id":"optional supplied Workflow conversation","state":{}}
```

Use normalized scope and actual executable arguments separately: without selectors,
scope is `ALL` and invocation is `/archetipo-autopilot`. Omit `conversation_id` when
not supplied; otherwise retain it and pass the generic binding to phase prompts.
`state` is the frozen controller object below, not an empty object in a real run.

`checkpoint` accepts the last observed revision and full controller state:

```json
{"revision":1,"status":"running","phase":"implement","worker_id":"fresh worker id","state":{},"last_error":""}
```

Use `blocked` for a concrete blocker and `complete` after final reconciliation.
Every command returns `archetipo/v1`, `kind: workflow_run`, with the record in
`data` (an array for list). `E_INVALID_INPUT`: fix the payload; `E_NOT_FOUND`:
resolve the right run; `E_CONFLICT`: reread and reconcile revision/scope/binding,
never overwrite blindly; `E_INTERNAL`: preserve progress and report the failure.

The controller-owned `state` is:

```json
{"scope":"US-001","max_specs":null,"selected_specs":["US-001"],"excluded_by_max_specs":[],"current_spec":"US-001","max_review_iterations":3,"max_phase_recoveries":3,"specs":{"US-001":{"outcome":"pending","review_iterations":0,"phase_recoveries":{}}}}
```

`scope` is `ALL` without selectors; otherwise deduplicate and sort selectors with
EP codes first, then US codes, joined with spaces. Freeze selection, caps and
budgets at creation. `outcome` is bookkeeping: terminal values are `done`,
`left_in_review` and `skipped_blocked`. The latter two retain unresolved findings
or their blocking chain. Resuming preserves counters, including interrupted rework.

Before creating a run, list unfinished runs for this skill. Resume exactly one
matching scope and binding, and use its frozen state; a different `--max-specs`
does not widen it. Conflicting scopes or multiple unfinished runs are blockers.
A run marked `blocked` resumes only on an explicit user request. Store a fresh
worker's ID and phase before waiting; clear the ID only after terminal completion.
Never start another writer until prior worker ownership has been resolved.

For compatibility, when no CLI run exists, read any legacy
`.archetipo/autopilot-state-*.yaml` without editing it. Import the single matching
run's frozen queue, outcomes and rework counters into the initial `state`; retain
its previous `planning_transport_retries` as consumed planning recoveries. Refuse
conflicting legacy scopes. Once imported, only the CLI run is authoritative.

Checkpoint after each dispatch observation, completed rework cycle or recovery.
Before dispatching acceptance, checkpoint `pending_acceptance` with the spec,
pre-dispatch status and `review_iterations`. On resume, reconcile that marker
before another dispatch: observed TODO with `spec.rework: true` increments the
recorded pre-dispatch count exactly once; DONE records acceptance; REVIEW retries
the incomplete acceptance. Clear the marker in the same checkpoint as the reconciled
count/outcome. Other states require explicit reconciliation, not a guessed verdict.

At successful queue completion mark the run `complete`; retain it as history.
Never delete an unfinished run. Never store source code or phase summaries.

## Workflow

### Phase 0 — Initialize

1. Validate the input and compute the normalized scope.
2. Verify the isolated worker contract.
3. Run `archetipo config show` and retain `data.project_root`, configured workflow labels and paths.
4. Detect and resume a matching active run when present.
5. Otherwise run `archetipo spec list` and build the selection as the union of what every selector contributes:
   - no selector (`ALL`): specs in configured `TODO`, `PLANNED`, `IN PROGRESS`, or `REVIEW` states — a spec waiting in review is remaining work now that the target is `{config.workflow.statuses.done}`;
   - `EP-XXX`: the same eligible states within that epic;
   - `US-XXX`: the exact spec, including configured `DONE` so an already-satisfied invocation can finish as a no-op.

   Deduplicate by spec code: a spec contributed by several selectors is one entry.
6. For unknown `EP-XXX` or `US-XXX` selectors, stop before creating state and report **every** missing selector, not just the first.
7. If the union is empty, report that the scope already has no work requiring Autopilot and stop successfully without creating state.
8. Order the union as described below. A dependency cycle or a missing blocker is a blocking error here, because it leaves the order undefined.
9. When `max_specs` is set, walk the ordered queue and retain: every spec already in configured `DONE`, which consumes no budget because it needs no worker, plus the first `N` specs below `DONE`. Drop the rest, preserving the order of what stays, and record the dropped codes in `excluded_by_max_specs`.
10. **Only now** validate external blockers, and only for the retained specs: a dependency outside the frozen selection counts as satisfied solely when its current status is configured `DONE`. Running this check after truncation is deliberate — checking it before would fail the run over a blocker of a spec the cap excluded anyway.
11. Create the CLI run with one `specs` entry per retained code (`outcome: pending`, `review_iterations: 0`) and announce the frozen queue, including how many eligible specs the cap left out.

### Dependency ordering

Use `blocked_by` from `archetipo spec list` to build a dependency graph.

- Place every selected blocker before its dependent spec.
- Among currently dependency-ready specs, sort by priority `HIGH`, `MEDIUM`, `LOW`, then by code.
- Treat a dependency outside the frozen selection as satisfied only when its current status is configured `DONE`. The threshold is `DONE` rather than `REVIEW` because this run accepts specs: in the worktree workflow `spec integrate` refuses a spec with unintegrated blockers, and outside it, accepting work built on unaccepted work is incoherent.
- Treat a missing blocker, a dependency cycle, or an external blocker below `DONE` as a blocking error. Do not widen an explicit epic or spec scope automatically.

Because every selected blocker precedes its dependent, any prefix of this order is closed under internal blockers: a retained spec can never lose a blocker that was part of the selection. This is what makes `--max-specs` truncation safe — it drops dependents, never their prerequisites, and cannot manufacture spurious `skipped_blocked` outcomes.

### Phase 1 — Reconcile specs

For each frozen spec code:

1. Skip the code when its recorded `outcome` is already terminal — a resumed run does not redo settled specs. Otherwise set `current_spec` and update `updated_at` in the CLI checkpoint.
2. **Pre-dispatch blocker check.** Walk the `blocked_by` chain inside the frozen selection, transitively. If any blocker already holds outcome `left_in_review` or `skipped_blocked`, set this spec's outcome to `skipped_blocked` with the recorded chain, spawn no worker, and advance to the next code.
3. Run `archetipo spec show <US-CODE>`. Retain this observation as the **pre-dispatch fingerprint** of the phase you are about to run: `data.spec.status`, `data.spec.rework`, `data.spec.branch`, `data.spec.worktree`, `data.spec.fork_base`, and the ordered list of `data.tasks[].id` with each `status`. It is the observation used to reconcile phase effects in *Failure handling*; keep it in the controller context only, never in the CLI checkpoint.
4. Choose exactly one action from the observed state:
   - configured `TODO`: run the planning phase;
   - configured `PLANNED` or `IN PROGRESS`: run the implementation phase;
   - configured `REVIEW`: run the acceptance phase;
   - configured `DONE`: set outcome `done` and advance to the next code;
   - any other state: fail the run.
5. Loop on the same spec — observe, dispatch, observe again — until it reaches a terminal outcome (`done`, `left_in_review`, or `skipped_blocked`). Never advance on a worker summary alone; every transition is confirmed with `archetipo spec show`.

#### Planning phase

In compact mode execute the installed planning skill inline, then apply the same CLI cross-check below. Otherwise spawn one fresh worker with the following prompt:

```text
Working directory: {data.project_root}
Communication language: {detected human language}
Spec: {US-CODE}
Phase skill: archetipo-plan
Skill file: {resolved installed archetipo-plan SKILL.md path}
Expected terminal state: {config.workflow.statuses.planned}

Load the installed archetipo-plan skill and execute it for {US-CODE}. Read the
project instructions and current repository state yourself. Do not assume any
knowledge from earlier Autopilot phases. Complete the whole planning phase in
this worker and return a concise outcome.
```

After the inline phase returns or the worker terminates, run `archetipo spec show <US-CODE>` regardless of its textual result. Accept the phase only when:

- `data.spec.status` equals configured `PLANNED`; and
- `data.tasks` is non-empty.

This is deliberately blind to how the worker ended: a worker killed right after `archetipo spec plan` succeeded produced a correct phase, and CLI state says so.

Otherwise apply *Failure handling*, recording the observed status and task count.

The same phase handles a spec that came back from acceptance: `archetipo-plan` turns the persisted rework feedback into Fix tasks. The controller needs no variant prompt for it.

#### Implementation phase

In compact mode execute the installed implementation skill inline, then apply the same CLI cross-check below. Otherwise spawn a different fresh worker with the following prompt:

```text
Working directory: {data.project_root}
Communication language: {detected human language}
Spec: {US-CODE}
Phase skill: archetipo-implement
Skill file: {resolved installed archetipo-implement SKILL.md path}
Expected terminal state: {config.workflow.statuses.review}

Load the installed archetipo-implement skill and execute it for {US-CODE}.
Read the persisted plan, project instructions, and current repository state
yourself. Do not assume any knowledge from planning or earlier specs. Complete
the whole implementation phase in this worker and return a concise outcome.
```

After the inline phase returns or the worker terminates, run `archetipo spec show <US-CODE>` regardless of its textual result. Accept the phase only when:

- `data.spec.status` equals configured `REVIEW`; and
- `data.tasks` is non-empty; and
- every `data.tasks[].status` is canonical `DONE`.

Otherwise apply *Failure handling* with the observed status and remaining task IDs.

#### Acceptance phase

Read the spec's `review_iterations` from the CLI checkpoint before spawning, and set `Request-changes allowed` to `yes` while it is below `max_review_iterations`, otherwise to `no`. Spawn a fresh worker using the acceptance prompt template in `./references/acceptance-loop.md` verbatim, including its three literal activation lines — `archetipo-review` refuses to enter Autonomous acceptance mode without them.

After the worker terminates, run `archetipo spec show <US-CODE>` regardless of its textual result and apply the CLI-authoritative cross-check table of `./references/acceptance-loop.md` exactly:

- configured `DONE` → outcome `done`. When `data.spec.branch` was set in the pre-worker observation, also require `branch`, `worktree`, and `fork_base` to be empty now; a residual value is a failed integration: apply phase recovery.
- configured `TODO` → the acceptance worker requested changes. Increment `review_iterations`, persist it before anything else, and continue reconciling the same spec from its observed state.
- configured `REVIEW` → outcome `left_in_review` **only** when the worker declared `verdict: LEFT_IN_REVIEW` or the attempt already ran with `Request-changes allowed: no`. Record its `unresolved` items and advance to the next spec. `REVIEW` with budget still available and no `LEFT_IN_REVIEW` verdict is a worker failure: apply phase recovery.
- any other status → reconcile the observation and apply *Failure handling*.

The verdict block is telemetry. When it is missing, truncated, or unparsable, the observed CLI status decides; record that the worker returned no verdict.

Mockups, E2E coverage, task execution, fix loops, Wiki maintenance, demo-video recording, acceptance classification, and code review belong entirely to the phase skills. Do not duplicate or infer those responsibilities from worker summaries. The same holds for the config gates each of them owns — the **Wiki gate** in `.archetipo/shared-runtime.md` included: the controller neither reads them nor mentions them to a worker.

### Phase 2 — Complete

After the queue is exhausted:

1. Run `archetipo spec show` once more for every frozen code.
2. Require each observed status to match the recorded outcome: `done` ⇒ configured `DONE`, `left_in_review` ⇒ configured `REVIEW`. A `skipped_blocked` spec was never touched, so any status is acceptable. A mismatch fails the run.
3. Mark the CLI run `complete` through `execution workflow checkpoint`.
4. Report the normalized scope and, per spec, its outcome, final status, and the rework iterations consumed. For every `left_in_review` spec add the unresolved findings and the suggested action `/archetipo-review US-XXX`; for every `skipped_blocked` spec add the blocking chain that stopped it.
5. When `excluded_by_max_specs` is non-empty, state how many eligible specs `--max-specs` left out, list their codes, and suggest re-running Autopilot with the same selectors to continue. Never let a capped run read as an exhausted backlog.

## Failure handling

A phase succeeds from observed CLI effects even if its worker died after the final
write. Never infer success from prose, a receipt or a running acknowledgement.

For a phase that did not finish:

1. Classify the failure. A missing local test environment or a correctable defect
   within the approved spec belongs to the phase. A transport failure requires
   ownership reconciliation; never infer that an old writer stopped from a timeout.
2. Re-read `spec show`. Confirm that the worker is terminal, inspect its correlated
   result and retain remaining task IDs/statuses. In isolated mode the controller reads no code: the fresh phase worker reconciles partial edits and reruns affected checks. In compact mode invoke the relevant base skill inline for that repair; the control layer still owns no alternative implementation.
3. If repair is local and authorized, and `phase_recoveries[phase] < 3`, increment
   and checkpoint that counter before re-entering the same inline phase or spawning one fresh worker of the same isolated phase. Acceptance remains isolated in both modes.
   Append: `Resume from persisted state. Preserve completed tasks and partial edits;
   establish ownership of prior workers, repair remaining work and run the required
   checks. Do not change product scope or weaken acceptance.` Use the same spec,
   root, execution binding, model constraints and phase contract.
4. Recovery counters survive resume and never consume/reset `review_iterations`.
   Verification failures stay failures until the actual checks pass. Acceptance
   rework remains governed solely by `archetipo-review` and its bounded loop.
5. For uncertain ownership/delivery, external prerequisites that cannot be restored,
   `E_INTERNAL`, invalid dependencies, a required scope decision or exhausted phase
   recoveries, checkpoint `status: blocked` with spec, phase, stable error code,
   attempts, observed state and the minimum action needed. Return that concrete
   blocker and the run ID. Do not claim the queue is finished or skip broken work.

`left_in_review` is an expected technical verdict, not a transport failure: retain
its findings and skip dependent specs as the normal reconciliation loop prescribes.
A completed selected queue containing that outcome is not a fully delivered scope.

## User-facing updates

Announce the frozen selection, phase transitions, recoveries and concrete blockers
concisely. Keep waiting within the active turn while a worker is running; a wait
timeout is not completion. Render output in the shared-runtime language.
