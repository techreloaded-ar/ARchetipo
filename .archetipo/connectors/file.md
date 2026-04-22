# Connector: File System

This file implements the ARchetipo connector contracts for the local file system. This is the default connector when `.archetipo/config.yaml` has `connector: file` or when config.yaml does not exist.

All data is stored as markdown files in the project directory. Paths come from `.archetipo/config.yaml` with these defaults:

```yaml
paths:
  prd: docs/PRD.md
  backlog: docs/BACKLOG.md
  planning: docs/planning/
  mockups: docs/mockups/
  test_results: docs/test-results/
```

---

## SETUP: initialize_connector

No authentication or external service setup is needed. The file connector uses local filesystem paths from config.yaml.

**Outputs:**
- `config.paths.backlog` — path to the backlog file (default: `docs/BACKLOG.md`)
- `config.paths.planning` — path to the planning directory (default: `docs/planning/`)
- `config.paths.prd` — path to the PRD file (default: `docs/PRD.md`)
- `config.paths.mockups` — path to the mockups directory (default: `docs/mockups/`)
- `config.paths.test_results` — path to the test results directory (default: `docs/test-results/`)

Verify that `{config.paths.backlog}` exists. If it does not exist and the calling skill requires a backlog, stop and tell the user to run `archetipo-spec` first.

---

## READ: fetch_backlog_items

Read `{config.paths.backlog}`. Parse story blocks by matching `#### US-\d+:` headers and extracting fields from each block:

- Title from the `#### US-XXX: {Title}` header line
- `**Epic:**` -> epic code and title
- `**Priority:**` -> HIGH / MEDIUM / LOW
- `**Story Points:**` -> numeric value
- `**Status:**` -> current status (TODO, PLANNED, IN PROGRESS, REVIEW, DONE)
- `**Blocked by:**` -> dependency references (US-XXX codes or `-` if none)

If `status_filter` is provided, return only stories matching that status.

If the backlog file does not exist, return an empty list and inform the calling skill.

---

## READ: select_story

Pick a story from the backlog by code or auto-select by priority.

1. **If a story code was passed as argument** (e.g., "US-005"):
   - Find that story in the backlog
   - If not found, list available stories and stop

2. **If a free-text description was passed** (not a US-XXX code):
   - Read the existing backlog to determine the next available US code and existing epics
   - Create a new user story following the standard backlog template:
     - Assign the next available US code
     - Infer the most relevant existing epic (or create EP-NEW if none fits)
     - Infer priority (default MEDIUM) and story points (default 3)
     - Write story text ("As [persona], I want..., so that...") and acceptance criteria
   - Append the new story to `{config.paths.backlog}` in the appropriate epic section
   - Update the **Backlog Summary** table at the top
   - Select the newly added story as the target

3. **If no argument was passed (auto-select):**
   - Among eligible stories (filtered by status criteria from the caller), select highest priority (HIGH > MEDIUM > LOW)
   - Break ties with the lowest story number (US-001 before US-002)
   - If no eligible stories exist, inform the user and stop

---

## READ: read_story_detail

Read the full content of a story from the backlog file.

Find the story block in `{config.paths.backlog}` by matching the `#### US-XXX:` header and extract everything up to the next story header or end of section.

Returns: full story text including acceptance criteria, epic, priority, story points, blocked by, and scope.

---

## READ: read_story_tasks

Read the task list for a story from its planning file.

Read `{config.paths.planning}/{US-CODE}.md` and parse the **Implementation Tasks** table.
The table columns are extracted by **position index**.

**Expected column order (headers rendered in the detected project language):**

| Index | Semantic Field | Example Content | Typical Header (translated) |
|---|---|---|---|
| 1 | Status | `TODO` or `DONE` | "Status" / "Stato" / etc. |
| 2 | Task ID | `TASK-01`, `TASK-02` | "#" |
| 3 | Title | Task name | "Task" / "Titolo" |
| 4 | Description | Brief description (1-2 sentences) | "Description" / "Descrizione" |
| 5 | Type | `Impl` or `Test` | "Type" / "Tipo" |
| 6 | Dependencies | Other `TASK-XX` codes or `-` | "Dependencies" / "Dipendenze" |

For each row, extract:
- Status (TODO / DONE)
- Task ID (TASK-01, TASK-02, etc.)
- Title
- Description
- Type (Impl / Test)
- Dependencies (other TASK-XX codes or `-`)

If the planning file does not exist, stop and tell the user to run `archetipo-plan` first.

---

## READ: read_existing_backlog

Read the existing backlog to determine what stories already exist (for idempotency when extending).

Read `{config.paths.backlog}` and extract:
- All existing story codes (US-XXX)
- Last US-XXX code used (for next code generation)
- Existing epics
- Story titles (for duplicate detection)

---

## WRITE: save_prd

Write the PRD document to `{config.paths.prd}`.

The calling skill provides the complete PRD content. This operation writes (or overwrites) the file.

Create the parent directory if it does not exist.

---

## WRITE: save_initial_backlog

Write the initial backlog to `{config.paths.backlog}`.

The calling skill provides the complete backlog content (formatted markdown with epics, stories, summary table). This operation writes (or overwrites) the file.

Create the parent directory if it does not exist.

---

## WRITE: append_stories

Append new stories to the existing backlog file without rewriting existing content.

1. Read the current backlog from `{config.paths.backlog}`
2. Determine the correct insertion point (within the appropriate epic section, or create a new epic section)
3. Append each new story block
4. Update the **Backlog Summary** table at the top of the file to reflect the new totals
5. Write the updated file

---

## WRITE: save_plan

Save an implementation plan for a story to `{config.paths.planning}/{US-CODE}.md`.

The calling skill provides the complete plan content (formatted markdown with technical solution, test strategy, task table). This operation writes the file.

Create the `{config.paths.planning}/` directory if it does not exist.

If the file already exists, the calling skill should ask the user whether to overwrite or skip before calling this operation.

---

## WRITE: transition_status

Change the workflow status of a story in the backlog file.

1. Read `{config.paths.backlog}`
2. Find the story block by matching the `#### US-XXX:` header
3. Find the `**Status:**` field within that block
4. Replace the current status value with the new status
5. Write the updated file

Example: change `**Status:** TODO` to `**Status:** PLANNED`

---

## WRITE: complete_task

Mark a task as completed in the planning file.

1. Read `{config.paths.planning}/{US-CODE}.md`
2. Find the task row in the **Task di Implementazione** table
3. Change the task status from `TODO` to `DONE`
4. Write the updated file

---

## WRITE: post_comment

Not applicable for the file connector. There is no equivalent of comments on a local file.

The calling skill should skip this operation silently when using the file connector.
