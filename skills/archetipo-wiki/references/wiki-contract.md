# DDD Wiki contract and CLI protocol

## Page format

Concept pages live below `paths.wiki`, excluding the reserved `index.md` and `log.md` files. Every concept is a UTF-8 Markdown file with YAML frontmatter. Its stable ID is its Wiki-relative path with the `.md` suffix removed: `domains/trips.md` has ID `domains/trips`. Every concept requires non-empty `type`, `title`, and `description` fields. Relationships use standard Markdown links: `/domains/trips.md` is bundle-relative, while `../domains/trips.md` is relative to the current page. Producer-defined frontmatter fields are allowed and must be preserved when a page is rewritten.

```markdown
---
type: domain
title: Trips
description: Trip lifecycle, stages, publication, and owned trip data
classification: candidate
status: generated
sources:
  - path: src/app/api/trips/[id]/publish/route.ts
    role: inbound-api
    symbol: PATCH
  - path: src/lib/trips/tripValidationService.ts
    role: application-domain
    symbol: TripValidationService.validateForPublication
issues:
  - code: UNREACHABLE_REVIEW_STATE
    summary: The schema declares a review state but no inspected write path reaches it
---
# Trips

<!-- archetipo:wiki section=purpose -->
...

## Related concepts

Trips participates in the [context map](/architecture/context-map.md).
```

Allowed persisted statuses are only:

- `generated`: created or materially changed, useful for routing but not explicitly reviewed;
- `reviewed`: explicitly approved against recorded content and evidence.

`stale` and `attention` are derived display states. `stale` means reviewed content or cited evidence changed. `attention` means the page contains unresolved `issues`. Generated pages must not carry `review`; reviewed pages require CLI-produced `review.content_hash`, `review.evidence_revision`, and `review.reviewed_at`.

An issue is an approval blocker, so reserve it for a concrete contradiction, unreachable modeled behavior, or evidence gap that makes the page unsafe to trust. Candidate classification, shared-runtime coupling, a child entity lacking an independent lifecycle, test-coverage observations, release uncertainty, tradeoffs, and descriptive uncertainties are not issues by themselves; record them in the relevant body section. Issues exist only in the frontmatter `issues` array; an `archetipo:wiki section=issues` body block is invalid and produces `WIKI_BODY_ISSUES`.

Delete replaced pages after updating links; Git is the history.

`index.md` has no frontmatter. `archetipo wiki catalog` groups concepts under headings and writes entries as `* [Title](relative/path.md) - description`. `log.md` has no frontmatter; it starts with `# Wiki Update Log` and groups `* **Review**:` or `* **Update**:` entries below ISO date headings such as `## 2026-07-16`, newest first. The CLI is the sole writer for both reserved files; agents must not synthesize or reformat them. Validation reports `WIKI_LOG_FORMAT` for a malformed log.

Optional project documents are normal `type: reference` concepts below `references/`. A reference requires `title`, `description`, `status: generated` until reviewed, and the original project-relative path in `sources` with `role: original`. Use `resource` only for a canonical URI. Preserve the source content in the body. Do not store frontmatter-free Markdown anywhere below `paths.wiki`.

## Architectural decisions

Architectural Decision Records are ordinary Wiki concepts with stable IDs under `decisions/` and `type: decision`. The Wiki lifecycle `status` remains `generated` or `reviewed`; `decision_status` records the decision lifecycle and is either `accepted` or `superseded`.

```yaml
type: decision
title: Shared rate-limit store
description: Use a shared Redis-backed rate-limit store with an in-memory local fallback
decision_status: accepted
status: generated
sources:
  - path: src/lib/rate-limiting/providers/RedisRateLimitStore.ts
    role: implementation
  - path: src/tests/unit/lib/rate-limiting.test.ts
    role: verification
```

Every decision page contains meaningful content under these markers:

```markdown
<!-- archetipo:wiki section=context -->
<!-- archetipo:wiki section=decision -->
<!-- archetipo:wiki section=alternatives -->
<!-- archetipo:wiki section=consequences -->
<!-- archetipo:wiki section=verification -->
```

The context states the forces and scope. The decision names the chosen option. Alternatives records at least one viable alternative and why it was not selected. Consequences includes positive and negative tradeoffs plus operational implications. Verification cites the implementation and tests/configuration that demonstrate adoption. Decision pages require repository evidence in `sources`; rationale comes from the planning decision, never from reverse-engineering implementation shape. A later choice that replaces an ADR sets the old page to `decision_status: superseded`, links it to the replacement with a standard Markdown link, and creates or updates the new accepted decision page instead of deleting history.

## Domain and bounded-context model

Domains and contexts share one page type: `type: domain`. `classification` is required:

- `candidate`: a capability cluster supported by evidence but not yet proven to be an autonomous bounded context;
- `bounded-context`: vocabulary, ownership, contracts, runtime behavior, and boundary are sufficiently evidenced.

Every domain page contains these language-neutral markers with meaningful content:

```markdown
<!-- archetipo:wiki section=purpose -->
<!-- archetipo:wiki section=language -->
<!-- archetipo:wiki section=ownership -->
<!-- archetipo:wiki section=contracts -->
<!-- archetipo:wiki section=flows -->
<!-- archetipo:wiki section=code -->
<!-- archetipo:wiki section=invariants -->
<!-- archetipo:wiki section=verification -->
```

The code section maps UI, inbound APIs, application/domain logic, owned data, integrations, configuration, and tests. Ownership means the data and business decisions controlled by the domain, not the people maintaining it. The flows section separates observed runtime transitions from declared-but-unobserved states. For state machines, an observed transition requires an exact write assignment, its source-state guard when present, and the cited source path. Derive `A -> B` from the code that assigns `B`, never from an enum member, endpoint name, comment, UI label, request-body field, delete operation, or expected workflow. If code guarded on `A` writes `C` while `B` exists only in the model, document `A -> C` and flag `B` as unreachable. The invariants section separates constraints enforced by executable code/schema/tests from assumptions or declared intent; a TypeScript type alone is not runtime enforcement. Bootstrap persists every domain as `candidate`; promotion to `bounded-context` is a separate semantic review decision. The bootstrap profile rejects premature promotion with `WIKI_BOOTSTRAP_BOUNDARY_UNREVIEWED`.

## Context map and code map

`architecture/context-map` is the logical DDD view. It contains:

Its page type is `context-map`; `engineering/code-map` uses `code-map`, `overview` uses `overview`, and `operations/development` uses `operations`.

Every bootstrap core page must participate in the concept graph through a standard Markdown link to an existing Wiki concept or an incoming link from one. The bootstrap profile rejects an isolated core page with `WIKI_BOOTSTRAP_CORE_ORPHAN`.

```markdown
<!-- archetipo:wiki section=contexts -->
<!-- archetipo:wiki section=relationships -->
<!-- archetipo:wiki section=shared -->
<!-- archetipo:wiki section=uncertainties -->
```

It describes domain responsibilities and relationships. Use specialized DDD relationship names only when evidence supports them.
Do not combine alternatives such as `Conformist/Shared Kernel`. Name one DDD relationship only when code shows the corresponding collaboration and governance semantics; otherwise describe the observed dependency in plain language and record the relationship type as unresolved.

Page bodies are plain Markdown. Model or tool protocol wrappers such as `<content>`, `</content>`, `<invoke>`, `</invoke>`, `<tool_use>`, and `<tool_result>` are invalid persisted content and produce `WIKI_PROTOCOL_ARTIFACT`.

`engineering/code-map` is the physical crosswalk from domains to code. It contains:

```markdown
<!-- archetipo:wiki section=domain-code -->
<!-- archetipo:wiki section=shared -->
<!-- archetipo:wiki section=unmapped -->
<!-- archetipo:wiki section=coverage -->
```

Its main table maps each domain to UI, entry points, application/domain code, owned data, integrations, tests, and its Wiki page. It does not repeat architecture prose.

## Deterministic coverage

`engineering/code-map` frontmatter represents every item returned by `wiki inspect`:

```yaml
coverage:
  - kind: boundary
    path: src
    status: mapped
    pages: [engineering/code-map]
  - kind: capability
    path: trip
    status: mapped
    pages: [domains/trips]
  - kind: capability
    path: ui
    status: partial
    note: Shared UI primitives are mapped physically but are not a business domain
```

Allowed kinds are `boundary` and `capability`. Allowed statuses are:

- `mapped`: requires one or more valid page IDs;
- `partial`: requires a reason;
- `excluded`: requires a reason.

## CLI operations

All commands receive no stdin payload and emit the standard `archetipo/v1` envelope.

All Wiki commands accept the persistent `--project-root <checkout>` flag. Spec implementation and review pass `data.workdir` explicitly so a nested Git worktree is inspected and mutated instead of the parent checkout.

- `archetipo wiki init` → `kind: wiki_init_result`, `data.root`, `data.created`.
- `archetipo wiki inspect` → `kind: wiki_inspection_result`; content-free deterministic inventory including `data.boundaries`, `data.capability_candidates`, evidence categories, exclusions, uninspected areas, and optional `data.project_sources`.
- `archetipo wiki status` → `kind: wiki_status`; derived state counts and page items plus findings.
- `archetipo wiki validate [--profile bootstrap]` → `kind: validation_result`, `data.ok`, `data.pages`, `data.findings`. Bootstrap validation also requires core DDD pages and full boundary/capability coverage.
- `archetipo wiki search [query] [--type TYPE] [--status STATE]` → `kind: wiki_search_result` without page bodies.
- `archetipo wiki affected [--base REV --head REV | --file PATH...]` → `kind: wiki_affected_result`.
- `archetipo wiki catalog` → `kind: wiki_catalog_result`, `data.cataloged`; rebuilds navigation without changing review state.
- `archetipo wiki reset <page-id...>` → `kind: wiki_reset_result`, `data.reset`; returns selected reviewed pages to generated and removes review metadata before semantic edits.
- `archetipo wiki approve [page-id...]` → `kind: wiki_approve_result`, `data.approved`; marks issue-free generated pages reviewed and records review metadata.

Relevant error codes:

- `E_PRECONDITION`: Wiki missing or repository evidence absent;
- `E_INVALID_INPUT`: malformed config, arguments, page IDs, or Git revisions;
- `E_CONFLICT`: approval blocked by validation errors or unresolved issues;
- `E_INTERNAL`: filesystem, Git, or encoding failure.
