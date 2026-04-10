# AIRchetipo Backend Contracts

This file is the single entry point for all backend operations. Skills read this file instead of managing backend-specific logic themselves.

## How It Works

1. Read `.airchetipo/config.yaml` to determine the `backend` value (default: `file`)
2. Read `.airchetipo/backends/{backend}.md` — that file contains the implementation of every operation listed below
3. When a skill references an operation (e.g., `SETUP: initialize_backend`), find the matching section header in the loaded backend file and follow its instructions

> **Context discipline:** Load this file and the backend file once at the start of the skill. Do not re-read them unless the skill explicitly requires a refresh.

## Configuration

The backend file receives these values from `.airchetipo/config.yaml`:

```yaml
backend: file | github          # which backend implementation to load
paths:                          # filesystem paths (used by all backends for PRD, mockups, etc.)
  prd: docs/PRD.md
  backlog: docs/BACKLOG.md      # primary source for file backend
  planning: docs/planning/
  mockups: docs/mockups/
  test_results: docs/test-results/
workflow:
  statuses:                     # status labels used by transition_status
    todo: TODO
    planned: PLANNED
    in_progress: IN PROGRESS
    review: REVIEW
    done: DONE
```

If `.airchetipo/config.yaml` does not exist, assume `backend: file` with the default paths above.

---

## Operation Catalog

### SETUP

| Operation | Description | Inputs | Outputs |
|---|---|---|---|
| `initialize_backend` | Authenticate, detect repository, find or create the project/backlog, load field metadata | config values | `$OWNER`, `$REPO_NAME`, `$REPO_SLUG`, `$PROJECT_NUMBER`, `$PROJECT_NODE_ID`, field metadata (backend-specific; file backend outputs config paths only) |
| `ensure_project_infrastructure` | Create custom fields, status options, epic field, link project to repository. Only needed when creating or extending a backlog for the first time. | `$PROJECT_NUMBER`, `$OWNER` | field IDs, option IDs |

### READ

| Operation | Description | Inputs | Outputs |
|---|---|---|---|
| `fetch_backlog_items(status_filter?)` | Retrieve all items from the backlog, optionally filtered by status | optional status filter | list of stories with: code, title, epic, priority, story points, status, blocked_by |
| `select_story(code_or_auto, eligible_statuses)` | Pick a specific story by code, or auto-select the highest-priority story matching the eligible statuses | story code OR `auto`, list of eligible statuses | single story reference with full metadata |
| `read_story_detail(reference)` | Read the full body/content of a story | story reference (US code or issue number) | story body text (acceptance criteria, scope, context) |
| `read_story_tasks(parent_reference)` | Read the task list for a story (sub-issues or planning file task table) | parent story reference | ordered list of tasks with: id, title, description, status, dependencies |
| `read_existing_backlog()` | Read existing backlog items for idempotency checks (avoid duplicates when extending) | — | list of existing story codes and titles |

### WRITE

| Operation | Description | Inputs | Outputs |
|---|---|---|---|
| `save_initial_backlog(stories[])` | Create the initial backlog from a list of stories. Handles all persistence: file creation, issue creation, project board setup, field assignment, labels, dependencies | array of story objects (code, title, epic, priority, story_points, acceptance_criteria, blocked_by, scope) | confirmation + references to created items |
| `append_stories(stories[])` | Add new stories to an existing backlog without rewriting existing content | array of story objects (same format as above) | confirmation + references to created items |
| `save_plan(story, strategic_plan, tasks[])` | Save an implementation plan for a story. The strategic plan goes into the main document/issue body. Tasks become individual trackable items (file sections or sub-issues) | story reference, plan markdown, array of task objects | confirmation + references to created items |
| `transition_status(story, new_status)` | Change the workflow status of a story | story reference, target status label | confirmation |
| `complete_task(task)` | Mark a single task as completed | task reference | confirmation |
| `post_comment(story, text)` | Post a comment on a story (completion summary, review notes, etc.) | story reference, comment text | confirmation (no-op for backends without comment support) |
| `add_label(story, label)` | Add a label/tag to a story | story reference, label name | confirmation (no-op for backends without label support) |
| `create_labels(labels[])` | Batch-create labels/tags that will be used by stories | array of label definitions | confirmation (no-op for backends without label support) |
| `backfill_dependencies(stories_map)` | Replace symbolic dependency references (e.g., US-003) with backend-native references (e.g., #42) | map of story codes to backend references | confirmation (no-op for backends with text-based references) |

---

## Notes for Skill Authors

- **Call only what you need.** Not every skill uses every operation. Unused operations have zero cost.
- **Content templates belong in the skill, not here.** The skill defines *what* to write (plan format, sub-issue body template, story structure). The backend defines *how* to persist it.
- **Validation policies belong in the skill.** Post-processing and validation of data returned by READ operations (e.g., malformed task parsing, confidence thresholds) is the skill's responsibility.
- **No-op operations are explicit.** If a backend does not support an operation (e.g., file backend has no comments), the backend file says so. The skill should not fail — it simply skips that step.
