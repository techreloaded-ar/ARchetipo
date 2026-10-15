---
name: archetipo-increment
description: Guide an isolated product increment from a business intent through development, feedback and one final PR. Keep ordinary conversations and standalone spec delivery unchanged.
---

# Product increment

This is a thin composition of ARchetipo's existing methods. It works standalone
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

Without an ID, discover the idea in conversation first. When the person asks
to begin an isolated increment, use `increment create --file -` with
`{"title":"…","request":"…"}`, then continue in its returned workdir.
This does not approve the intent or authorize publication.

Never retarget an existing native session to another directory. A standalone
agent that cannot work in the returned directory must report that limitation;
a View conversation is already created in that sandbox. Never seed/resume a
native session by copying a transcript.

## Discovery and feedback

When `status` is `DISCOVERY`, interview the person briefly, using the
discovery method of archetipo-spec: clarify benefit, users, scope, exclusions
and a concrete demonstration. Ask one focused question at a time. For a
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

When `status` is `DEVELOPING`, use the existing skills rather than a
second implementation of their process. Invoke
`/archetipo-autopilot <current local story codes>` in the sandbox to
compose planning, implementation and autonomous technical acceptance. Only
the approved local stories are eligible. The sandbox disables per-spec
worktrees, so acceptance stays on the increment branch and cannot integrate
into the original branch.

Read the existing skills' own contracts. Keep human product acceptance here:
technical acceptance of a local story is not delivery into the product.
When wiki.enabled is false, perform no automatic Wiki work.

For a revised intent, reconcile the whole code diff with the current outcome.
Removing a story does not remove its implementation automatically: remove or
adapt obsolete code, tests and docs, then verify the combined result.

Run relevant checks and prepare a concrete demonstration. Use the project's
documented preview command when it exists; otherwise ask for missing startup
information when necessary. Never invent a preview URL, a passing check, or
a deployment. A preview uses separate test data and local services; never copy
production credentials or point it at production resources.

Commit the verified product changes on the existing increment branch. Keep
runtime journals and sandbox process files out of commits. Then call:

`archetipo -C <workdir> increment ready <id> --file -`

Payload:

    {"summary":"What the person can now try, with demonstration instructions","preview_url":"http://127.0.0.1:3000","checks":["Exact command — actual result"]}

preview_url is optional and must be a real local HTTP preview. An unavailable
interactive preview must be stated plainly in the summary; provide other
appropriate observable evidence. Every story and task must be complete, the
accepted story content must match, and the product working tree must be clean.
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
The CLI opens/reuses one GitHub PR using gh and publishes the current stories
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
verification for this conversation; each existing workflow action retains its
own receipt contract.
