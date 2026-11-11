---
name: archetipo-implement
description: Implement a planned backlog story by carrying out its persisted plan in the project checkout, then hand it to the person for review. Use when someone asks to implement or resume a PLANNED or IN PROGRESS story. Not for product increments, discovery or planning.
---

# Story implementation

Read `.archetipo/shared-runtime.md` exactly once. Use the person's language.
This is the classic backlog path: plan, implement, review by the person. It is
not product increment work. Do not create an increment, a sandbox or a pull
request, do not ask for a specification approval, and do not create a branch or
worktree yourself: the CLI creates a per-story worktree only when the workspace
configuration enables it.

Read `archetipo config show` (`kind: setup`) and keep `data.project_root` as the
directory for every connector command below. Load the story with
`archetipo spec show <code>` (`kind: spec`); use `spec next --status planned`
only when no code was given. `E_PRECONDITION` means there is no eligible story:
say so and stop. A story with no `data.tasks` has no plan: ask the person to plan
it first and stop. A persisted plan is the approved scope; start implementing
without asking for confirmation.

Run `archetipo spec start <code>`. It moves the story to IN PROGRESS, is a safe
no-op when it already is, and may have created the configured worktree, so run
`archetipo spec show <code>` again and use that envelope from here on. Follow
the shared **Worktree Working Directory** rule: read, edit and run everything
for this story under `data.workdir`, which is the project root unless a
worktree exists. Never branch on connector type or on the worktree setting.

Read the project instructions and the code the tasks name; load only what
matters. Carry out the tasks in dependency order, skipping those already `DONE`
and preserving their work. A task body is its contract: follow Objective, Read,
Change, Steps, Verify, Done and Blockers. Reuse the project's patterns and add
no scope beyond the story. Adapt locally when the code differs from the plan
without changing the intended solution. Ask the person only when a task would
change scope, acceptance criteria or a user-facing contract, or when it depends
on work outside this story.

Verify each task as its Verify section says, following the shared **Reusable
project verification** rules when the project has a recipe. Do not invent test
results; report a check you could not run as not run. Only after its
verification passes, run `archetipo task done <code> <TASK-ID>`
(`kind: write_result`). After the last task, run the checks the plan requires
for the whole story against the final code and fix what this work broke.

When every task is `DONE`, hand the story over with
`archetipo spec review <code> --commit-type <type> [--commit-summary "<summary>"]`,
piping a short Markdown summary on stdin: what changed, the checks run with
their outcome, and how the person can try the result. `<type>` is the
Conventional Commit type of the work (`feat`, `fix`, `refactor`, `test`,
`docs`, `build`, `ci`, `perf`, `chore`). The CLI decides whether anything is
committed; do not commit, push, merge or integrate yourself. The story ends in
REVIEW: never move it to DONE, because the person tries the result and decides.

Branch on `error.code`: `E_INVALID_INPUT` means repair the command;
`E_PRECONDITION` means resolve the missing prerequisite; `E_CONFLICT` means
reread state before retrying. Do not write connector files.

Finish with a brief explanation and the verified receipt line:
`{"spec_code":"US-001","status":"REVIEW","tasks_done":3,"tests":"one line on the final checks"}`
using actual values, only after `spec show` reports the story in REVIEW.
This receipt also works when the skill is invoked standalone.
