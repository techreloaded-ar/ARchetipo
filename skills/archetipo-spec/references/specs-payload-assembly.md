# Specs payload assembly

The `archetipo spec add` payload is **staged as part files and assembled by a script**, never produced as one large model response.

## Why

A backlog bootstrap can generate 15+ specs, each with a full markdown body; a single model response holding the whole payload risks the same failure measured on `archetipo spec plan`: a response that streams for minutes and can be cut off mid-stream, losing the entire phase. One part file per spec keeps every single response small regardless of how many specs are generated, because **a tool call ends a response**.

Unlike planning, `archetipo spec add` is append-only and idempotent: it creates the backlog when missing, appends otherwise, and specs whose `code` already exists are skipped and reported in `data.skipped`. There is nothing to carry over — only a build step is needed. If a worker dies mid-phase, rerunning `build` and `spec add` on the same staging directory is safe: already-persisted specs are simply skipped.

Assembly also removes hand-escaped JSON: part files are plain markdown, and `JSON.stringify` produces the payload.

## Staging layout

Every temporary artifact this skill writes lives under `.archetipo/tmp/` in `data.project_root` — one staging directory per batch, plus the assembled payload beside it. Nothing temporary is ever written outside that root, so a leftover is always visible in one place:

```text
.archetipo/tmp/specs-{first-code}-{last-code}/
  spec-01.md          # one file per spec the worker writes
  spec-02.md
```

Ordering: `spec-*` parts are sorted by the **first number in the filename**, so `spec-9.md` precedes `spec-10.md` without zero padding. A spec's `blocked_by` may only reference codes defined earlier in the assembled list (same-epic dependencies, per the flow's own rules — the assembler only checks that the referenced code exists).

## Spec part file format

A `---` delimited header of `key: value` lines, then the markdown body. Only the **first** closing `---` delimits the header, so horizontal rules inside the body are safe.

```markdown
---
code: US-001
title: User can export a monthly report as CSV
epic_code: EP-001
epic_title: Reporting
priority: HIGH
points: 3
status: TODO
scope: MVP
blocked_by:
---
As a finance lead
I want to export my monthly report as CSV
so that I can share it outside the tool

## Acceptance Criteria
...

## Demonstrates
...
```

`code`, `title`, `epic_code`, `priority`, `points`, and `scope` are required. `epic_title` is **required only for a newly proposed epic**: omit it for an epic that already exists, so the CLI resolves the real title from the backlog instead of receiving a guess. `status` defaults to `TODO`. `blocked_by` is a comma-separated list of spec codes, omitted or left empty when there are none. The body must follow the Spec Template from `SKILL.md`.

## Commands

Run from `data.project_root`. `{SKILL_DIR}` is this skill's own base directory — the parent of the `references/` directory you resolved to read this file, so you already know it. When the assembler is not there, the skill installation is incomplete: stop and report it rather than hand-writing the payload.

```bash
# 1. Assemble the payload from the staged parts.
node {SKILL_DIR}/references/assemble-specs-payload.mjs build .archetipo/tmp/specs-{first-code}-{last-code} .archetipo/tmp/payload-{first-code}-{last-code}.json

# 2. Validate, then persist.
archetipo validate spec --file .archetipo/tmp/payload-{first-code}-{last-code}.json
archetipo spec add --file .archetipo/tmp/payload-{first-code}-{last-code}.json

# 3. Clean up both the staging directory and the payload (cross-platform).
node {SKILL_DIR}/references/assemble-specs-payload.mjs clean .archetipo/tmp/specs-{first-code}-{last-code} .archetipo/tmp/payload-{first-code}-{last-code}.json
```

`build` fails on a duplicate spec code, a `blocked_by` reference to an unknown code, a malformed header, a non-numeric `points`, or an empty body. It reports the spec count — check it before validating.

Semantic validation stays with `archetipo validate spec`: the assembler does not judge spec content, INVEST compliance, or cross-epic dependency rules.

If validation reports `severity: "error"` findings, fix the offending part file(s) and rerun `build` — do not hand-edit the assembled JSON.

## Recovering an interrupted attempt

The staging directory survives a worker that died mid-phase. Because `archetipo spec add` is idempotent, a later attempt can safely reuse the staged parts and rerun `build` + `spec add`: specs already persisted are skipped and reported in `data.skipped`, and only the missing ones are written. When in doubt, `clean` and stage again — correctness outranks the saved work.

## No safety net here: `clean` is the only cleanup

The CLI sweeps spec-scoped temporaries when a spec reaches `{config.workflow.statuses.done}`, but a batch staging directory is **not** spec-scoped: `specs-{range}/` and its payload belong to a bootstrap or an extension, which happens before any spec is implemented, so nothing ever sweeps them for you. Running `clean` once `archetipo spec add` has answered is the only thing that removes them — treat skipping it as leaving garbage behind, not as a missed optimisation.
