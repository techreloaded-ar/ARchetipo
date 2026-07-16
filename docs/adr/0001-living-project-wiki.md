# ADR 0001: Living project Wiki

- Status: accepted
- Date: 2026-07-12

## Context

ARchetipo currently treats the PRD, backlog and implementation plans as separate artifacts. Agents either load a large document or rediscover project knowledge from source code. That makes context expensive and allows product and architectural knowledge to drift after implementation.

## Decision

ARchetipo uses a local, Markdown, Git-versioned Wiki as the canonical project knowledge base. Bootstrap is codebase-first and organizes current-system knowledge around DDD domains and candidate bounded contexts. `architecture/context-map` describes logical relationships; `engineering/code-map` maps those domains to physical code, data, contracts, and tests. A concept's stable identity is its Wiki-relative path without `.md`; relationships use standard Markdown links.

Every concept has YAML frontmatter with `type`, `title`, `description`, lifecycle metadata, and evidence where applicable. Persisted review state is only `generated` or `reviewed`. The CLI derives `stale` from identity/content/evidence changes and `attention` from explicit issues.

The CLI owns deterministic repository inspection, capability clustering, parsing, indexing, search, graph/DDD validation, evidence freshness, affected-page discovery, and review metadata. Skills own semantic DDD interpretation, reference ingestion, and content generation. Connectors continue to own backlog and workflow only; Wiki commands never branch on connector type.

`index.md` is a routing catalog rather than a cumulative project document. Source code is cited rather than copied. Optional project documents become `references/` concepts only after code mapping and remain evidence of intent rather than implementation.

Development skills load the index first and only then select relevant pages. Plans declare `wiki_impact`. Implementation resets changed pages to `generated`; review approves selected issue-free pages after acceptance.

Material architectural choices are represented as first-class `type: decision` pages under stable `decisions/` IDs. Planning creates the decision contract only when a choice has viable alternatives and meaningful cross-cutting consequences; implementation attaches repository evidence; acceptance reviews the rationale, alternatives, consequences, and adoption together with the code. Code-only bootstrap does not invent historical rationale.

## Consequences

- Projects compile discovered domain language, ownership, contracts, flows, and code locations into progressively loaded pages.
- Deterministic capability candidates must be mapped, partially mapped, or explicitly excluded.
- Agents can progressively load project knowledge without embeddings or a monolithic project document.
- Wiki changes become part of the same reviewable Git history as code changes.
- ADRs grow with qualifying development decisions without turning routine implementation details into decision noise.
- Structural correctness is machine validated; semantic quality remains the responsibility of skills and reviewers.
