# Plan payload assembly

The plan payload is **staged as part files and assembled by a script**, never produced as one large model response.

## Why

`archetipo spec plan` replaces the whole plan, so a naive payload re-emits every task body on every save. Measured on a real run: a 14-task plan cost a single 54k-token response that streamed for six minutes, and the rework of that same plan — 14 carried tasks plus new Fix tasks — never finished: the response was cut off mid-stream after sixteen minutes and the phase lost all its work.

Two properties fix this, and both come from staging:

- **A tool call ends a response.** One part file per unit of content keeps every single response small regardless of plan size.
- **Carried content is never regenerated.** Existing task bodies and the existing plan body are copied verbatim from the CLI. In rework only the new Fix tasks are actually written.

Assembly also removes hand-escaped JSON: part files are plain markdown, and `JSON.stringify` produces the payload.

## Staging layout

Every temporary artifact this skill writes lives under `.archetipo/tmp/` in `data.project_root` — one staging directory per spec, plus the assembled payload beside it. Nothing temporary is ever written outside that root, so a leftover is always visible in one place:

```text
.archetipo/tmp/plan-{US-CODE}/
  plan-body-00-carried.md    # rework only — written by carry-over, do not edit
  plan-body-01-*.md          # written by the worker, merged in numeric order
  existing-tasks.json        # rework only — written by carry-over, do not edit
  task-01.md                 # one file per task the worker actually writes
  task-02.md
```

Ordering: `plan-body*` parts and `task-*` parts are each sorted by the **first number in the filename**, so `task-9.md` precedes `task-10.md` without zero padding. Carried tasks always precede staged ones. A task's `dependencies` may only reference ids defined earlier in the assembled list.

## Task part file format

A `---` delimited header of `key: value` lines, then the markdown body. Only the **first** closing `---` delimits the header, so horizontal rules inside the body are safe.

```markdown
---
id: TASK-15
title: Registro dichiarato dei mesi riservati in tests/e2e/support/date.ts
type: Fix
status: TODO
dependencies: TASK-14
---
## Objective
...

## Read
...

## Change
...

## Steps
...

## Verify
...

## Done
...

## Blockers
None.
```

`id`, `title` and `type` are required. `status` defaults to `TODO`. `dependencies` is a comma-separated list, omitted or left empty when there are none. The body must follow the seven-heading task execution contract from the skill.

## Commands

Run from `data.project_root`. `{SKILL_DIR}` is this skill's own base directory — the parent of the `references/` directory you resolved to read this file, so you already know it. When the assembler is not there, the skill installation is incomplete: stop and report it rather than hand-writing the payload.

```bash
# 1. Rework only: carry over the persisted plan body and every persisted task.
node {SKILL_DIR}/references/assemble-plan-payload.mjs carry-over {US-CODE} .archetipo/tmp/plan-{US-CODE}

# 2. Assemble the payload from the staged parts.
node {SKILL_DIR}/references/assemble-plan-payload.mjs build {US-CODE} .archetipo/tmp/plan-{US-CODE} .archetipo/tmp/payload-{US-CODE}-plan.json

# 3. Validate, then persist.
archetipo validate plan {US-CODE} --file .archetipo/tmp/payload-{US-CODE}-plan.json
archetipo spec plan {US-CODE} --file .archetipo/tmp/payload-{US-CODE}-plan.json

# 4. Clean up both the staging directory and the payload (cross-platform).
node {SKILL_DIR}/references/assemble-plan-payload.mjs clean .archetipo/tmp/plan-{US-CODE} .archetipo/tmp/payload-{US-CODE}-plan.json
```

`build` fails on a duplicate task id, a dependency on an unknown id, a malformed header, or an empty body. It reports the task count and the assembled `plan_body` size — check both before validating.

Semantic validation stays with `archetipo validate plan`: the assembler does not judge task content.

## Recovering an interrupted attempt

The staging directory survives a worker that died mid-phase, and nothing was persisted unless `archetipo spec plan` ran. A later attempt on the same spec may reuse the parts, but must first confirm they belong to the current situation:

1. Run `archetipo spec show {US-CODE}`. If the status is already `{config.workflow.statuses.planned}` and the rework marker is cleared, the previous attempt did persist — do not rebuild, just clean up.
2. Otherwise compare the staged parts against the current spec body: in rework, every `## Rework Feedback` bullet must have a corresponding staged task.
3. Discard any part file that does not match the current feedback, keep the rest, and write only what is missing.

When in doubt, `clean` and stage again. Correctness outranks the saved work.

## The safety net

Running `clean` remains your responsibility and must not be skipped: it frees the staging area as soon as the CLI has answered. But it is no longer the only guarantee. When the spec reaches `{config.workflow.statuses.done}` — through `archetipo spec integrate` or `archetipo spec move --to done` — the CLI itself removes `.archetipo/tmp/plan-{US-CODE}/` and `.archetipo/tmp/payload-{US-CODE}-plan.json`, in the same step that removes the worktree. An attempt that died before reaching its own `clean` therefore cannot leave anything behind past the end of the spec's lifecycle.
