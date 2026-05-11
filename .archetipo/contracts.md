# ARchetipo Connector Contracts

This file is the single entry point for connector operations. Skills read this file to know **how to invoke the CLI** that performs every operation deterministically.

## How It Works

1. Read `.archetipo/config.yaml` to determine the active `connector` (`file` or `github`) and the target paths.
2. Invoke the CLI binary at `.archetipo/bin/archetipo`.
3. Parse the JSON envelope written to stdout. On failure, the JSON envelope on stderr describes the error.

> **Context discipline:** Load this file once at the start of the skill. Do not re-read it unless the skill explicitly requires a refresh.

## Protocol

### Stdout envelope (success)

```json
{"schema":"archetipo/v1","kind":"<kind>","data":{...}}
```

### Stderr envelope (failure)

```json
{"schema":"archetipo/v1","kind":"error","error":{"code":"E_*","message":"...","hint":"..."}}
```

The skill should never branch on `message` (free-text); branch on `code`.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | generic error (includes `E_NOT_FOUND`, `E_CONFLICT`, `E_INTERNAL`) |
| `2` | invalid input (bad flag, malformed structured input) |
| `3` | connector failure (auth, network, gh, fs) |
| `4` | precondition missing (e.g. backlog absent) |

### Error codes (`error.code`)

| Code | Used when |
|---|---|
| `E_INVALID_INPUT` | Bad flag combination or malformed structured input |
| `E_AUTH_SCOPE` | `gh` is missing the required scopes |
| `E_NETWORK` | Transient network failure talking to GitHub |
| `E_CONNECTOR` | Backend (filesystem, gh) reported an unexpected failure |
| `E_PRECONDITION` | A required artifact is missing (no backlog, no eligible story, no plan) |
| `E_NOT_FOUND` | A specific artifact (story, task) referenced by code does not exist |
| `E_CONFLICT` | Operation cannot proceed from the current state (e.g. `story start` on a TODO story) |
| `E_INTERNAL` | Unexpected internal error |

### Configuration

The CLI reads `.archetipo/config.yaml` from the project root (walks up if invoked from a subdir). Defaults: `connector: file` with the canonical paths.

```yaml
connector: file | github
paths:
  prd: docs/PRD.md
  backlog: .archetipo/backlog.yaml
  planning: .archetipo/plans/
  mockups: docs/mockups/
  test_results: docs/test-results/
workflow:
  statuses:
    todo: TODO
    planned: PLANNED
    in_progress: IN PROGRESS
    review: REVIEW
    done: DONE
```

---

## Operation Catalog

The CLI exposes workflow and board operations grouped by entity. Every command emits an envelope with `schema: archetipo/v1`. The `kind` of each envelope is listed below; `data.*` fields follow the schemas in [domain types](#domain-types).

### `archetipo init`

Authenticate, detect repo/project, load metadata. Idempotent.

- **Args:** none
- **Stdin:** none
- **Stdout kind:** `setup` — `data` is a `SetupInfo`.
- **Errors:** `E_AUTH_SCOPE` (gh missing scopes), `E_PRECONDITION` (no project linked).

```bash
.archetipo/bin/archetipo init
```

### `archetipo prd write`

Persist the PRD markdown.

- **Args:** none
- **Stdin:** raw markdown body.
- **Stdout kind:** `write_result`.
- **Errors:** filesystem errors as `E_CONNECTOR`.

```bash
cat PRD.md | .archetipo/bin/archetipo prd write
```

### `archetipo backlog show`

Aggregated read of the backlog: filtered items + idempotency summary in a single envelope.

- **Args:** `--status <STATUS>` (optional) filter `items` by workflow status.
- **Stdin:** none
- **Stdout kind:** `backlog` — `data: {items: Story[], summary: BacklogSummary}`. `summary` is always the full backlog metadata regardless of `--status`.
- **Errors:** none in normal use; an empty backlog returns `items: []` and an empty `summary`.

```bash
.archetipo/bin/archetipo backlog show --status TODO
```

### `archetipo story add`

Idempotent create-or-append on the backlog. Replaces both `backlog save` and `backlog append`: stories whose `code` is already present are skipped and reported in `data.skipped`.

- **Args:** none
- **Args:** `--file <path|->` required. `-` means stdin.
- **Input file:** YAML or JSON payload `{"stories":[Story, ...]}`.
- **Stdout kind:** `write_result` — `data.skipped: string[]` lists codes that were not written because they already existed.
- **Errors:** `E_INVALID_INPUT` (no stories in input payload), `E_CONNECTOR` (filesystem/gh failure).

```bash
.archetipo/bin/archetipo story add --file stories.yaml
.archetipo/bin/archetipo story add --file - < stories.json
```

### `archetipo story show`

Read a story's body together with its task list. Two mutually exclusive forms:

- **Form A (lookup by code):** `archetipo story show US-XXX` → returns the story matching the code.
- **Form B (auto-select):** `archetipo story show --status STATUS` → returns the first eligible story sorted by priority (HIGH > MEDIUM > LOW) then by story code.

If both `<US-XXX>` and `--status` are passed (or neither), the CLI returns `E_INVALID_INPUT`.

- **Stdout kind:** `story` — `data: {story: Story, tasks: Task[]}`. `tasks` is `[]` when the story has no plan yet (not an error).
- **Errors:** `E_PRECONDITION` (no eligible story / story not found), `E_INVALID_INPUT` (form mismatch).

```bash
.archetipo/bin/archetipo story show US-005
.archetipo/bin/archetipo story show --status TODO
```

### `archetipo story plan US-XXX`

Save the implementation plan and transition the story to `PLANNED`. Atomic from the skill's perspective.

- **Args:** `<US-XXX>` (positional, required).
- **Args:** `--file <path|->` required. `-` means stdin.
- **Input file:** YAML or JSON payload `{"plan_body":"<markdown>","tasks":[Task, ...]}`.
- **Stdout kind:** `write_result`.
- **Effect (file):** writes `{paths.planning}/{US-XXX}-plan.yaml` with schema `archetipo/plan/v2`, updates `.archetipo/stories/{US-XXX}.yaml`, and synchronizes `.archetipo/backlog.yaml` board order to `PLANNED`.
- **Effect (github):** appends the plan body to the parent issue, creates one sub-issue per task, then moves the project card to `PLANNED`.
- **Idempotent:** re-running on a `PLANNED` story upserts the plan body without erroring.
- **Errors:** `E_CONFLICT` when the story is past `PLANNED` (e.g. `IN PROGRESS`, `REVIEW`, `DONE`); `E_PRECONDITION` (story not found).

```bash
.archetipo/bin/archetipo story plan US-005 --file plan.yaml
.archetipo/bin/archetipo story plan US-005 --file - < plan.json
```

### `archetipo backlog reorder US-XXX`

Move a story within the linear backlog order without changing its status.

- **Args:** `<US-XXX>` (positional, required).
- **Flags:** one of `--before <US-YYY>` or `--after <US-YYY>` optional. With no anchor, the story moves to the end.
- **Stdout kind:** `write_result`.
- **Effect (file):** updates `.archetipo/backlog.yaml` `orders.backlog`.

```bash
.archetipo/bin/archetipo backlog reorder US-010 --before US-005
```

### `archetipo board move US-XXX`

Move a story inside the board order, optionally changing workflow column and story status.

- **Args:** `<US-XXX>` (positional, required).
- **Flags:** `--to <todo|planned|in_progress|review|done>` required; one of `--before` / `--after` optional.
- **Stdout kind:** `write_result`.
- **Effect (file):** updates `.archetipo/backlog.yaml` `orders.board`; when the target column maps to a different workflow status it also updates `.archetipo/stories/{US-XXX}.yaml`.

```bash
.archetipo/bin/archetipo board move US-010 --to review
.archetipo/bin/archetipo board move US-010 --to planned --after US-003
```

### `archetipo story start US-XXX`

Transition a story from `PLANNED` to `IN PROGRESS`.

- **Args:** `<US-XXX>` (positional, required).
- **Stdin:** none.
- **Stdout kind:** `write_result`.
- **Idempotent:** calling `start` on a story already `IN PROGRESS` is a no-op success.
- **Errors:** `E_CONFLICT` when the story is in any state other than `PLANNED` or `IN PROGRESS`.

```bash
.archetipo/bin/archetipo story start US-005
```

### `archetipo story review US-XXX`

Transition a story from `IN PROGRESS` to `REVIEW`. Optionally posts a closing comment.

- **Args:** `<US-XXX>` (positional, required).
- **Stdin:** raw markdown body, optional. When non-empty, it is posted as a comment on the story (no-op for the file connector — comment is silently ignored).
- **Stdout kind:** `write_result`.
- **Idempotent:** calling `review` on a story already in `REVIEW` is a no-op success; if stdin is non-empty in this case the comment is still posted.
- **Errors:** `E_CONFLICT` when the story is in any state other than `IN PROGRESS` or `REVIEW`.

```bash
echo "Closing notes" | .archetipo/bin/archetipo story review US-005
.archetipo/bin/archetipo story review US-005   # transition only, no comment
```

### `archetipo task done US-XXX TASK-NN`

Mark a single task within a story's plan as completed.

- **Args:** `<US-XXX>` (parent story, positional), `<TASK-NN>` (task code, positional).
- **Stdin:** none.
- **Stdout kind:** `write_result`.
- **Effect (file):** flips the `[ ]` checkbox to `[x]` in the plan file.
- **Effect (github):** closes the sub-issue.
- **Errors:** `E_PRECONDITION` (story or task not found).

```bash
.archetipo/bin/archetipo task done US-005 TASK-01
```

---

## Workflow at a glance

```
                    ┌─ archetipo prd write ──┐
                    │                         │
         (PRD)──────┘                         │
                                              ▼
              ┌─ archetipo story add ──────► YAML backlog store (TODO stories)
              │                                  │
              │                                  ▼
              │             ┌── archetipo story show <code | --status TODO>
              │             │
              │             ▼
              │   archetipo story plan US-XXX  →  PLANNED
              │             │
              │             ▼
              │   archetipo story start US-XXX  →  IN PROGRESS
              │             │
              │             ├── archetipo task done US-XXX TASK-NN  (per task)
              │             ▼
              │   archetipo story review US-XXX  →  REVIEW
              │
              ├─ archetipo backlog reorder US-XXX    (linear backlog order)
              │
              ├─ archetipo board move US-XXX         (board order / status-aware move)
              │
              └─ archetipo backlog show [--status]   (read-only, any time)
```

The CLI does not transition stories to `DONE`; that is left to the human reviewer or the CI/CD pipeline.

---

## Domain types

All field names in JSON are `snake_case`.

### `Story`

```jsonc
{
  "code": "US-001",
  "title": "Login utente",
  "epic": {"code": "EP-001", "title": "Auth Foundations"},
  "priority": "HIGH",            // HIGH | MEDIUM | LOW
  "story_points": 3,
  "status": "TODO",              // value from workflow.statuses
  "blocked_by": ["US-002"],      // optional, strings
  "scope": "MVP",                // optional
  "body": "## Story\n\n...",     // markdown body — produced by the skill
  "ref": "US-001",               // connector-local id (issue number for github)
  "url": "https://..."           // populated when the connector has one
}
```

### `Task`

```jsonc
{
  "id": "TASK-01",
  "title": "Schema DB",
  "description": "Create the users schema",
  "type": "Impl",                // Impl | Test
  "status": "TODO",              // value from workflow.statuses
  "dependencies": ["TASK-00"],   // optional
  "body": "...",                 // optional markdown body (read on github)
  "ref": "TASK-01"               // connector-local id (sub-issue number for github)
}
```

### `Ref`

```jsonc
{"code": "US-001", "number": 42, "path": ".archetipo/backlog.yaml", "url": "https://..."}
```

`number`, `path`, `url` are populated only when the connector has one.

### `WriteResult`

```jsonc
{
  "ok": true,
  "refs": [{"code": "US-003", "path": ".archetipo/backlog.yaml"}],
  "skipped": ["US-001"]          // optional; populated by `story add` for codes already present
}
```

### `SetupInfo`

```jsonc
{
  "connector": "file",
  "paths": { ... },              // mirrors config.yaml paths
  "workflow": { "statuses": { ... } },
  "repo": { "owner": "...", "name": "...", "slug": "owner/name", "node_id": "..." },     // github only
  "project": { "number": 4, "node_id": "...", "url": "...", "fields": { ... } }          // github only
}
```

### `BacklogSummary`

```jsonc
{
  "codes": ["US-001", "US-002"],
  "last_code": "US-002",
  "epics": [{"code": "EP-001", "title": "..."}],
  "titles": ["Login", "Logout"]
}
```

---

## Notes for skill authors

- **Call only what you need.** Not every skill uses every command. Unused commands have zero cost.
- **Domain verbs encode workflow.** The CLI exposes `plan`, `start`, `review` instead of a generic `status set`. Skills don't need to know the literal status strings (`PLANNED`, `IN PROGRESS`, ...) — they just call the verb that matches their phase.
- **Idempotent transitions.** Re-running `story plan / start / review` on a story that is already at the target state is a no-op success. This means a skill can safely retry a step without conditional logic. Calling a verb from a wrong source state returns `E_CONFLICT`.
- **`story add` is idempotent.** Skills that extend the backlog don't need to inspect existing codes first: they pass the full set and the CLI skips duplicates, reporting them in `data.skipped`.
- **`story show` covers both lookup and auto-pick.** Use the positional code form when you know which story to read; use `--status` when you want the next eligible story by priority.
- **Content templates belong to the skill, not to the CLI.** The skill produces the markdown body of stories, plans, comments and PRDs. The file connector persists those bodies inside YAML v2 documents rather than Markdown wrappers.
- **Branch on error `code`, not on `message`.** The CLI guarantees stable codes; messages are human-readable and may change.
- **Compose with stdin/stdout.** Every command that takes content reads it from stdin; every command that returns data writes a single JSON envelope to stdout. Pipe and parse.
