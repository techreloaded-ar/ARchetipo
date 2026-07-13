# ADR 0001: Living project Wiki

- Status: accepted
- Date: 2026-07-12

## Context

ARchetipo currently treats the PRD, backlog and implementation plans as separate artifacts. Agents either load a large document or rediscover project knowledge from source code. That makes context expensive and allows product and architectural knowledge to drift after implementation.

## Decision

ARchetipo uses a local, Markdown, Git-versioned Wiki as the canonical project knowledge base. Bootstrap is codebase-first and organizes current-system knowledge around DDD domains and candidate bounded contexts. `architecture.context-map` describes logical relationships; `engineering.code-map` maps those domains to physical code, data, contracts, and tests.

Every ordinary page has YAML frontmatter with a stable `id`, `type`, routing `summary`, explicit links and evidence. Persisted review state is only `generated` or `reviewed`. The CLI derives `stale` from content/evidence changes and `attention` from explicit issues.

The CLI owns deterministic repository inspection, capability clustering, parsing, indexing, search, graph/DDD validation, evidence freshness, affected-page discovery, and review metadata. Skills own semantic DDD interpretation, source archiving, and content generation. Connectors continue to own backlog and workflow only; Wiki commands never branch on connector type.

`index.md` is a routing catalog rather than a cumulative project document. Source code is cited rather than copied. Optional project documents are archived only after code mapping and remain evidence of intent rather than implementation.

Development skills load the index first and only then select relevant pages. Plans declare `wiki_impact`. Implementation resets changed pages to `generated`; review approves selected issue-free pages after acceptance.

## Consequences

- Projects compile discovered domain language, ownership, contracts, flows, and code locations into progressively loaded pages.
- Deterministic capability candidates must be mapped, partially mapped, or explicitly excluded.
- Agents can progressively load project knowledge without embeddings or a monolithic project document.
- Wiki changes become part of the same reviewable Git history as code changes.
- Structural correctness is machine validated; semantic quality remains the responsibility of skills and reviewers.
