# Spec proposal flow

Use this flow for one or more stories proposed for review, before persistence. It works standalone and when another caller requests a structured receipt.

The shared runtime has already been read by the entry skill; do not read it again.

## Read the workspace

Run `archetipo config show`. Expect the standard `archetipo/v1` JSON envelope with `kind: setup` and a `data` setup object carrying the project root, paths and workflow configuration.

Run `archetipo spec list`. Expect the same envelope with `kind: backlog`, `data.items` containing full specs and `data.summary.epics` containing declared epics. Null or empty lists are empty lists. If the command fails with `error.code: E_PRECONDITION` because the backlog is missing, proceed with an empty backlog. For other error codes, follow the shared runtime's error rules; do not treat an unreadable backlog as empty.

## Discover the intended increment

Use the core backlog method. A request to evaluate an idea is exploratory even when the caller
asks for a single proposed story. Agree on the outcome and scope before writing
acceptance criteria. No artifact or receipt is due while a question is pending.

## Propose the agreed stories

- Use the repository and the person's request as context. No PRD, populated backlog or existing epic is required. Do not redirect to inception or generate a whole backlog.
- Use the agreed outcome and scope. Include a user story, a concrete demonstration and verifiable acceptance criteria.
- Reuse a declared epic with `epic_code`, or propose a new epic name with `epic_title` and an empty `epic_code`. Leave new epic and story codes unassigned until confirmation.
- Do not run `archetipo spec add` or write backlog/spec files. Present the proposal for review.
- Give each story a stable `draft_id` such as `story-1`; keep that identity
  when changing its title or order. A new story still has no assigned code.
- When the person asks to revise a story already accepted into the backlog,
  read its current content with `archetipo spec list`, retain its `draft_id`
  when known and set `revises_spec` to its existing code. Do not infer a target
  from array positions. If a renamed legacy proposal has no clear identity,
  ask which story it updates. Do not run `archetipo spec update` during drafting.
- One invocation can propose several independently demonstrable stories when
  the agreed scope warrants it. Respect an explicit single-story request.

When a structured receipt is requested, close with one JSON line using this shape, with markdown newlines escaped as `\n`:

```json
{"artifact":"spec_draft","status":"PROPOSED","title":"Story title","epic_code":"","epic_title":"New epic name","priority":"MEDIUM","points":0,"scope":"MVP","blocked_by":[],"body":"Complete story markdown"}
```

For an existing epic, supply its `epic_code` and omit `epic_title` or leave it empty. Persist only when the person subsequently requests creation, using the existing CLI extension flow.

For several stories, emit one receipt containing the complete set:

```json
{"artifact":"spec_draft","status":"PROPOSED","proposals":[{"title":"First story","epic_code":"","epic_title":"New epic name","priority":"MEDIUM","points":0,"scope":"MVP","blocked_by":[],"body":"Complete story markdown"},{"title":"Second story","epic_code":"","epic_title":"New epic name","priority":"MEDIUM","points":0,"scope":"MVP","blocked_by":[],"body":"Complete story markdown"}]}
```

When a structured receipt is requested, keep the surrounding chat to a short
explanation of decisions and the split: the caller renders each story from the
receipt. A request for changes continues this conversation and produces a new
complete receipt replacing the proposed set, without persistence. Preserve
unchanged pending stories in that set. Stories already accepted into the
backlog are no longer pending; revise them only on explicit request. Standalone,
present readable markdown and use the same confirmation-before-persistence rule.

Example revision item (inside `proposals`, or in the single-story receipt):

```json
{"draft_id":"story-1","revises_spec":"US-001","title":"Revised title","epic_code":"EP-001","priority":"MEDIUM","points":0,"scope":"MVP","blocked_by":[],"body":"Complete revised story markdown"}
```

Standalone, after explicit approval of a revision, use
`archetipo spec update US-001 --file -` with a JSON partial patch containing
only the approved `title`, `priority`, `points`, `scope`, `blocked_by`, `body`
and, if changed, `epic: {"code":"EP-001","title":"Existing epic title"}`.
Expect `kind: write_result` and `data.ok: true` in the standard envelope;
handle `E_PRECONDITION` by rereading the backlog and resolving a missing target,
and follow the shared error rules for other codes. Preserve the story's status,
plan, worktree and implementation metadata. Verify the result with
`archetipo spec list`. Never use `spec add` to apply a revision.
