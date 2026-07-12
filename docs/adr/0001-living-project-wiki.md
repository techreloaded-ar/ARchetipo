# ADR 0001: Living project Wiki

- Status: accepted
- Date: 2026-07-12

## Context

ARchetipo currently treats the PRD, backlog and implementation plans as separate artifacts. Agents either load a large document or rediscover project knowledge from source code. That makes context expensive and allows product and architectural knowledge to drift after implementation.

## Decision

ARchetipo uses a local, Markdown, Git-versioned Wiki as the canonical project knowledge base. The default root is `docs/wiki/` and contains a compact `index.md`, an append-only `log.md`, and pages grouped under `vision`, `product`, `architecture`, `domains`, `decisions`, `engineering`, `operations`, `history`, and `sources`.

Every ordinary page has YAML frontmatter with a stable `id`, `type`, routing `summary`, lifecycle `status`, explicit `links`, provenance `sources`, `git_revision`, and `last_verified_at`. Allowed states are `draft`, `verified`, `needs-review`, and `superseded`.

The CLI owns deterministic filesystem operations: scaffolding, parsing, indexing, search, graph validation, evidence checks, affected-page discovery, atomic publication, and legacy migration. Skills own semantic interpretation and content generation. Connectors continue to own backlog and workflow only; Wiki commands never branch on connector type.

`index.md` is a routing catalog rather than a cumulative project document. Source code is cited rather than copied. Imported PRDs and Codemaps are retained below `sources/` but excluded from ordinary context search unless explicitly requested.

Development skills load the index first and only then select relevant pages. Plans declare `wiki_impact`. Implementation may prepare draft pages, while review validates and publishes them after acceptance.

## Consequences

- New projects compile discovery into the Wiki and archive the PRD as provenance.
- Existing projects bootstrap from repository boundaries and may migrate legacy PRD or Codemap artifacts.
- Agents can progressively load project knowledge without embeddings or a monolithic Codemap.
- Wiki changes become part of the same reviewable Git history as code changes.
- Structural correctness is machine validated; semantic quality remains the responsibility of skills and reviewers.
