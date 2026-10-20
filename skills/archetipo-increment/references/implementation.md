# Implementation method

Use this method for an approved, bounded set of stories in an existing delivery
checkout. Inputs are the selected story codes, absolute project/code roots,
applicable instructions, diff base, execution constraints and any retained evidence.
The caller owns approvals, checkpoints, retry budgets and final publication.
Inherit its already-loaded shared runtime; a fresh worker reads that runtime
exactly once. Use the workspace language for human-facing output.

## Implement the selected outcome

Read `archetipo -C <project-root> config show` and `spec show <code>` for the
selected stories. Reuse supplied current envelopes when no write intervened.
Read applicable repository instructions, including `AGENTS.md`. Within the delivery
checkout (`data.workdir`), read `.archetipo/CUSTOM_INSTRUCTIONS.md` and
`docs/DESIGN.md` when present. These are exact, case-sensitive paths relative to
that checkout; do not search for namesakes or case variants elsewhere. Follow any
additional documents explicitly referenced by these files. Their absence alone
is not a blocker. Keep these project documents versioned so isolated checkouts
receive them; never substitute an uncommitted copy from another checkout.
Follow existing code, architecture and test conventions; report conflicting
requirements.

Use the caller's bounded scope and source references, not its conversation
history. Read the selected stories and applicable instructions from their original
sources; a handoff summary never replaces acceptance criteria or product decisions.
Reuse unchanged material already read in this activation. Inspect relevant source
and targeted report sections before expanding to entire directories or logs.
Keep full output in the original evidence files and return concise results with
paths, tested candidate and blockers rather than copying that output to the caller.

Choose the working order and whether independent work benefits from parallel
workers. Keep each story's outcome identifiable while sharing setup and combined
verification. Assign distinct file ownership or coordinate overlapping edits;
the caller serializes CLI transitions and commits after writers have finished.
Do not create a formal plan or tasks solely to satisfy this method. If tasks
already exist, fulfill their contracts and dependencies; never mark them complete
just to clear a gate. Preserve completed work and inspect partial edits on resume.

This method uses in-context delivery (`data.worktree.enabled: false`); if another
mode is configured, return that prerequisite to the caller before changing state.
For each selected TODO/PLANNED story, run `spec move <code> --to in_progress`, then
reread `spec show` and use its `data.workdir` for code, commands and evidence.
Resume IN PROGRESS stories without restarting them; leave REVIEW stories for
assessment and preserve DONE stories unless later edits invalidate their evidence.
Use the caller's explicit diff base for the combined change: a planless board move
does not record a per-spec fork base. Implement the approved behavior, tests and
necessary documentation. Do not change the accepted story body or add product
scope to explain away a failing check.

Use the shared local environment policy. Inspect the existing recipe once;
prepare and reuse owned local services and synthetic data. Use existing project
scripts and test tools. Keep temporary verification helpers outside application
source discovery unless they belong in the product; do not change compiler or
build inputs merely to run a review helper.

Verify the changed behavior and the project's required checks. Include real
browser/API/database checks where the behavior requires them. Preserve full
reports, tested source, exact commands and results. Reuse a valid named check
against unchanged declared inputs; rerun affected checks after edits, not every
suite at each handoff. An observed failure or missing evidence must be resolved
or returned as a blocker, with its original diagnostics.

With automatic Wiki enabled, respect existing impact contracts and inspect the
exact changed paths using `wiki affected --file <path>` (repeat the flag for each
path, including deletions). Inspect the affected claims, update only inaccurate
or required pages and retain the required page IDs in the handoff. Reset reviewed
pages before editing; leave changes generated, without approval metadata. Run
`wiki catalog`, `wiki validate --profile bootstrap` and
`wiki status --require-generated <required-id>...` when there are required pages.
Never fabricate review metadata or edit generated index/log files directly.
With the gate disabled, perform no Wiki work.

After verification, complete any existing verified tasks using
`task done <code> <task-id>`. Hand each implemented story to technical review with
`spec review <code> --file <summary.md>`; the file is plain Markdown containing
the delivered outcome, exact check results/report paths and remaining concerns.
This operation may commit changes: first await all writers and inspect the
candidate files to keep runtime journals, secrets and unrelated changes out.
Do not mark stories DONE during implementation.

Return a compact handoff: story states, changed areas, original evidence paths,
environment/preview ownership and replay commands, required Wiki IDs if enabled,
and unresolved risks. Do not generate a dossier or acceptance-criteria matrix.

## CLI contract

All calls use `archetipo -C <project-root>`; environment operations use the actual
code root. Parse the `archetipo/v1` envelope and warnings before dependent calls.
`config show` returns `kind: setup`; `spec show` returns `kind: spec` with
`data.spec`, `data.tasks`, `data.plan_body`, `data.workdir`. `spec move`,
`task done` and `spec review` return `kind: write_result`. Workflow artifacts are
persisted only through the CLI, without connector-specific branches.

Environment commands used here:

- `execution environment show`
- `execution environment configure --file <recipe.json>`: version 1 JSON with
  `version`, `inputs`, `outputs`, `env_keys`, `prepare`, `probe`, `cleanup`,
  `checks`; commands contain `name`, `argv`, `timeout_seconds`; checks also
  declare `inputs` and `cache`.
- `execution environment prepare`
- `execution environment check <name> --spec <code> [--fresh]`

These return `kind: verification_result`. Show/configure return the recipe;
prepare/check return `data.passed`, `stable`, `reused`, `report_path`, original
`reused_report` and full step logs/exit codes. Require `passed && stable` and read
the original report on reuse. Configure only missing/changed commands or inputs;
`--fresh` is for a specific evidence or reproduction need.

Conditional Wiki calls use `wiki --project-root <code-root>`:
`affected` returns `wiki_affected_result`, `reset <id>...` returns
`wiki_reset_result`, `catalog` returns `wiki_catalog_result`, `validate` returns
`validation_result` (`data.ok` must be true), and `status` returns `wiki_status`.
The required-generated gate fails with `E_CONFLICT` for missing or ineligible IDs.

Branch on `error.code`: `E_INVALID_INPUT`/`E_INVALID_USAGE`/`E_VALIDATION` repair
input; `E_NOT_FOUND` resolve the selected artifact; `E_PRECONDITION` repair the
specific missing prerequisite; `E_CONFLICT` reread state or await its live owner.
`E_CONNECTOR`/`E_NETWORK`/`E_AUTH_SCOPE` report the external blocker;
`E_INTERNAL` or persistence failure prevents a completed handoff. A successful
envelope with `passed: false` is a failed check, not a pass.
