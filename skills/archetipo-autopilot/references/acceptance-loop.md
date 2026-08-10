# Acceptance loop — controller protocol

This file is controller protocol only: how to spawn an acceptance worker, how to read
its outcome, and how to count the rework budget. It contains **no** acceptance rules.
Which Wiki pages are approvable, how a page is classified, when a demo video is
recorded, and how a verdict is executed all live in `archetipo-review` and must never
be restated, summarized, or anticipated here or in the controller context.

`MAX_REVIEW_ITERATIONS = 3`.

## Acceptance worker prompt

Spawn one fresh worker whose prompt contains only:

```text
Working directory: {data.project_root}
Spec: {US-CODE}
Phase skill: archetipo-review
Autonomous acceptance mode: enabled by archetipo-autopilot
Iteration: {n} of 3
Request-changes allowed: {yes|no}
Expected terminal state: {config.workflow.statuses.done} (accepted),
{config.workflow.statuses.todo} (changes requested), or
{config.workflow.statuses.review} (left in review)

Load the installed archetipo-review skill and execute it for {US-CODE} in its
Autonomous acceptance mode. Read the spec, the increment, the Wiki dossier when
the Wiki gate is on, and the current repository state yourself. Do not assume any knowledge from planning,
implementation, or earlier specs. Decide and execute the verdict in this worker,
ask no questions, and end your output with the ARCHETIPO_REVIEW_VERDICT block.
```

The three activation lines are literal — the review skill refuses to enter autonomous
mode without them. `{n}` is `review_iterations + 1` for the spec. `Request-changes
allowed` is `yes` while `review_iterations < 3` and `no` once the budget is spent, so
the attempt that follows the third rework is final.

## CLI-authoritative cross-check

The verdict block is telemetry. After the worker terminates, run
`archetipo spec show <US-CODE>` and decide from the observed status:

| Observed `data.spec.status` | Controller conclusion | Additional requirement |
|---|---|---|
| configured `DONE` | accepted; spec outcome `done` | When `data.spec.branch` was set in the pre-worker observation, also require `branch`, `worktree`, and `fork_base` to be empty now. A residual value is a failed integration: hard stop. |
| configured `TODO` | changes requested; a rework cycle starts | Increment `review_iterations` (see below), then re-dispatch the same spec from its observed state. |
| configured `REVIEW` | left in review **only** when the worker declared `verdict: LEFT_IN_REVIEW`, or the budget was already exhausted for this attempt (`Request-changes allowed: no`) | Otherwise this is a worker failure: hard stop. |
| any other status | worker failure | Hard stop. |

A missing, truncated, or unparsable verdict block never overrides the observed state:
the CLI status wins, and the one case that needs the block — `REVIEW` with budget
still available — resolves to a hard stop precisely because the block is absent.

Record `unresolved` from the block when the outcome is `left_in_review`; when the
block is missing, record the observed state instead and note that the worker returned
no verdict.

## Iteration accounting

- `review_iterations` starts at `0` and counts **completed rework cycles**, not worker
  spawns.
- Increment it only when a real transition to configured `TODO` is observed after an
  acceptance worker. A worker that crashes, returns nothing, or leaves the spec where
  it was consumes no budget, so a transient runtime failure never silently shortens
  the fix loop.
- With `review_iterations = 3` the next acceptance worker receives
  `Request-changes allowed: no`; its only outcomes are `ACCEPTED` or `LEFT_IN_REVIEW`.
- The cap is frozen per run in `max_review_iterations` when the state file is created,
  so a resume keeps the budget the run started with.

## Per-spec bookkeeping and resume

The controller keeps one entry per selected spec under `autopilot.specs` (full state
file shape in `SKILL.md`):

```yaml
US-002: { outcome: pending, review_iterations: 2 }
```

- `outcome` is autopilot bookkeeping, never a mirror of workflow state. Terminal values
  are `done`, `left_in_review`, and `skipped_blocked`; `left_in_review` also carries
  `unresolved: ["..."]`, and `skipped_blocked` carries the blocking chain.
- Write `review_iterations` to the state file immediately after observing the `TODO`
  transition, before dispatching the re-plan, so an interruption cannot lose the count.

On resume, the authoritative state is always `archetipo spec show`. Two mid-rework
cases matter:

- A spec in configured `TODO` with `review_iterations > 0` is inside a fix cycle:
  continue plan → implement → acceptance with the recorded budget. Do not reset it and
  do not treat the spec as fresh work.
- A spec in configured `REVIEW` with `outcome: pending` was interrupted during or before
  an acceptance worker: dispatch a new acceptance worker with the recorded budget. The
  review skill's own operations are idempotent, so a repeated attempt is safe.
