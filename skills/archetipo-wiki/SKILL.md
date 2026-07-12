---
name: archetipo-wiki
description: Bootstrap, query, ingest, refresh, migrate, and lint ARchetipo's Markdown project Wiki. Use for an existing codebase without living documentation, importing project documents, answering project questions from canonical knowledge, repairing Wiki drift, or maintaining knowledge outside a single spec workflow.
---

# ARchetipo Wiki

Maintain `paths.wiki` as the canonical, progressively loaded project knowledge base. Keep code as evidence, not copied content. Keep archived documents under `sources/` out of ordinary queries.

## Start

1. Locate the project root and read `.archetipo/shared-runtime.md` exactly once.
2. Run `archetipo config show`; use `data.project_root` for CLI calls and resolve all repository reads from it.
3. Read [references/wiki-contract.md](references/wiki-contract.md) before creating or changing pages.
4. Select exactly one operation: `bootstrap`, `ingest`, `refresh`, `query`, or `lint`. Infer it from the request; default to `query` for a question and `bootstrap` when the Wiki is absent.

## Bootstrap

1. Run `archetipo wiki init`.
2. Run `archetipo wiki migrate` to archive an existing configured PRD and `docs/CODEMAP.md`; never treat migration as semantic ingestion.
3. Inventory repository boundaries using manifests, top-level directories, entry points, public contracts, configuration, and tests. For a large repository, sample representative files per component and explicitly record uninspected areas.
4. Create focused draft pages. Use one page per domain, component, decision, or operational concern; do not create a monolithic Codemap. Derive every page path from its stable ID using the canonical mapping in the Wiki contract.
5. Cite repository-relative source paths in frontmatter.
6. Run `archetipo wiki validate`. Repair every error finding; review warnings and record genuine coverage gaps as `needs-review` pages.
7. Run `archetipo wiki publish` only after the generated content is internally consistent.

## Ingest

1. Preserve the original document below `docs/wiki/sources/<category>/`.
2. Read the existing index, then run `archetipo wiki search` for overlapping concepts.
3. Update existing pages when they represent the same stable concept; create draft pages only for new concepts.
4. Link every derived claim to the archived document or repository evidence.
5. Validate and publish. If the new source conflicts with verified knowledge, preserve both claims, mark the page `needs-review`, and report the conflict instead of choosing silently.

## Refresh

1. Run `archetipo wiki affected --base <revision> --head <revision>` or pass repeated `--file` flags.
2. Inspect each returned page against the changed code. Search for related pages not directly evidenced by the changed files.
3. Update only claims made obsolete by the changes. Refresh evidence and verification metadata through `wiki publish`.
4. Validate, then publish draft updates. Report changed pages and unresolved gaps.

## Query

1. Read `docs/wiki/index.md` first.
2. Run `archetipo wiki search "<compact query>"`; refine with `--type` or `--status` when useful.
3. Read only the selected page paths. Follow explicit links only when the answer requires them.
4. Verify time-sensitive or implementation-specific claims against cited code before answering.
5. State when the Wiki does not cover the question; do not silently scan the entire repository as if it were documented knowledge.

## Lint

1. Run `archetipo wiki validate` and classify its deterministic findings.
2. Inspect summaries, duplicated concepts, contradictory claims, missing decision rationale, and pages whose evidence no longer supports their body.
3. Apply safe structural repairs. For semantic uncertainty, mark `needs-review` and explain the required human decision.
4. Re-run validation. Publish only if requested or if lint is part of an already-authorized maintenance workflow.

## Safety

- Never branch on connector type; Wiki storage is local for every connector.
- Branch on `error.code`, never `error.message`.
- Do not publish invalid drafts or convert uncertain claims to `verified`.
- Do not delete legacy source documents during migration.
- Do not load all source files or all Wiki pages when index and search can bound the context.
