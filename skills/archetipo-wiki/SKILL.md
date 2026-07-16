---
name: archetipo-wiki
description: Bootstrap, query, ingest, refresh, review, and lint ARchetipo's codebase-first DDD Wiki. Use for mapping an existing repository into domains and candidate bounded contexts, locating domain code, maintaining living project knowledge, or answering project questions with bounded context.
---

# ARchetipo Wiki

Maintain `paths.wiki` as a progressively loaded, codebase-first map. Implemented code is evidence of current behavior. Optional project documents are evidence of intent. Never turn a folder name, schema enum, or repeated word into a domain fact without checking executable flows.

## Start

1. Locate the project root and read `.archetipo/shared-runtime.md` exactly once.
2. Run `archetipo config show`; resolve repository reads from `data.project_root`.
3. Read [references/wiki-contract.md](references/wiki-contract.md) before creating or changing pages.
4. Select one operation: `bootstrap`, `ingest`, `refresh`, `query`, `review`, or `lint`. Default to `query` for a question and `bootstrap` when the Wiki is absent.

## Bootstrap

1. Run `archetipo wiki init`, then `archetipo wiki inspect`. Stop on `E_PRECONDITION`.
2. Read every reported manifest, entry point, schema, public contract, configuration file, and representative boundary file. For every `data.capability_candidates` item, read its reported entry points, UI, application/domain files, contracts, and tests. Record reported exclusions and sampling limits.
3. Build an internal evidence matrix before writing pages: candidate, currently observed purpose, actors, ubiquitous terms, commands/use cases, owned data, inbound/outbound contracts, dependencies, code paths, tests, observed flows, enforced invariants, failure modes, and confidence. “Ownership” means domain ownership of data and decisions, not team ownership.
4. Merge or split deterministic candidates by semantic evidence. A folder cluster is only a candidate. Merge a child entity or projection into its parent capability when it has no independent lifecycle, owned decisions, or contracts; map shared storage, mapping, validation, and UI clusters as infrastructure unless they participate in one coherent capability with domain behavior. Prefer the smallest set of coherent capabilities that explains all inspected candidates. Create one `domains/<id>.md` page per resulting capability and always use `classification: candidate` during bootstrap. The concept ID is its Wiki-relative path without `.md`. Promotion to `bounded-context` requires a later explicit semantic review; bootstrap evidence alone never promotes it. Candidate is a valid reviewable classification, not by itself an issue. Never create test, placeholder, or scratch pages below `paths.wiki`.
5. Create these generated core pages:
   - `overview`: system purpose, actors, stack, mapping scope, and exclusions;
   - `architecture/context-map`: domain relationships, shared infrastructure, upstream/downstream dependencies, and unresolved boundaries;
   - `engineering/code-map`: physical domain-to-code matrix plus shared and unmapped code;
   - `operations/development`: build, test, runtime, deployment, and operational constraints.
   Connect every core page to the concept graph with at least one standard Markdown relationship to an existing Wiki concept, or an incoming relationship from one. No core page may remain isolated.
   Do not create `type: decision` pages by inferring rationale from code during a code-only bootstrap. ADRs enter the Wiki from an explicit planning choice or from a reference concept whose rationale can be attributed.
6. In `engineering/code-map`, represent every inspected physical boundary and every capability candidate in `coverage`. Map it to domain pages, mark it `partial`, or exclude it with a reason. Never omit a candidate silently.
7. Separate observed runtime behavior from declared-but-unobserved models. For every state machine, enumerate assignments/writes to each state and derive transitions from the source-state guard plus the exact assigned target; endpoint names, comments, UI labels, and enums are not transition evidence. Cite the write path beside every claimed observed transition. Inspect code and tests for permissions, side effects, invariants, and integrations too. A type declaration or dependency is not an enforced invariant or implemented capability. Use `issues` only for actionable contradictions or missing evidence that blocks trusting the page; candidate classification, monolith boundaries, ordinary tradeoffs, and observations belong in the page body or context-map uncertainties. Do not encode uncertainty in page status.
8. Only after the codebase map exists, represent each optional `data.project_sources` item as a generated `type: reference` concept below `<paths.wiki>/references/`, using the lowercase source basename without its extension as the concept filename. Give it `title` and `description`, cite the exact original project-relative path in `sources` with `role: original`, set `status: generated`, and preserve the source content in the concept body. Use `resource` only when the underlying asset has a canonical URI. Reconcile references as intent, not implementation evidence. Attribute every document-only statement to that document and re-check it against executable code before describing current behavior; never turn a PRD claim into a code observation.
9. Keep model and tool protocol syntax out of persisted Markdown. Never copy wrapper tags such as `<content>`, `</content>`, `<invoke>`, `</invoke>`, `<tool_use>`, or `<tool_result>` into a page.
10. Before validation, perform an adversarial state-machine audit. For every claimed transition, re-open the cited write path and verify the exact target assignment plus the source-state guard. A request-body field, enum member, endpoint name, delete operation, UI label, or intended workflow is not a transition. If any modeled state has no assignment, remove the false transition and add an issue when the gap blocks trust.
11. Audit every issue. Keep it only when it identifies a concrete contradiction or missing evidence that makes the page unsafe to trust. Move ordinary tradeoffs, missing independence, candidate classification, test-coverage observations, release uncertainty, and non-blocking limitations into the body or context-map uncertainties. Re-open every source needed by an issue; do not base an issue on a project document when executable code resolves the claim.
12. Set every created page to `status: generated` with no `review` block. Run the exact command `archetipo wiki validate --profile bootstrap` and inspect its JSON envelope. A plain `archetipo wiki validate` does not satisfy the bootstrap gate. Repair every error (including `WIKI_PROTOCOL_ARTIFACT`, `WIKI_BOOTSTRAP_BOUNDARY_UNREVIEWED`, and `WIKI_BOOTSTRAP_CORE_ORPHAN`), run `archetipo wiki catalog`, then run `archetipo wiki validate --profile bootstrap` again. Report bootstrap success only when the final envelope has `kind: validation_result` and `data.ok: true`; otherwise continue repairing or report the bootstrap as failed.

## Ingest

1. Create or update its `type: reference` concept below `paths.wiki/references/`, with the original path in `sources` using `role: original` and the source content in the body. Set `resource` only for a canonical URI.
2. Read the index and search for overlapping concepts.
3. Update the existing stable page when it represents the same concept. Create a page only for a new concept.
4. For every materially changed reviewed page, run `archetipo wiki reset <page-id>...` before editing. Generated pages already need no transition.
5. Preserve disagreements as explicit `issues`, validate, and catalog.
6. When the ingested source is an explicit architecture decision, create or update its canonical `decisions/<slug>` page using the decision contract; attribute rationale to the source and verify current adoption against repository evidence.

## Refresh

1. Run `archetipo wiki affected --base <revision> --head <revision>` or pass repeated `--file` flags.
2. Inspect affected pages and related domains against changed code and tests.
3. Run `archetipo wiki reset <page-id>...` for reviewed pages that require changes, then update obsolete claims only and retain unresolved issues.
4. Validate and catalog. Approval is a separate operation.

## Query

1. Read `docs/wiki/index.md` first.
2. Search with a compact query and optional type/state filters.
3. Read only selected pages and explicit links.
4. Treat `generated`, `stale`, and `attention` pages as routing knowledge that requires code verification. Verify implementation-specific claims against cited symbols or paths.
5. State uncovered or contradictory areas instead of silently treating inference as fact.

## Review

1. Run `archetipo wiki status` and `archetipo wiki validate`.
2. Review selected generated pages against their cited code, tests, domain ownership, contracts, flows, invariants, and issues.
3. Resolve every issue on a page before approval. Structural validation alone never authorizes review.
4. Only after explicit user approval, run `archetipo wiki approve <page-id>...`. With no IDs the command approves every issue-free generated page.
5. `reviewed` records a content hash, evidence revision, and timestamp. `stale` and `attention` are derived by the CLI and never written as lifecycle states.

## Lint

1. Run `archetipo wiki validate` and classify deterministic findings.
2. Inspect duplicated domains, unjustified bounded-context claims, missing ownership, contradictory flows, orphan capability candidates, and evidence that no longer supports a page.
3. Apply structural repairs. For semantic uncertainty, add an issue and reset the page to `generated`.
4. Re-run validation and catalog. Do not approve without an explicit review request.

## Safety

- Never branch on connector type; Wiki storage is local for every connector.
- Branch on `error.code`, never `error.message`.
- Do not read secret contents surfaced by inspection.
- Do not load every Wiki page when the catalog and search can bound context.
- Do not infer architectural rationale from implementation shape alone.
- Do not treat a data model state as reachable until a write path proves it.
- Do not approve pages with unresolved issues.
