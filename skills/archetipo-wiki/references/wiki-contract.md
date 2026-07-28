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

`evidence-changed`, `stale`, and `attention` are derived display states. `evidence-changed` means a successfully recomputed fingerprint shows that tracked evidence changed after review and the page needs reason-aware reconciliation; it does not by itself prove the page obsolete. `stale` means semantic content/review metadata changed or evidence freshness could not be recomputed safely. `attention` means the page contains unresolved `issues`. Generated pages must not carry `review`; reviewed pages require CLI-produced `review.content_hash`, `review.evidence_revision`, and `review.reviewed_at`. New approvals also record `review.evidence_hash` as `sha256:<64 lowercase hex>`. Existing reviewed pages without `evidence_hash` remain valid and transparently use `evidence_revision` for freshness until reset and reapproved.

Each source may set `freshness: tracked` or `freshness: context`; omission is exactly equivalent to `tracked`. Any other value is invalid (`WIKI_INVALID_SOURCE_FRESHNESS`). Tracked sources participate in `review.evidence_hash`, legacy `evidence_revision` fallback path matching, and `wiki affected`. Context sources remain visible and serialized provenance, remain part of semantic page content hashing, and still pass normal path/existence validation, but are excluded from freshness and affected discovery. Changing the `freshness` field itself changes semantic content and makes an existing review outdated; reset, correct, and approve that page once rather than silently reinterpreting old metadata.

`evidence_hash` fingerprints tracked project-relative repository sources exactly as they exist in the working tree at approval time. Only syntactically valid absolute `http` or `https` URLs with a non-empty hostname are external and excluded from local evidence, with case-insensitive scheme matching; a port-only authority is not external. Every other non-empty source—including malformed URLs, unknown schemes, Windows-looking `C://...` values, and traversal strings containing `://`—must pass project-relative validation.

Local sources share one fail-closed portable namespace on every host. `/` and `\` are accepted as input separators for command processing and normalized to `/` internally, but persisted source metadata is never rewritten; skill authors must store `/`. `.` and `./` remain valid whole-project evidence, as do legitimate Unicode names, interior spaces/dots, and literal non-alias `name~1` names. The CLI rejects NUL/control bytes, `:`, `<`, `>`, `"`, `|`, `?`, `*`, traversal, absolute/rooted/drive-relative/UNC/device/extended paths, trailing dot/space components, and case-insensitive DOS devices including extension forms. It never rewrites source metadata to repair an alias.

Exact stage-0 Git index spelling is authoritative for tracked evidence; exact stored directory-entry spelling is authoritative otherwise. Every existing component is attested without following it, so wrong-case spelling, Unicode-equivalent aliases, and available Windows 8.3 aliases fail rather than downgrade to untracked evidence. Portable collision identity is computed component-wise with deterministic Unicode NFC normalization plus Unicode case folding. A collision that intersects directly cited or directory evidence blocks that evidence on every OS; unrelated collisions do not globally poison the repository.

The configured project root is a trusted physical anchor. Intermediate symlinks, Windows junctions/name-surrogate reparses, and other redirections are rejected. Windows rejects ADS/device/trailing aliases before filesystem I/O, uses no-follow handles for canonical spelling/reparse/file identity, and supports long internal paths. A supported terminal symlink remains valid evidence represented only by its link-target text and is never followed; unsupported terminal reparse types fail closed. Linux rejects mount-ID crossings, including same-filesystem bind mounts. macOS rejects mounted-on or filesystem-identity crossings. Verified hard links remain allowed. Unsupported or unverifiable identity is unreadable. These checks do not claim to eliminate concurrent hostile filesystem replacement and are not a hostile-filesystem sandbox.

In a Git worktree, the stage-0 index is authoritative for tracked type and executable identity. Modes `100644` and `100755` pipe current working-tree bytes through `git hash-object --stdin --path=<project-relative-path>` and record the resulting Git-clean-normalized object identity with the executable marker from the index. The command does not write an object or stage content. Clean/EOL/filter-equivalent checkouts therefore share identity, while dirty working-tree content remains observable independently of the staged blob; filter failures block evidence inspection. Host permission bits and `core.fileMode` do not alter identity. Untracked and non-Git regular files retain raw-byte SHA-256 identity with a stable non-executable marker. Mode `120000` hashes the same link-target payload whether represented by an OS symlink or a `core.symlinks=false` materialized file. Unmerged index stages block evidence inspection. Directory fingerprints deterministically include tracked files plus non-ignored untracked files in Git repositories, or a sorted filesystem walk outside Git; deleted tracked entries, missing paths, and newly added evidence therefore change freshness. A nested ordinary repository, linked worktree, or bare/reftable/future-layout repository below the trusted project root is an unsupported evidence boundary: directly cited roots, descendants, and non-ignored roots reached during directory expansion fail closed. Repository-like markers, including a regular or symlink `HEAD`, trigger authoritative Git confirmation, and confirmation failure closes inspection. The configured project repository root itself is allowed, and an ignored nested repository is excluded from a parent-directory fingerprint unless directly cited.

Mode `160000` is clean-only submodule evidence. A source may cite the exact gitlink root or a parent directory containing it, but every strict descendant below a gitlink is rejected before entering the submodule filesystem; callers must cite the gitlink root instead. The exact submodule must be initialized, its checked-out `HEAD` must equal the stage-0 gitlink OID, and `git status --porcelain=v2 -z --untracked-files=all --ignore-submodules=none` must be empty. Ignored files and uninitialized nested submodule worktrees are allowed only when that Git status remains empty. Missing cited worktrees, HEAD mismatches, tracked/untracked non-ignored/conflicted changes, and nested submodule dirtiness block evidence inspection. The manifest records the gitlink path and matching index/HEAD OIDs; it never recursively hashes a dirty submodule.

Wiki concepts are fingerprinted semantically: CLI-owned lifecycle fields `status` and `review` do not contribute, so resetting or approving a cited concept does not stale its dependents, while any other frontmatter or body change does. Directory traversal excludes CLI-managed Wiki `index.md` and `log.md`. Approval fingerprints every selected page before writing any page.

A successfully recomputed hash/revision mismatch produces the non-blocking warning `WIKI_EVIDENCE_CHANGED` and derives the `evidence-changed` state. Inspection failures are never labeled as changes: unsafe traversal produces `WIKI_UNSAFE_SOURCE_PATH`, unreadable/unsupported/index/submodule evidence produces `WIKI_EVIDENCE_UNREADABLE`, and a failed legacy Git comparison produces `WIKI_EVIDENCE_RECOMPUTE_FAILED`; these findings have error severity, make `data.ok: false`, and derive `stale` conservatively. During spec acceptance, an affected-only non-reference page with only `WIKI_EVIDENCE_CHANGED` may be explicitly reconfirmed only after the dossier verifies its semantic claims against the exact diff. An evidence-changed reference whose tracked original changed must be refreshed rather than routinely reconfirmed.

`wiki affected` applies the same project-relative normalization to tracked sources and changed files, then compares path components. Every explicit `--file` must be a nonempty exact portable local path; invalid caller input returns `E_INVALID_INPUT` and never an empty success. Invalid persisted tracked sources fail discovery closed with `E_CONFLICT` rather than returning a partial affected set. Revision-based discovery consumes project-scoped, project-relative NUL-delimited Git names even when the configured project is nested inside a larger repository. Rename/copy detection is disabled, preserving spaces, tabs, newlines, and Unicode: a rename is intentionally observed as deletion plus addition so both old and new paths participate, while a copied destination appears as an addition and the unchanged source remains unchanged. A tracked source of `.` or `./` matches every valid project-relative changed file; `src` matches `src/a.go` but never `src2/a.go`. Context sources, valid external HTTP(S) URLs, absolute paths, and out-of-project paths never enter affected discovery.

An issue is an approval blocker, so reserve it for a concrete contradiction, unreachable modeled behavior, or evidence gap that makes the page unsafe to trust. Candidate classification, shared-runtime coupling, a child entity lacking an independent lifecycle, test-coverage observations, release uncertainty, tradeoffs, and descriptive uncertainties are not issues by themselves; record them in the relevant body section. Issues exist only in the frontmatter `issues` array; an `archetipo:wiki section=issues` body block is invalid and produces `WIKI_BODY_ISSUES`.

Delete replaced pages after updating links; Git is the history.

`index.md` has no frontmatter. `archetipo wiki catalog` groups concepts under headings and writes entries as `* [Title](relative/path.md) - description`. `log.md` has no frontmatter; it starts with `# Wiki Update Log` and groups `* **Review**:` or `* **Update**:` entries below ISO date headings such as `## 2026-07-16`, newest first. The CLI is the sole writer for both reserved files; agents must not synthesize or reformat them. Validation reports `WIKI_LOG_FORMAT` for a malformed log.

Optional project documents are normal `type: reference` concepts below `references/`. A reference requires `title`, `description`, `status: generated` until reviewed, and the original project-relative path in `sources` with `role: original`. The original artifact is tracked, explicitly or by omission. Implementation files used only to compare current behavior or navigate the codebase must be ordinary Markdown links or sources with `freshness: context`, so an unchanged reference becomes `evidence-changed` when its original changes rather than whenever a shared implementation hub changes. Use `resource` only for a canonical URI. Preserve the source content in the body. Do not store frontmatter-free Markdown anywhere below `paths.wiki`.

Migration is explicit: identify references that incorrectly track implementation pointers, reset each page once, remove those sources in favor of Markdown links or mark them context, validate, and approve the corrected page once. There is no automatic reinterpretation of existing sources and no recurring reconfirmation for this reference case.

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

All Wiki commands accept the persistent `--project-root <checkout>` flag. The nearest configuration belonging to that target checkout is authoritative. If the target has no configuration of its own, the invoking checkout configuration is inherited and runtime `ProjectRoot` is retargeted to the explicit checkout. Spec implementation and review pass `data.workdir` explicitly so a nested Git worktree is inspected and mutated instead of the parent checkout.

- `archetipo wiki init` → `kind: wiki_init_result`, `data.root`, `data.created`.
- `archetipo wiki inspect` → `kind: wiki_inspection_result`; content-free deterministic inventory including `data.boundaries`, `data.capability_candidates`, evidence categories, exclusions, uninspected areas, and optional `data.project_sources`.
- `archetipo wiki status` → `kind: wiki_status`; derived state counts and page items plus findings.
- `archetipo wiki validate [--profile bootstrap]` → `kind: validation_result`, `data.ok`, `data.pages`, `data.findings`. Bootstrap validation also requires core DDD pages and full boundary/capability coverage.
- `archetipo wiki search [query] [--type TYPE] [--status STATE]` → `kind: wiki_search_result` without page bodies.
- `archetipo wiki affected [--base REV --head REV | --file PATH...]` → `kind: wiki_affected_result`.
- `archetipo wiki catalog` → `kind: wiki_catalog_result`, `data.cataloged`; rebuilds navigation without changing review state.
- `archetipo wiki reset <page-id...>` → `kind: wiki_reset_result`, `data.reset`; returns selected reviewed pages to generated and removes review metadata before semantic edits.
- `archetipo wiki approve [page-id...]` → `kind: wiki_approve_result`, `data.approved`; marks issue-free generated pages reviewed and records review metadata.
- `archetipo wiki reconfirm <page-id...>` → `kind: wiki_reconfirm_result`, `data.reconfirmed`, `data.root`, `data.pages`; after explicit human verification, refreshes only evidence hash/revision and `reviewed_at` for unchanged, persisted-reviewed, issue-free pages with present evidence and a current semantic content hash. Selection is mandatory and batch preflight is atomic. A current hash is a no-op; a selected legacy no-hash review upgrades. It rebuilds the index and adds one Review log entry only when metadata changed. A spec acceptance review may invoke it for exact reconfirm-ready non-reference pages named in the approved dossier; never use it as routine maintenance for references.

Relevant error codes:

- `E_PRECONDITION`: Wiki missing or repository evidence absent;
- `E_INVALID_INPUT`: malformed config, arguments, page IDs, Git revisions, or explicit `wiki affected --file` paths;
- `E_CONFLICT`: affected discovery blocked by invalid persisted evidence, or approval/reconfirmation blocked by validation errors, lifecycle/content eligibility, unresolved issues, missing evidence, unsafe traversal, an unmerged index, an unsupported entry, or a non-clean submodule;
- `E_INTERNAL`: unexpected filesystem, Git, or encoding implementation failure.
