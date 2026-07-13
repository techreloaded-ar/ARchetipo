---
name: archetipo-wiki
description: Bootstrap, query, ingest, refresh, and lint ARchetipo's Markdown project Wiki. Use for an existing codebase without living documentation, importing project documents, answering project questions from canonical knowledge, repairing Wiki drift, or maintaining knowledge outside a single spec workflow.
---

# ARchetipo Wiki

Maintain `paths.wiki` as the canonical, progressively loaded project knowledge base. Treat implemented code as evidence of current behavior and project documents as optional evidence of intent. Keep code as evidence, not copied content, and archived documents under `sources/` out of ordinary queries.

## Start

1. Locate the project root and read `.archetipo/shared-runtime.md` exactly once.
2. Run `archetipo config show`; use `data.project_root` for CLI calls and resolve all repository reads from it.
3. Read [references/wiki-contract.md](references/wiki-contract.md) before creating or changing pages.
4. Select exactly one operation: `bootstrap`, `ingest`, `refresh`, `query`, or `lint`. Infer it from the request; default to `query` for a question and `bootstrap` when the Wiki is absent.

## Bootstrap

1. Run `archetipo wiki init`.
2. Run `archetipo wiki inspect` before opening a PRD or broad documentation. Stop on `E_PRECONDITION`; do not invent a Wiki for an evidence-free repository.
3. Read every reported manifest, entry point, schema, configuration file, and public contract. For each `data.boundaries` item, read its representative implementation and test files. When `data.uninspected` is non-empty, preserve those limitations in the code map.
4. Create these draft core pages with repository evidence: `overview`, `architecture`, `engineering.code-map`, and `operations.development`. In `engineering.code-map`, record every inspected boundary in `coverage` using the contract below; never omit a boundary silently.
5. Add focused pages only when evidence supports them: product or vision from product evidence; domains from functional capabilities; components from autonomous technical boundaries; decisions from explicit rationale or trade-offs; engineering, operations, and history from corresponding repository evidence. Do not create pages merely to fill scaffold categories.
6. If `data.prd.present` is true, read the configured PRD only now and preserve it verbatim at `<paths.wiki>/sources/prd.md`. Use it as evidence of intent, never as proof of implemented behavior. Mark material code-versus-PRD conflicts `needs-review`. Do not create a placeholder when absent.
7. Cite repository-relative evidence in frontmatter. Link archived sources with ordinary Markdown links; reserve `[[page.id]]` for ordinary Wiki pages.
8. Run `archetipo wiki validate --profile bootstrap`. Repair every error and record real gaps as `mapped-only`, `needs-review`, or `excluded` coverage with a reason.
9. Run `archetipo wiki catalog`. Leave generated pages as `draft`; call `wiki publish` only after explicit human approval in a later review.

## Ingest

1. Preserve the original document below `docs/wiki/sources/<category>/`.
2. Read the existing index, then run `archetipo wiki search` for overlapping concepts.
3. Update existing pages when they represent the same stable concept; create draft pages only for new concepts.
4. Link every derived claim to the archived document or repository evidence.
5. Reset materially changed verified pages to `draft`, validate, and run `wiki catalog`. Publish only after explicit human approval. If the new source conflicts with verified knowledge, preserve both claims, mark the page `needs-review`, and report the conflict instead of choosing silently.

## Refresh

1. Run `archetipo wiki affected --base <revision> --head <revision>` or pass repeated `--file` flags.
2. Inspect each returned page against the changed code. Search for related pages not directly evidenced by the changed files.
3. Update only claims made obsolete by the changes and reset materially changed verified pages to `draft`.
4. Validate, then run `wiki catalog`. Report changed pages and unresolved gaps; publish only after explicit human approval.

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
- Do not read secret contents surfaced by repository exploration; `wiki inspect` intentionally omits them.
- Do not delete or modify source documents while archiving or ingesting them.
- Do not load all source files or all Wiki pages when index and search can bound the context.
