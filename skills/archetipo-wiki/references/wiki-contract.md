# Wiki contract and CLI protocol

## Page format

Ordinary pages live below `paths.wiki`, excluding `index.md`, `log.md`, and archived files below `sources/`. Their repository-relative path is derived from the stable page ID: replace every `.` with `/` and append `.md`. For example, `architecture.authentication` must live at `architecture/authentication.md`; an ID without dots such as `overview` lives at `overview.md`. Do not flatten a dotted ID into a hyphenated root filename.

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
coverage:
  - path: cli
    status: documented
    pages: [architecture, engineering.code-map]
git_revision: optional-git-sha
last_verified_at: optional-RFC3339
---
# Authentication
```

Allowed statuses are `draft`, `verified`, `needs-review`, and `superseded`. IDs remain stable across content changes. Summaries are compact routing descriptions. Use repository-relative paths for local evidence. When the body needs a Wiki-style reference, target the stable page ID (for example, `[[domains.identity]]`) rather than its filesystem path.

`archetipo wiki validate` reports `WIKI_NONCANONICAL_PATH` as an error when a page path does not match its ID-derived path. Move the page to the reported canonical path before publishing.

`engineering.code-map` additionally carries one `coverage` entry for every boundary returned by `wiki inspect`. Allowed coverage statuses are `documented`, `mapped-only`, `needs-review`, and `excluded`. `documented` requires at least one valid page ID. The other statuses require a concise `note`; use them to make sampling, uncertainty, and intentional exclusions visible.

Body references in `[[...]]` must target an ordinary stable page ID. Archived files below `sources/` are provenance rather than Wiki pages and use standard Markdown links.

## CLI operations

All commands receive no stdin payload and emit the standard `archetipo/v1` envelope.

- `archetipo wiki init` → `kind: wiki_init_result`, `data.root`, `data.created`. Idempotently creates the scaffold.
- `archetipo wiki inspect` → `kind: wiki_inspection_result`; compact codebase inventory with `data.boundaries`, evidence categories, optional `data.prd`, exclusions, and uninspected areas. It returns no source contents.
- `archetipo wiki status` → `kind: wiki_status`, page/status counts and validation findings.
- `archetipo wiki validate [--profile bootstrap]` → `kind: validation_result`, `data.ok`, `data.pages`, `data.findings`. The bootstrap profile also checks core pages and inspection coverage. An invalid Wiki is a successful command with `data.ok: false`.
- `archetipo wiki search [query] [--type TYPE] [--status STATUS] [--include-sources]` → `kind: wiki_search_result`, compact `data.items` without page bodies.
- `archetipo wiki affected [--base REV --head REV | --file PATH...]` → `kind: wiki_affected_result`, changed files and matching evidence-backed pages.
- `archetipo wiki catalog` → `kind: wiki_catalog_result`, `data.cataloged`. Rebuilds index and log without changing page status.
- `archetipo wiki publish` → `kind: wiki_publish_result`, `data.published`. It promotes valid drafts, rebuilds the index, records Git revision and verification time, and appends the log.

Use `catalog` after generated or refreshed content. Use `publish` only after explicit human approval; structural validation alone never authorizes promotion to `verified`.
Relevant error codes:

- `E_PRECONDITION`: Wiki missing or repository evidence absent; initialize where appropriate, but never fabricate bootstrap content.
- `E_INVALID_INPUT`: malformed config, arguments, or Git revisions; repair the input.
- `E_CONFLICT`: publication blocked by validation; inspect `wiki validate` findings.
- `E_INTERNAL`: filesystem or encoding failure; stop without inventing success.
