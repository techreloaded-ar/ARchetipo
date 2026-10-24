---
name: archetipo-increment
description: Explore a product problem, build and test a working solution, preserve learning and hand a verified version to developers.
---

# Product work

Read `.archetipo/shared-runtime.md` once. Use the person's language. Start with
`archetipo increment show <id>`; use `-C <workdir>` for all subsequent commands.
The worktree, native session and test data belong to this work throughout its life.
For a record without `development_mode: continuous`, read `references/legacy.md`
and follow that historical delivery contract instead of the product method below.

## Understand and preserve the problem

Read the request, current specification, learning and applicable repository
instructions. Understand whose problem this is, the desired outcome, current
behavior and relevant constraints. Ask only questions needed to make the next
useful decision. Do not assume that a proposed feature is the best solution.

Persist the evolving specification with `increment propose <id> --file -`:

```json
{"intent":"Markdown: problem and people; desired behavior and concrete examples; assumptions; exclusions; decisions and open questions"}
```

No stories, epics, points, tasks or formal implementation plan are required.
The CLI versions the specification and makes it actionable without declaring it
human-approved. Keep the specification consistent with decisions; never rewrite
requirements to conceal a failed implementation or test. Preserve unknowns.

An investigation may conclude that no software change is needed. Record useful
observations with `increment learn <id> --file -`:

```json
{"observation":"What was observed, source and limits","decision":"Optional decision and its rationale"}
```

To conclude without publishing code, use `increment conclude <id> --file -` with
the same payload and a required decision. Do this when the person agrees to the
conclusion or explicitly delegated that decision. Do not equate technical success
with validated business value. Never invent user research or observations.

## Build and try

Implement directly in this worktree using the project's architecture, design and
checks. Read `.archetipo/CUSTOM_INSTRUCTIONS.md` and `docs/DESIGN.md` if present.
Use a short internal plan only when useful. Do not invoke delivery skills or create
an internal backlog merely to satisfy a workflow. Give concise progress updates
about behavior and what can be tried; keep logs in original evidence files.

Prepare/reuse the project's environment recipe; delegate `/archetipo-environment`
when configuration or repair is needed, preserving its technical session identity.
Use managed resources, assigned ports and synthetic data. Never touch production
services, copy secrets, start unowned background processes or reset demo data just
because a revision starts. A missing capability is a concrete blocker.

For long-running work, use `execution workflow start --file -` with
`{"skill":"archetipo-increment","invocation":"/archetipo-increment <id>","scope":"<id>","conversation_id":"supplied conversation, omit if absent","state":{"phase":"building","revision":1}}`.
Use `execution workflow list`, `show <run-id>` and `checkpoint <run-id> --file -`
to resume. A checkpoint passes the last observed `revision`, `status`
(`running`, `blocked`, `complete`), `phase` and full `state` with original evidence
references and worker ownership. These return `kind: workflow_run` in the standard
envelope. Keep one run and bounded retry counters across recovery; complete it
after `increment ready` or `conclude` succeeds. Store decisions, not copied logs.
Respect existing ownership and retry budgets; never replace the native session
with a transcript. For a small change, work directly in this session.

Verify changed behavior with appropriate real browser, API and database checks.
Reuse evidence only when its inputs remain valid. Review the combined change;
use independent assessment for material risk such as permissions, migrations,
concurrency or data loss. Keep unrelated failures distinct from regressions.
Do not run a full ceremony per feature or repeat setup/checks without a reason.

Commit the verified product changes on this branch, excluding runtime and secrets.
Call `increment ready <id> --file -`:

```json
{"summary":"What can be tried, instructions and known limits","checks":["Actual check and result; original evidence path"],"preview_url":"http://127.0.0.1:3000"}
```

The preview URL is optional and must be observed, never invented. Configure a
managed preview recipe through the environment method for a web app. The viewer
launches the app through that same deterministic contract. Readiness freezes a
clean committed candidate and the current specification; no story state is needed.

## Learn, adapt and hand over

A question alone preserves the specification and candidate. An explicit requested
change after readiness is recorded with `increment feedback <id> --file -`:

```json
{"feedback":"Requested behavior and any selected page/element reference"}
```

Then update the specification, implement and verify in this same work. Keep
observations and decisions with `learn` when they add product knowledge. Do not
create another environment or require another approval for ordinary iteration.
Do not expand scope beyond the person's request without discussing it.

Only publish after explicit human approval of the displayed tested version:
`increment publish <id> --file -` with `{"revision":N,"head_sha":"tested commit"}`.
The PR carries the specification and evidence. Developers may evolve its code;
that does not retroactively approve or verify their commits. `increment refresh`
reads the actual PR state. Never merge or deploy on behalf of this method.

## Deterministic contract

All increment operations return `archetipo/v1`, `kind: increment`, record in `data`.
Parse errors and warnings before dependent writes. `E_INVALID_INPUT` means repair
the payload; `E_CONFLICT` means reread the revision, ownership or candidate;
`E_PRECONDITION` means resolve a missing prerequisite. Other failures are blockers,
not permission to write workflow files directly. Only the CLI persists artifacts.
