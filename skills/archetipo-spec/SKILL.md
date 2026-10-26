---
name: archetipo-spec
description: Draft, create or refine enterprise backlog stories with explicit product outcomes and acceptance criteria.
---

# Backlog and stories

Read `.archetipo/shared-runtime.md` exactly once. Use the person's language.
Storing product intent in the enterprise backlog is useful; a backlog is not a
mandatory internal checklist for every app change. Use the shared runtime's
personalities and intent discovery, including inception-style brainstorming for
open ideas before drafting acceptance criteria. Explore alternative directions
with Andrea, Costanza and Livia, using the shared techniques and clickable choices
when available. Do not require a PRD, fixed discovery rounds or separate
implementation/review skills.

Read `archetipo config show` (`kind: setup`) and `archetipo spec list`
(`kind: backlog`, `data.items`, `data.summary`). Use existing codes, epics and
original requirements; inspect relevant product documents and actual behavior.
Explore intent one question at a time, including assumptions and alternatives.
Preserve decisions, uncertainties and dependencies.
Do not regenerate a backlog when asked to add or refine one story.

Write a compact story around people, need and intended outcome. Include:
- User story or a concrete technical/operational problem.
- Demonstrates: starting state, action and observable result.
- Acceptance criteria with stable IDs, each covering one observable outcome.
- Out of scope and material decisions/open questions.

A story must not prescribe a technical implementation unless it is a real
constraint. Keep claims about implemented behavior separate from desired behavior.

## Draft for discussion

When asked for a proposal, load `references/spec-proposal-flow.md`. Persist
nothing until creation or revision is requested. The same draft can be edited
in chat or accepted from the viewer. A draft receipt carries content, not proof
of persistence, and must retain draft/story identities across revisions.

## Persist an authorized story

`archetipo spec add --file -` accepts JSON/YAML:

```json
{"specs":[{"code":"US-011","title":"Concrete outcome","epic":{"code":"EP-001","title":"Existing or explicitly agreed new epic"},"priority":"MEDIUM","points":0,"status":"TODO","scope":"MVP","blocked_by":[],"body":"Markdown story"}]}
```

Use distinct available codes from the actual backlog and agreed metadata. Points
are optional estimates, never an implementation gate. Validate via
`archetipo validate spec --file -` with the same payload (`kind: validation_result`,
`data.ok`, `data.findings`) before `spec add` (`kind: write_result`, `data.created`,
`data.skipped`). Existing codes are skipped; do not mistake a skip for an update.
For a large batch use `references/specs-payload-assembly.md` to preserve partial work.

For an authorized revision, read `archetipo spec show <code>` (`kind: spec`)
then `archetipo spec update <code> --file -` with only changed fields, for example
`{"body":"revised Markdown","title":"revised title"}` (`kind: write_result`).
Preserve the original story identity, unrelated scope and status history.
A product work links original codes through the increment CLI; never clone them
into a separate delivery backlog or mark them done on technical completion.

All calls use `archetipo/v1`. Branch on `error.code`: repair `E_INVALID_INPUT`,
resolve `E_PRECONDITION`, and reread on `E_CONFLICT`. Only the CLI persists artifacts.
For a requested backlog generation, verify actual persisted items, then emit
`{"artifact":"backlog","status":"WRITTEN","epics":1,"specs":1}` with actual counts.
For proposal-only work, use the draft receipt defined by the proposal reference.
