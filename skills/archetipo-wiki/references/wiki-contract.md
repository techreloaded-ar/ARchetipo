# Wiki contract and CLI protocol

## Page format

Ordinary pages live below `paths.wiki`, excluding `index.md`, `log.md`, and archived files below `sources/`.

```markdown
---
id: architecture.authentication
type: architecture
summary: Authentication boundaries, token lifecycle, and trust relationships
status: draft
links:
  - id: domains.identity
    relation: implements
sources:
  - path: cli/internal/auth/service.go
    revision: optional-git-sha
git_revision: optional-git-sha
last_verified_at: optional-RFC3339
---
# Authentication
```

Allowed statuses are `draft`, `verified`, `needs-review`, and `superseded`. IDs remain stable across file moves. Summaries are compact routing descriptions. Use repository-relative paths for local evidence.

## CLI operations

All commands receive no stdin payload and emit the standard `archetipo/v1` envelope.

- `archetipo wiki init` → `kind: wiki_init_result`, `data.root`, `data.created`. Idempotently creates the scaffold.
- `archetipo wiki status` → `kind: wiki_status`, page/status counts and validation findings.
- `archetipo wiki validate` → `kind: validation_result`, `data.ok`, `data.pages`, `data.findings`. A structurally invalid Wiki is a successful command with `data.ok: false`.
- `archetipo wiki search [query] [--type TYPE] [--status STATUS] [--include-sources]` → `kind: wiki_search_result`, compact `data.items` without page bodies.
- `archetipo wiki affected [--base REV --head REV | --file PATH...]` → `kind: wiki_affected_result`, changed files and matching evidence-backed pages.
- `archetipo wiki publish` → `kind: wiki_publish_result`, `data.published`. It promotes valid drafts, rebuilds the index, records Git revision and verification time, and appends the log.
- `archetipo wiki migrate [--prd PATH] [--codemap PATH]` → `kind: wiki_migration_result`, archived paths and `requires_semantic_ingest`. It preserves sources but does not interpret them.

Relevant error codes:

- `E_PRECONDITION`: Wiki missing; initialize it where appropriate.
- `E_INVALID_INPUT`: malformed config, arguments, or Git revisions; repair the input.
- `E_CONFLICT`: publication blocked by validation; inspect `wiki validate` findings.
- `E_INTERNAL`: filesystem or encoding failure; stop without inventing success.
