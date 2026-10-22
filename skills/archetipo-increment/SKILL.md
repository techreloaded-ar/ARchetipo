---
name: archetipo-increment
description: Guide an isolated product increment from a business intent through development, feedback and one final PR. Keep ordinary conversations and standalone spec delivery unchanged.
---

# Product increment

This skill owns discovery, delivery and business approval for an isolated increment. It works standalone
and inside a native View conversation. Never assume View is running.

Read `.archetipo/shared-runtime.md` exactly once at activation. Use the
workspace language. Read context progressively; avoid unrelated backlog work.

## Resolve the increment

The argument is the increment ID. Run `archetipo increment show <id>`.
Success is the standard `archetipo/v1` envelope, `kind: increment`, with
the record in `data`. Its `workdir` is the only code/workflow root for
this increment. Inside it, ordinary CLI commands use the isolated file
connector; increment commands resolve its owning record automatically.
Always pass `-C <workdir>` for subsequent CLI calls.
The sandbox marker resolves inherited configuration even when no local
`.archetipo/config.yaml` is present. Use `archetipo config show` to inspect the
effective isolated settings; do not copy the parent configuration or infer a
missing configuration from the absence of that file.

Without an ID, discover the idea in conversation first. When the person asks
to begin an isolated increment, use `increment create --file -` with
`{"title":"…","request":"…"}`, then continue in its returned workdir.
The CLI creates a dedicated branch and worktree from local `main`. If `main`
is unavailable, report the prerequisite instead of using another branch.
This does not approve the intent or authorize publication.

Never retarget an existing native session to another directory. A standalone
agent that cannot work in the returned directory must report that limitation;
a View conversation is already created in that sandbox. Never seed/resume a
native session by copying a transcript.

## Discovery and feedback

When `status` is `DISCOVERY`, interview the person briefly, using the
installed archetipo-spec skill in its proposal-only mode: clarify benefit, users, scope, exclusions
and a concrete demonstration. That skill receives the request and project context,
produces a proposal without persisting it, and needs no knowledge of this controller.
Ask one focused question at a time. For a
feedback round, concentrate on the requested delta and show its consequences.
Do not write product code before the latest intent is approved.

Propose the complete current intent and story set with:

`archetipo -C <workdir> increment propose <id> --file -`

Payload:

    {"intent":"Complete agreed intent in Markdown","stories":[{"code":"US-001","title":"…","epic":{"code":"EP-001","title":"Increment title"},"priority":"MEDIUM","points":3,"status":"TODO","scope":"MVP","blocked_by":[],"body":"Complete story with Demonstrates and - [ ] acceptance criteria"}]}

This persists a proposal only. Preserve local story codes across title or
requirement changes, preserve unchanged stories, and omit retired stories from
the current set. Never invent a main-backlog US code: local codes are sandbox
identities. Use small, independently demonstrable stories and existing spec
validation rules. History is retained by the CLI.

Explain the proposal and wait for the human's approval. The View approval
button calls `increment approve <id> --file -` with
`{"revision":N}`. Standalone, call it only after the person explicitly
approves that exact displayed revision. The CLI reconciles sandbox stories,
preserving unchanged work and invalidating changed/retired work artifacts.
It never updates the parent backlog.

## Develop and prepare the result

### Execution ownership and checkpoints

After the intent is approved, own delivery until `increment ready` has frozen the
verified result. A child phase finishing is not the end of this invocation. Keep
human product decisions at discovery/intent approval and verified-result review.
Local setup, failed checks and recoverable worker failures belong to delivery.

Use the shared checkpoint protocol, with `skill: archetipo-increment`, `scope: <id>`,
`invocation: /archetipo-increment <id>` and the supplied `Workflow conversation`
binding when present. This skill uses
`execution workflow start --file -`, `list`, `show <run-id>` and
`checkpoint <run-id> --file -`.

Start payload:

```json
{"skill":"archetipo-increment","invocation":"/archetipo-increment <id>","scope":"<id>","conversation_id":"optional supplied Workflow conversation","state":{"revision":1,"stories":["US-001"],"phase":"implementation","review_attempts":0,"recoveries":{}}}
```

Omit the binding when not supplied. Checkpoint with the last observed `revision`,
`status` (`running`, `blocked` or `complete`), `phase`, `worker_id`, the full `state`
and `last_error` for a blocker. All four operations return `archetipo/v1`,
`kind: workflow_run`, with the record in `data` (an array for list).
`E_INVALID_INPUT`: repair input; `E_NOT_FOUND`: resolve the run; `E_CONFLICT`:
reread/reconcile the revision or conflicting scope/binding; `E_INTERNAL`: preserve
state and report the failure. Never edit checkpoint files directly.
Its `state` holds the approved intent revision, selected story codes, delivery
phase (`implementation`, `technical-review`, `verify`, `ready`), worker IDs,
review attempts, recovery counts, diff base/candidate and evidence references. Store delegated worker
IDs before waiting. On resume, reconcile CLI story states, partial changes and
existing workers before dispatching work; preserve approval and completed work.
An older checkpoint with phase `autopilot` is a recovery input: reconcile its
existing workers and nested run, retain findings and consumed review attempts,
then continue the unfinished work with the methods below. Do not invoke another
coordinator, duplicate a live worker or reset an exhausted review budget.

Allow at most three recovery attempts per failed phase, retaining their counts
across resume. Technical review allows an initial assessment and at most three
correction rounds. Record the chosen review scope, all blockers and consumed
attempts before starting another round. An exhausted budget, unresolved product
ambiguity or unavailable required capability is a concrete `blocked` checkpoint,
with remaining findings and the minimum intervention. Never mark a run complete
until `increment ready` succeeds.

If an interruption occurred after `increment ready` persisted but before this
checkpoint closed, reread the increment. A frozen result already in `REVIEW`
settles this delivery run as complete before waiting for the business verdict;
do not rerun development or publication to repair bookkeeping.

Apply the shared local verification environment policy when preparing tests and
preview. The methods and workers receive ordinary project/spec context and generic
execution constraints. Keep business state and approvals in this coordinator.
For a browser preview, first read `execution preview show` in the code checkout.
Reuse a valid project recipe instead of rediscovering startup at every increment.
When creating or repairing it, persist through `execution preview configure
--file -` a version 1 JSON object with `directory` relative to that checkout,
explicit `argv`, a loopback HTTP `url`, and `prepare_environment: true`. Use
`{port}` in the URL port and launch arguments, with the project's strict-port
option or equivalent so it cannot silently switch endpoints. Keep this recipe,
`.archetipo/environment.json` and setup helpers versioned with product changes;
do not copy runtime checkpoints, credentials or dependencies between checkouts.
`show`/`configure` return `kind: preview_recipe`; `execution preview prepare`
returns `kind: preview` with `data.preparation` containing the standard environment
result. Require `passed && stable` and inspect its report (including original
evidence on reuse). Preparation does not start the preview server. Missing recipe
is `E_PRECONDITION`, invalid configuration is `E_INVALID_INPUT`, and concurrent
ownership is `E_CONFLICT`; repair the specific cause rather than starting another
setup. The viewer's Start and standalone `execution preview run` use the same
preparation contract when the flag is enabled; neither needs a new agent turn.
Preserve the model and options explicitly requested by the person across
delegation, resume and recovery.

### Keep delivery context bounded

At approval, feedback or recovery, resolve the latest persisted increment and
workflow before choosing work. Carry the approved revision, selected story codes,
roots, diff base/candidate, applicable product decisions, unresolved questions,
worker ownership and original evidence references in the existing checkpoint.
Keep references and concise decisions there, not copies of the proposal, reports
or conversation. Current approved stories remain authoritative for scope; retain
relevant product context beyond their text and do not silently discard a decision
merely because it is absent from an acceptance criterion.

A long discovery conversation is not a useful implementation handoff. When it
contains substantial document/tool output and a bounded implementation scope
benefits from delegation, prefer a worker with its own context; use the harness's
documented non-inheriting spawn option when available. Supply the scope and exact
source paths below, not the discovery transcript. Use the active runtime's
documented native tools: do not assume a shared
spawn flag, fork mode or compaction command. Keep this conversation and its
native session as coordinator; do not spawn another coordinator or force workers
for a small change. Without that capability, work inline with targeted reads.
Context isolation never removes required instructions, permissions or the chosen
model/options from a worker.

Load only the method needed for the active phase. Reuse unchanged documents and
envelopes already read in this activation; after edits or a revision change,
reread the affected sources. Handoffs name outcomes, blockers and original
reports; inspect targeted evidence rather than echoing full logs, proposals or
source into messages. Expand reads when a concrete concern requires it. Full
diagnostics and acceptance evidence must remain available at their original paths.
These rules apply equally standalone and in View. They limit new context growth;
they do not compact existing native history. Never reset the session, replace it
with a transcript/summary, or claim compaction the harness did not perform.

### Delivery workflow

When `status` is `DEVELOPING`, read and apply
[implementation](references/implementation.md) for the approved local stories.
Then read and apply [technical review](references/technical-review.md) to the
combined change. These are internal methods, not separately installed skills.
No formal plan, synthetic task list or additional delivery skill is required.
Keep stories independently demonstrable and individually tracked; choose a
bounded working group from dependencies, overlap and available context. Do not
collapse their acceptance criteria into an undifferentiated assignment.

Choose whether to use workers and parallelism. Delegate independent scopes when
useful; coordinate shared files and serialize CLI state changes and commits.
Avoid a second coordinator that merely relays the same work. Workers receive the
absolute code/CLI roots, selected stories, approved intent, relevant product
context, applicable instructions, explicit diff base (the increment BaseSHA for
the full result), candidate and evidence,
language, execution constraints and model/options; no business-state dependency.
Preserve worker ownership and await terminal results before handing over their
files. A worker's completion is an internal handoff: continue until ready or blocked.

Review the combined result once per working group, rather than automatically
opening a review session per story. Use a fresh independent reviewer for material
risks or uncertain evidence (for example access control, data loss, migrations,
concurrency or cross-story behavior). For a small low-risk change, an in-context
assessment is sufficient; disclose that it was not independent. Persist this
choice and its reason before review. Reuse valid original evidence; independence
does not require repeating all tests. On blockers, use the reference's rework
contract and the same checkpoint/budget, then recheck fixes and affected flows.
A required independent review unavailable in the harness blocks completion.

Only stories with sufficient implementation, evidence and no unresolved technical
blocker move to DONE. Verify persisted effects through the CLI; reuse immediately
observed states when no intervening write occurred. `increment ready` enforces the
approved story snapshot, completion of any existing tasks and a clean committed
result. Technical completion does not authorize publication or replace the final
human result approval. When wiki.enabled is false, perform no automatic Wiki work.

For a revised intent, reconcile the whole code diff with the current outcome.
Removing a story does not remove its implementation automatically: remove or
adapt obsolete code, tests and docs, then verify the combined result.

Use the existing verified reports and prepare a demonstration appropriate to the
changed surface. A UI change needs an interactive preview when runnable. For an
API/CLI-only change, reproducible real request/response or command traces already
produced by the accepted tests are a valid demonstration. Do not invent an ad-hoc
persistent preview or rerun those same requests solely to duplicate that evidence,
unless the person requested a running endpoint or the project already provides
its documented preview command. Re-run
checks only when inputs changed, evidence is missing, or a specific concern needs
reproduction. Batch independent final state/diff/report reads instead of serial
rediscovery. Use the project's
documented preview command when it exists; otherwise inspect the project and
attempt the permitted local setup before reporting missing startup information.
Never invent a preview URL, a passing check, or a deployment. A preview uses separate test data and local services; never copy
production credentials or point it at production resources.

Commit any remaining verified product changes on the existing increment branch; do not create another commit when the accepted branch is already clean. Keep
runtime journals and sandbox process files out of commits. Then call:

`archetipo -C <workdir> increment ready <id> --file -`

Payload:

    {"summary":"What the person can now try, with demonstration instructions","preview_url":"http://127.0.0.1:3000","checks":["Exact command — actual result"]}

preview_url is optional and must be a real local HTTP preview. An unavailable
interactive preview must be stated plainly in the summary; provide other
appropriate observable evidence. Every story and every existing task must be
complete (a plan is optional), the accepted story content must match, and the
product working tree must be clean.
The CLI freezes the verified code and story snapshot.

## Business review, another round, publication

At `REVIEW`, present the result, its evidence and any limitations. Wait.
An ordinary question about the result does not invalidate it. When the
person explicitly requests a change, both in View and standalone, call `increment feedback <id> --file -` with
`{"feedback":"The person's requested changes"}`, then repeat the shorter
discovery and exact-revision approval. All rounds stay in this increment.

Only after explicit human approval of the verified result may standalone
execution call `increment publish <id> --file -` with
`{"head_sha":"ready_sha from the displayed record","revision":N}`.
View performs this operation deterministically from its approval button.
Never create a PR yourself with another command, publish early, or merge it.
The CLI opens/reuses one GitHub PR targeting `main` using gh and publishes the current stories
through the original connector as proposals. Retrying the same publication
must use the same increment.

For `PUBLISHING`, explain any publication failure and retry only after an
explicit user request; never automatically retry uncertain native deliveries.
For `PROPOSED`, answer questions without changing the approved code/stories.
Call `increment refresh <id>` when requested to reconcile the actual PR:
stories become delivered only after GitHub confirms its merge and approved
head. `DELIVERED` is complete; keep the history inspectable.

## CLI errors and output

All commands above return `kind: increment` and the current record.
Read-only `increment list` returns an array of records for the current
workspace. Use it to locate an already-created increment after interruption.

Branch on `error.code`: `E_INVALID_INPUT` means fix the payload,
`E_PRECONDITION`/`E_NOT_FOUND` means restore missing prerequisites or
select the right increment, `E_CONFLICT` means reread current state and
resolve the named stale approval/working-tree/publication conflict,
`E_CONNECTOR`/`E_NETWORK`/`E_AUTH_SCOPE` means report the external
failure without claiming success, and `E_INTERNAL` means stop and report.
Never bypass a refused approval by writing JSON or backlog files directly.

Show readable product outcomes in the conversation. Do not paste CLI envelopes
or technical receipts into user messages. The persisted record is the effect
verification for this conversation; these internal methods emit no standalone
implement/review receipt. Existing workflow actions retain their own contracts.
