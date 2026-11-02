---
name: archetipo-plan
description: Prepare or revise an optional, concrete plan for a backlog story when someone asks to plan it.
---

# Story planning

Read `.archetipo/shared-runtime.md` exactly once. Use the person's language.
Planning is an optional project management tool. Product work can proceed from
its evolving specification without this skill, a task board or a planning gate.

Read `archetipo config show` (`kind: setup`) and `archetipo spec show <code>`
(`kind: spec`); use `spec next --status todo` only when asked to select the
next story. Use returned project/work directories, original story, existing tasks,
project instructions and relevant code/docs as authority. Load only what matters.
Do not introduce a virtual team, separate design worker or implementation workflow.

Describe the problem, proposed approach, dependencies, material risks and how the
result can be tried. A plan explains decisions; it does not prewrite all the code.
Define a small set of concrete tasks with stable IDs and observable completion.
Preserve completed tasks and their original bodies during revisions. A request to
revise a plan authorizes that revision; ask only about conflicting scope or a
replacement the person did not request. Do not invent estimates or test results.

For each task use these Markdown headings: Objective, Read, Change, Steps, Verify,
Done, Blockers. Name relevant paths and verification, keeping the task compact.
If a check is not performed, mark it pending. Environment setup belongs to the
project recipe; do not provision a second environment to write a plan.

Persist through `archetipo spec plan <code> --file -` with:

```json
{"plan_body":"Markdown approach, decisions, risks and verification","tasks":[{"id":"TASK-01","title":"Concrete outcome","type":"Test","status":"TODO","dependencies":[],"body":"Markdown task with the seven headings"}]}
```

Include at least one `Test` task, as required by the plan validator; use `Fix` for a correction. Dependencies reference earlier task IDs.
The command replaces the complete plan: carry existing content directly from the
CLI rather than regenerating it. For a large payload, load
`references/payload-assembly.md` to stage and assemble small parts.

Parse the standard `archetipo/v1` envelope, `kind: write_result`, and reread the
story to verify persisted tasks. Branch on `error.code`: `E_INVALID_INPUT` means
repair the payload; `E_PRECONDITION` means resolve the missing prerequisite;
`E_CONFLICT` means reread state before retrying. Do not write connector files.

Finish with a brief explanation and the verified receipt line:
`{"spec_code":"US-001","status":"PLANNED","tasks":1}` using actual values.
This receipt also works when the skill is invoked standalone.
