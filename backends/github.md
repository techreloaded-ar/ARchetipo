# Backend: GitHub Projects

This file implements the AIRchetipo backend contracts for GitHub Projects v2. Load it only when `.airchetipo/config.yaml` has `backend: github`.

## Performance Principles

- **Minimize API round-trips.** Every extra `gh` command adds 200-500ms of network latency.
- **Batch with single Bash calls.** Use loops inside a single Bash tool call for label creation, issue creation, and sub-issue creation. Never issue one tool call per item.
- **Batch with GraphQL aliases.** Use aliased mutations to add multiple items to a project or set multiple fields in a single HTTP request.
- **Use mutation responses.** Extract IDs directly from mutation responses instead of re-reading field lists.
- **Maximize parallel tool calls.** When two commands have no data dependency, run them as parallel tool calls.

---

## SETUP: initialize_backend

Authenticate, detect the repository, find the backlog project, and load field metadata.

### Step 1 — Auth & Repository Detection

Detect repository owner, name, slug, and node ID in one command:

```bash
gh repo view --json id,owner,name,nameWithOwner --jq '{owner: .owner.login, name: .name, repo: .nameWithOwner, repoId: .id}'
```

Save:
- `$OWNER`
- `$REPO_NAME`
- `$REPO_SLUG`
- `$REPO_NODE_ID`

Then verify GitHub Projects auth:

```bash
gh project list --owner "$OWNER" --limit 1 --format json
```

If this fails with a scope/permission error, stop and show:

```text
Non ho i permessi necessari per accedere ai GitHub Projects.

Esegui questo comando per abilitare lo scope necessario:
gh auth refresh -s read:project -s project

Poi rilancia la skill.
```

### Step 2 — Project Discovery

Find the project linked to the current git repository:

```bash
gh project list --owner "$OWNER" --format json
```

A project is linked only if its items contain issues whose `content.repository.nameWithOwner` matches `$REPO_SLUG`. To verify, inspect each candidate project's items and compare the repository.

If multiple linked projects exist, prefer:
1. exact title `$REPO_NAME Backlog`
2. otherwise a title containing `Backlog`
3. otherwise the linked project with the lowest project number

If no linked project is found, fall back to an exact title match `$REPO_NAME Backlog`.

If still not found:
- If the calling skill creates backlogs (spec, backlog), create a new project:
  ```bash
  gh project create --owner "$OWNER" --title "$REPO_NAME Backlog"
  ```
- Otherwise, stop and tell the user to run `airchetipo-spec` first to create the backlog.

Save the project number as `$PROJECT_NUMBER`.

### Step 3 — Field Metadata & Items Fetch

Fetch field metadata and project items in **parallel tool calls**:

```bash
gh project field-list $PROJECT_NUMBER --owner "$OWNER" --format json
```

```bash
gh project item-list $PROJECT_NUMBER --owner "$OWNER" --format json -L 200
```

From the field list, extract and save:
- `$PROJECT_NODE_ID` — the project's node ID
- `$STATUS_FIELD_ID` + status option IDs (matching `{config.workflow.statuses}`)
- `$PRIORITY_FIELD_ID` + priority option IDs (if Priority field exists)
- `$SP_FIELD_ID` (if Story Points field exists)
- `$EPIC_FIELD_ID` + epic option IDs (if Epic field exists)

From the item list, save the full item set for later filtering by the calling skill.

---

## SETUP: ensure_project_infrastructure

Create custom fields, status options, epic field, and link the project to the repository. Only needed when creating or extending a backlog.

### Step 1 — Link Project to Repository

Run this mutation once `$PROJECT_NODE_ID` and `$REPO_NODE_ID` are known:

```bash
gh api graphql -f query='mutation {
  linkProjectV2ToRepository(input: {
    projectId: "<PROJECT_NODE_ID>",
    repositoryId: "<REPO_NODE_ID>"
  }) {
    repository {
      id
      nameWithOwner
    }
  }
}'
```

If GitHub reports the repository is already linked, continue without failing. This step is required but idempotent.

Why: downstream skills discover the backlog project from repository context. Without the formal link, the project may be missed.

### Step 2 — Create Missing Fields

Create fields only if not already present (check the field list from `initialize_backend`):

```bash
# Run in a single Bash call — skip commands for fields that already exist
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Priority" --data-type "SINGLE_SELECT" --single-select-options "HIGH,MEDIUM,LOW"
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Story Points" --data-type "NUMBER"
```

### Step 3 — Ensure Status Options

The default Status field only has Todo / In Progress / Done. Add the configured statuses via GraphQL. The `updateProjectV2Field` mutation **replaces ALL options**, so always include existing ones:

```bash
gh api graphql -f query='mutation {
  updateProjectV2Field(input: {
    projectId: "<PROJECT_NODE_ID>",
    fieldId: "<STATUS_FIELD_ID>",
    name: "Status",
    singleSelectOptions: [
      {name: "{config.workflow.statuses.todo}", color: GRAY},
      {name: "{config.workflow.statuses.planned}", color: BLUE},
      {name: "{config.workflow.statuses.in_progress}", color: YELLOW},
      {name: "{config.workflow.statuses.review}", color: PURPLE},
      {name: "{config.workflow.statuses.done}", color: GREEN}
    ]
  }) {
    projectV2Field {
      ... on ProjectV2SingleSelectField { id options { id name } }
    }
  }
}'
```

Save the returned status option IDs directly from the mutation response.

### Step 4 — Create or Update Epic Field

Create the `Epic` single-select field after the epic list is finalized:

```bash
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Epic" --data-type "SINGLE_SELECT" --single-select-options "EP-001: [Title],EP-002: [Title],..."
```

If the Epic field already exists, update it via GraphQL `updateProjectV2Field` mutation to add any new options while preserving existing ones.

Extract the Epic field ID and option IDs from the create/update response. If the response does not include option IDs, do a single field-list re-read.

---

## READ: fetch_backlog_items

Retrieve all items from the backlog project, optionally filtered by status.

Use the item list already fetched during `initialize_backend` (Step 3). If a refresh is needed:

```bash
gh project item-list $PROJECT_NUMBER --owner "$OWNER" --format json -L 200
```

For each item, extract:
- Title (contains US code)
- Status field value
- Priority field value
- Story Points field value
- Epic field value
- Issue number (from `content.number`)

If `status_filter` is provided, return only items matching that status.

> Always use `-L 200` with `gh project item-list` to avoid the default limit of 30 items.

---

## READ: select_story

Pick a story by code or auto-select the highest-priority eligible story.

1. **If a story code was passed as argument** (e.g., "US-005"):
   - Search among the fetched items by title prefix
   - If not found, list available stories and stop

2. **If no argument was passed (auto-select):**
   - Among eligible items (filtered by the caller's status criteria), select highest Priority (HIGH > MEDIUM > LOW)
   - Break ties with the lowest story number (US-001 before US-002)

3. Read the full issue body:
   ```bash
   gh issue view <NUMBER> --json body,title,labels,number,url
   ```

4. Parse the `Blocked by` field from the issue body. If it contains issue references (e.g., `#NN (US-XXX)`), fetch those issue bodies in **parallel tool calls** to load blocking story context:
   ```bash
   gh issue view <BLOCKER_NUMBER> --json body,title,number,url
   ```
   If `Blocked by` is absent or `-`, treat the story as having no dependencies.

> **Free-text story creation** is not supported with the GitHub backend. If the argument is not a US-XXX code, inform the user to create the issue on GitHub first, or run `airchetipo-spec` to add it to the backlog.

---

## READ: read_story_detail

Read the full body/content of a story issue.

```bash
gh issue view <NUMBER> --json body,title,labels,number,url
```

Returns the complete issue body text, title, labels, issue number, and URL.

---

## READ: read_story_tasks

Read the task list for a story from its sub-issues.

```bash
gh api /repos/$OWNER/$REPO_NAME/issues/<PARENT_NUMBER>/sub_issues \
  -H "X-GitHub-Api-Version: 2026-03-10"
```

For each open sub-issue, extract when present:
- Stable identity: GitHub issue number
- Task ID from title (e.g., `TASK-01`)
- Type from `**Tipo:**` field
- Dependencies from `**Dipendenze:**` field
- Prose description from the body
- Completion criteria from `**Completamento:**`

Build the task list with enough structure to schedule execution waves when possible.

> **Note:** Validation policies (what to do when fields are missing or malformed) are defined by the calling skill, not by this backend file. This operation returns raw parsed data; the skill decides how to handle inconsistencies.

---

## READ: read_existing_backlog

Read existing backlog items for idempotency checks when extending the backlog.

```bash
gh issue list --label "airchetipo-backlog" --state all --json number,title,labels,body --limit 200
```

Extract:
- Existing story codes (US-XXX) from titles
- Last US-XXX code used (for next code generation)
- Existing epics from labels
- Current issue numbers (for dependency backfilling)

---

## WRITE: save_initial_backlog

Create the initial backlog from a list of stories. This is a multi-step operation.

### Step 1 — Idempotency Check

```bash
gh issue list --label "airchetipo-backlog" --state all --json number,title --limit 200
```

If issues are found, present options to the user:
- **Skip existing** — create only new stories
- **Recreate** — close existing and create new ones
- **Abort** — cancel the operation

### Step 2 — Create Labels (batch)

Create all labels in a **single Bash call**:

```bash
gh label create "airchetipo-backlog" --description "Story generated by AIRchetipo backlog" --color "1D76DB" --force
gh label create "EP-001: [Epic Title]" --description "[description]" --color "[color]" --force
gh label create "EP-002: [Epic Title]" --description "[description]" --color "[color]" --force
# ... one line per epic
```

### Step 3 — Create Issues (batch loop)

Create all issues in a **single Bash call** using a loop. Each issue body must contain:
- Story (As/I want/So that)
- Demonstrates (visible increment)
- Acceptance Criteria (checkboxes)
- Epic, Priority, Story Points, Blocked by, Scope

Use the `airchetipo-backlog` label plus the epic label.

```bash
gh issue create --title "US-001: [Story Title]" \
  --label "airchetipo-backlog" --label "EP-001: [Epic Title]" \
  --body "$(cat <<'EOF'
## Story

As [persona],
I want [action],
so that [benefit].

## Demonstrates

After implementing this story, the user can: [visible increment]

## Acceptance Criteria

- [ ] [criterion 1]
- [ ] [criterion 2]
- [ ] [criterion 3]

---

**Epic:** EP-XXX - [Epic Title]
**Priority:** HIGH | **Story Points:** N
**Blocked by:** -
**Scope:** MVP

_Created by AIRchetipo backlog_
EOF
)"

# Repeat for each story (all in the same Bash call)
```

### Step 4 — Backfill Dependencies

After all issues are created, for stories with dependencies (`Blocked by` is not `-`), update their issue body to replace symbolic references with actual GitHub issue references (`#NN (US-XXX)`).

Run in a **single Bash call** (only for stories with dependencies):

```bash
gh issue edit <NUMBER> --repo "$OWNER/$REPO_NAME" --body "$(updated body)"
# ... repeat for each story with dependencies
```

### Step 5 — Collect Node IDs

Fetch node IDs for all created issues in a single GraphQL query:

```bash
gh api graphql -f query='query {
  repository(owner: "$OWNER", name: "$REPO_NAME") {
    issues(labels: ["airchetipo-backlog"], last: N, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes { id number title }
    }
  }
}'
```

### Step 6 — Add to Project + Set Fields (batch GraphQL)

**Mutation 1:** Add all issues to the project:

```bash
gh api graphql -f query='mutation {
  add1: addProjectV2ItemById(input: {projectId: "<PROJECT_NODE_ID>", contentId: "<ISSUE_1_NODE_ID>"}) { item { id } }
  add2: addProjectV2ItemById(input: {projectId: "<PROJECT_NODE_ID>", contentId: "<ISSUE_2_NODE_ID>"}) { item { id } }
  # ... one addN per issue
}'
```

**Mutation 2:** Set all fields (Status, Priority, Story Points, Epic) for every item:

```bash
gh api graphql -f query='mutation {
  s1status: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_1_ID>", fieldId: "<STATUS_FIELD_ID>", value: {singleSelectOptionId: "<TODO_OPTION_ID>"}}) { projectV2Item { id } }
  s1priority: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_1_ID>", fieldId: "<PRIORITY_FIELD_ID>", value: {singleSelectOptionId: "<PRIORITY_OPTION_ID>"}}) { projectV2Item { id } }
  s1sp: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_1_ID>", fieldId: "<SP_FIELD_ID>", value: {number: N}}) { projectV2Item { id } }
  s1epic: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_1_ID>", fieldId: "<EPIC_FIELD_ID>", value: {singleSelectOptionId: "<EPIC_OPTION_ID>"}}) { projectV2Item { id } }
  # ... 4 field updates per issue, all in one mutation
}'
```

> **Why two mutations?** `addProjectV2ItemById` returns the item ID needed by `updateProjectV2ItemFieldValue`. You cannot reference one alias's output in another within the same request.

> **Mutation size limit:** GitHub GraphQL has a ~250KB query size limit. For backlogs with 30+ stories, split the field-update mutation into chunks of ~20 stories (80 field updates per mutation).

### Summary Format

```text
Backlog generato su GitHub Projects.

Project: [project URL]

Riepilogo:
- Epiche: N
- User Stories (Issues): N
- Story Points totali: N
- HIGH priority: N storie
- MEDIUM priority: N storie
- LOW priority: N storie

Issues create:
- #NN US-001: [title] (HIGH, 3pt)
- #NN US-002: [title] (HIGH, 2pt)
- ...
```

---

## WRITE: append_stories

Add new stories to an existing backlog without rewriting existing content.

### Step 1 — Read Existing Context

Use `READ: read_existing_backlog` to get existing story codes, epics, and issue numbers.

### Step 2 — Create Missing Labels and Epic Options

If new stories touch a new epic:
- Create the missing epic label in a single Bash call
- Add the missing option to the `Epic` field via `updateProjectV2Field` while preserving existing ones

### Step 3 — Create Only New Issues

Create each new issue using the same body format as `save_initial_backlog`. Use `airchetipo-backlog` and the epic label.

### Step 4 — Backfill Dependencies

For new stories that depend on other stories (new or existing), replace symbolic references with GitHub issue references.

### Step 5 — Add to Project + Set Fields

Same batch GraphQL approach as `save_initial_backlog` (Step 6), but only for the newly created issues.

### Summary Format

```text
Storie aggiunte al backlog GitHub.

Project: [project URL]

Aggiunte:
- #NN US-XXX: [title] (EP-XXX | PRIORITY | Npt)
- #NN US-XXX: [title] (EP-XXX | PRIORITY | Npt)
```

---

## WRITE: save_plan

Save an implementation plan for a story. The strategic plan goes into the parent issue body. Tasks become sub-issues.

> **Important:** With `backend: github`, GitHub is the **single source of truth** for the implementation plan. No local file is written in `{config.paths.planning}/`.

### Step 1 — Detect Epic Label

Read the labels from the parent issue (fetched during story selection). Identify the epic label matching the pattern `EP-XXX`. Save as `$EPIC_LABEL`.

### Step 2 — Update Parent Issue Body

Write the complete implementation plan into the parent issue body. This replaces the original story body with the story content PLUS the plan:

```bash
gh issue edit <NUMBER> --repo "$OWNER/$REPO_NAME" --body "$(cat <<'BODYEOF'
{ORIGINAL_STORY_BODY}

---

## Piano di Implementazione

**Generato da:** AIRchetipo Planning Team
**Data:** {DATE}

### Soluzione Tecnica

{content provided by the calling skill}

### Strategia di Test

{content provided by the calling skill}

### Riepilogo Task

- Task totali: {N}
- I task dettagliati sono nelle sub-issues associate

_Generato da AIRchetipo Planning Team_
BODYEOF
)"
```

> The plan content (technical solution, test strategy) is provided by the calling skill. This backend operation handles persistence only.

### Step 3 — Create Sub-Issues (batch)

Create all sub-issues in **one Bash tool call** using a loop. Sub-issues are the executable task details. Their body structure is defined by the calling skill.

```bash
OWNER="..."
REPO="..."
PARENT=N
LABEL="EP-XXX: ..."
NUMS=()

# TASK-01
URL=$(gh issue create --repo "$OWNER/$REPO" \
  --title "TASK-01: {Title}" \
  --label "$LABEL" \
  --body "$(cat <<'EOF'
{sub-issue body — provided by the calling skill}
EOF
)")
NUMS+=($(echo "$URL" | grep -o '[0-9]*$'))

# TASK-02
URL=$(gh issue create --repo "$OWNER/$REPO" \
  --title "TASK-02: {Title}" \
  --label "$LABEL" \
  --body "$(cat <<'EOF'
{sub-issue body — provided by the calling skill}
EOF
)")
NUMS+=($(echo "$URL" | grep -o '[0-9]*$'))

# ... repeat for all tasks

echo "Created issues: ${NUMS[*]}"
```

Create sub-issues in TASK order (TASK-01 first, then TASK-02, etc.) to maintain logical ordering.

### Step 4 — Link Sub-Issues to Parent (batch)

Link all sub-issues to the parent as native sub-issues in **one Bash tool call**:

```bash
OWNER="..."
REPO="..."
PARENT=N
NUMS=(N N N N)  # actual issue numbers from Step 3

for CHILD_NUMBER in ${NUMS[*]}; do
  CHILD_ID=$(gh api /repos/$OWNER/$REPO/issues/$CHILD_NUMBER --jq '.id')
  gh api -X POST /repos/$OWNER/$REPO/issues/$PARENT/sub_issues \
    -F "sub_issue_id=$CHILD_ID" \
    -H "X-GitHub-Api-Version: 2026-03-10"
done
```

> Sub-issues do NOT appear on the project board. Only issues explicitly added via `addProjectV2ItemById` appear on the board.

---

## WRITE: transition_status

Change the workflow status of a story on the project board.

```bash
gh project item-edit --project-id "<PROJECT_NODE_ID>" --id "<ITEM_ID>" --field-id "<STATUS_FIELD_ID>" --single-select-option-id "<TARGET_STATUS_OPTION_ID>"
```

To get `<ITEM_ID>`, search the project items fetched during `initialize_backend` for the item matching the target issue number.

### Status Transitions Reference

| From | To | Typical Trigger |
|---|---|---|
| {config.workflow.statuses.todo} | {config.workflow.statuses.planned} | Plan skill |
| {config.workflow.statuses.planned} | {config.workflow.statuses.in_progress} | Implement skill |
| {config.workflow.statuses.in_progress} | {config.workflow.statuses.review} | Implement skill (after code review) |
| {config.workflow.statuses.review} | {config.workflow.statuses.done} | Human reviewer only — no skill automates this |

---

## WRITE: complete_task

Mark a task (sub-issue) as completed by closing it.

```bash
gh issue close <SUB_ISSUE_NUMBER> --repo "$OWNER/$REPO_NAME"
```

> **Note:** The calling skill is responsible for determining which sub-issues to close. When task identity is ambiguous (e.g., sequential scheduling with weak task IDs), do not close sub-issues speculatively.

---

## WRITE: post_comment

Post a comment on a story issue.

```bash
gh issue comment <NUMBER> --repo "$OWNER/$REPO_NAME" --body "$(cat <<'EOF'
{comment text — provided by the calling skill}
EOF
)"
```

The comment content (format, sections, data) is defined by the calling skill. This operation handles persistence only.

---

## WRITE: add_label

Add a label to a story issue.

```bash
gh label create "<LABEL_NAME>" --repo "$OWNER/$REPO_NAME" --description "<description>" --color "<color>" --force 2>/dev/null
gh issue edit <NUMBER> --repo "$OWNER/$REPO_NAME" --add-label "<LABEL_NAME>"
```

The `--force` on `label create` makes it idempotent (creates only if not existing).

---

## WRITE: create_labels

Batch-create labels that will be used by stories.

```bash
gh label create "label-1" --repo "$OWNER/$REPO_NAME" --description "..." --color "..." --force
gh label create "label-2" --repo "$OWNER/$REPO_NAME" --description "..." --color "..." --force
# ... all in a single Bash call
```

---

## WRITE: backfill_dependencies

After creating issues, replace symbolic dependency references (US-XXX) with GitHub issue references (#NN).

For each story with dependencies (`Blocked by` is not `-`), update the issue body:

```bash
gh issue edit <NUMBER> --repo "$OWNER/$REPO_NAME" --body "$(updated body with #NN (US-XXX) in Blocked by field)"
# ... repeat for each story with dependencies, all in one Bash call
```

---

## Technical Reference

### Parsing IDs Flow

1. `gh repo view --json id,owner,name,nameWithOwner` -> `$OWNER`, `$REPO_NAME`, `$REPO_SLUG`, `$REPO_NODE_ID`
2. `gh project list --owner "$OWNER" --format json` -> project number + node ID
3. `gh project field-list $N --owner "$OWNER" --format json` -> field IDs + option IDs
4. `gh project item-list $N --owner "$OWNER" --format json -L 200` -> items with field values
5. Status mutation response -> status option IDs
6. Epic field create/update response -> epic option IDs
7. `gh api graphql` (issues query) -> issue node IDs
8. `addProjectV2ItemById` mutation response -> item IDs

Always use `--format json` or GraphQL for machine-parseable output.

### Item List Limit

Always use `-L 200` with `gh project item-list` to avoid the default limit of 30 items.

### JSON Parsing Warning

`gh project item-list --format json` may return JSON with unescaped control characters in `content.body` that break external `jq`. Always use `gh`'s built-in `--jq` flag instead of piping to the system `jq` binary.

### GraphQL Status Options Warning

The `updateProjectV2Field` mutation **replaces ALL options**. Always read existing options first and include them in the mutation to avoid data loss.

### Mutation Size Limit

GitHub GraphQL has a ~250KB query size limit. For bulk operations with 30+ items, split mutations into chunks of ~20 items.

### Board View Tip (first-time setup)

When a project is first created, sub-issues (tasks) appear on the board alongside stories. To hide them, the user should add a board filter: `no:parent-issue`.
