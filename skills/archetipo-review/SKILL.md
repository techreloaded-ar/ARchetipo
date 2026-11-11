---
name: archetipo-review
description: Prepare the acceptance review of a backlog story in REVIEW and carry out the person's verdict. Compares the delivered work with the acceptance criteria, then approves it (DONE) or sends it back with structured rework feedback. Use when someone asks to review, accept, approve or reject a story waiting in REVIEW. Not for product increments, code review during implementation or planning.
---

# Story acceptance review

Read `.archetipo/shared-runtime.md` exactly once. Use the person's language.
This is the last step of the classic backlog path: plan, implement, review. The
verdict belongs to the person: gather the evidence, present it, and execute only
the decision they give. It is not product increment work: do not create an
increment or a pull request.

Read `archetipo config show` (`kind: setup`) and run every connector command
from `data.project_root`. Load the story with `archetipo spec show <code>`
(`kind: spec`); use `spec next --status review` only when no code was given.
`E_PRECONDITION` means nothing is waiting for review: say so and stop. A story
in another status is not reviewable: name its status and the step that handles
it, then stop. Follow the shared **Worktree Working Directory** rule and read
the delivered code under `data.workdir`.

## Evidence

Build a compact dossier; read surgically and do not paste raw diffs.

- **Promise**: the story and each acceptance criterion in `data.spec.body`.
- **Work**: completed and open tasks in `data.tasks`. An open task is a finding.
- **Diff**: with `data.spec.branch`, `git diff <fork_base>...<branch> --stat`;
  otherwise the changes live in the project checkout, and `archetipo view`
  shows them against the recorded base.
- **Checks**: reuse the project recipe through the shared **Reusable project
  verification** rules. Read the original reports; report a check you could
  not run as not run. Absent evidence for a promised behaviour is a finding.

Judge each criterion `met`, `unclear` (evidence found, question not settled) or
`not_verifiable` (no evidence of that kind). A **blocker** makes the work
unacceptable as delivered: a failing build or check, a contradicted criterion,
promised evidence that is absent. Everything else worth reading is a **minor
finding** and never prevents approval. When unsure, ask whether you would send
the story back for that item alone.

With `data.wiki.enabled` true only, the shared **Required pages** are part of
the verdict: check them with `archetipo wiki --project-root <workdir> status`
and `validate --profile bootstrap`; a required page that is absent, stale or
invalid is a blocker. With the gate off, do no Wiki work and say nothing of it.

## Prepared dossier mode

Use this mode only when the invoking prompt asks for "prepared dossier mode",
as the viewer does: a person decides there from the panel. Do not ask for a
verdict and never run `spec move`, `spec integrate`, `spec request-changes` or
`wiki approve`. Write the payload with your file tool to
`.archetipo/tmp/payload-<code>-dossier.json`:

```json
{"execution_id":"<id given by the prompt, omitted when none>","summary":"<what the work delivers>","criteria":[{"id":"AC-1","verdict":"met","note":"<what proves it>"}],"blockers":[],"minor_findings":[]}
```

Run `archetipo spec review-dossier <code> --file <payload>` (`kind:
write_result`), delete the file, and confirm with `spec show` that the story is
still in REVIEW. Finish with the receipt as the last line:
`{"spec_code":"US-001","status":"REVIEW","criteria":3,"blockers":0}`
using actual counts.

## Verdict

Otherwise present the dossier, one line per criterion, then findings and how
the person can try the result. Ask for exactly one of **Approve**, **Request
changes** or **Postpone**; with blockers, say that approving closes the story
over them. An approval with conditions is a request for changes.

- **Approve**: with the Wiki gate on, first run `archetipo wiki --project-root
  <workdir> approve <required-ready ids>` and, in a worktree, commit those Wiki
  files on the story branch. Then run `archetipo spec integrate <code>` when
  `data.spec.branch` is set and `data.worktree.enabled` is true, otherwise
  `archetipo spec move <code> --to done`. On `E_CONFLICT` from the integration
  report the blockers or conflicting files and stop; never merge by hand.
  Confirm DONE with `spec show`.
- **Request changes**: write `{"comments":[{"file":"src/app.js","line":12,"body":"<what to change and why>"},{"body":"<general feedback>"}]}`
  to a temporary file, with anchors only where the feedback names a place, run
  `archetipo spec request-changes <code> --file <payload>` and delete the file.
  The story returns to TODO in rework; the next planning turns each item into
  a task.
- **Postpone**: change nothing.

Branch on `error.code`: `E_INVALID_INPUT` means repair the command;
`E_PRECONDITION` means resolve the missing prerequisite; `E_CONFLICT` means
reread state before retrying. Do not write connector files.
