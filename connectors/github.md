# Connector: GitHub Projects

This file implements the AIRchetipo connector contracts for GitHub Projects. Load it only when `.airchetipo/config.yaml` has `connector: github`.

## Performance Principles

- **Minimize API round-trips.** Every extra `gh` command adds 200-500ms of network latency.
- **Batch with single Bash calls.** Use loops inside a single Bash tool call for label creation, issue creation, and sub-issue creation. Never issue one tool call per item.
- **Batch with GraphQL aliases.** Use aliased mutations to add multiple items to a project or set multiple fields in a single HTTP request.
- **Use mutation responses.** Extract IDs directly from mutation responses instead of re-reading field lists.
- **Maximize parallel tool calls.** When two commands have no data dependency, run them as parallel tool calls.

---

## SETUP: initialize_connector

Authenticate, detect the repository, find the backlog project, and load field metadata.

### Step 1 — Auth & Repository Detection

Detect repository owner, name, slug, and node ID in one command:

```bash
gh repo view --json id,owner,name,nameWithOwner --jq '{owner: .owner.login, name: .name, repo: .nameWithOwner, repoId: .id}'
```

Save: `$OWNER`, `$REPO_NAME`, `$REPO_SLUG`, `$REPO_NODE_ID`.

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

> Always use `-L 200` with `gh project item-list` to avoid the default limit of 30 items.

> **JSON parsing:** `gh project item-list --format json` may return JSON with unescaped control characters in `content.body` that break external `jq`. Always use `gh`'s built-in `--jq` flag instead of piping to the system `jq` binary.

From the field list, extract and save:
- `$PROJECT_NODE_ID` — the project's node ID
- `$STATUS_FIELD_ID` + status option IDs (matching `{config.workflow.statuses}`)
- `$PRIORITY_FIELD_ID` + priority option IDs (if Priority field exists)
- `$SP_FIELD_ID` (if Story Points field exists)
- `$EPIC_FIELD_ID` + epic option IDs (if Epic field exists)

From the item list, save the full item set for later filtering by the calling skill.

**Step 4 — Project Infrastructure (new projects only)**

If a new project was created in Step 2, immediately run **Internal: ensure_project_infrastructure** (see below) to create custom fields, status options, and link the repository before returning.

---

## Internal: ensure_project_infrastructure

Create custom fields, status options, epic field, and link the project to the repository. Called internally by `initialize_connector` when a new project is created; not part of the public contract.

### Step 1 — Link Project to Repository

```bash
gh api graphql -f query='mutation {
  linkProjectV2ToRepository(input: {
    projectId: "<PROJECT_NODE_ID>",
    repositoryId: "<REPO_NODE_ID>"
  }) {
    repository { id nameWithOwner }
  }
}'
```

If GitHub reports the repository is already linked, continue without failing. This step is idempotent and required so downstream skills can discover the backlog project from repository context.

### Step 2 — Create Missing Fields

Create fields only if not already present (check the field list from `initialize_connector`):

```bash
# Run in a single Bash call — skip commands for fields that already exist
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Priority" --data-type "SINGLE_SELECT" --single-select-options "HIGH,MEDIUM,LOW"
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Story Points" --data-type "NUMBER"
```

### Step 3 — Ensure Status Options

The default Status field only has Todo / In Progress / Done. The `updateProjectV2Field` mutation **replaces ALL options**, so always include existing ones:

```bash
gh api graphql -f query='mutation {
  updateProjectV2Field(input: {
    fieldId: "<STATUS_FIELD_ID>",
    name: "Status",
    singleSelectOptions: [
      {name: "{config.workflow.statuses.todo}", color: GRAY, description: ""},
      {name: "{config.workflow.statuses.planned}", color: BLUE, description: ""},
      {name: "{config.workflow.statuses.in_progress}", color: YELLOW, description: ""},
      {name: "{config.workflow.statuses.review}", color: PURPLE, description: ""},
      {name: "{config.workflow.statuses.done}", color: GREEN, description: ""}
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

> **Board view tip:** When a project is first created, sub-issues (tasks) appear on the board alongside stories. To hide them, the user should add a board filter: `no:parent-issue`.

---

## READ: fetch_backlog_items

Retrieve all items from the backlog project, optionally filtered by status.

Use the item list already fetched during `initialize_connector` (Step 3). If a refresh is needed:

```bash
gh project item-list $PROJECT_NUMBER --owner "$OWNER" --format json -L 200
```

For each item, extract: Title (contains US code), Status, Priority, Story Points, Epic, Issue number (from `content.number`).

If `status_filter` is provided, return only items matching that status.

---

## READ: select_story

Pick a story by code or auto-select the highest-priority eligible story.

1. **If a story code was passed** (e.g., "US-005"): search among fetched items by title prefix. If not found, list available stories and stop.

2. **If no argument was passed (auto-select):** among eligible items (filtered by the caller's status criteria), select highest Priority (HIGH > MEDIUM > LOW). Break ties with the lowest story number.

3. Read the full issue body:
   ```bash
   gh issue view <NUMBER> --json body,title,labels,number,url
   ```

4. Parse the `Blocked by` field from the issue body. If it contains issue references (e.g., `#NN (US-XXX)`), fetch those issue bodies in **parallel tool calls**. If `Blocked by` is absent or `-`, treat the story as having no dependencies.

> **Free-text story creation** is not supported with the GitHub connector. If the argument is not a US-XXX code, inform the user to create the issue on GitHub first, or run `airchetipo-spec` to add it to the backlog.

---

## READ: read_story_detail

Read the full body/content of a story issue.

```bash
gh issue view <NUMBER> --json body,title,labels,number,url
```

---

## READ: read_story_tasks

Read the task list for a story from its sub-issues.

```bash
gh api repos/$OWNER/$REPO_NAME/issues/<PARENT_NUMBER>/sub_issues \
  -H "X-GitHub-Api-Version: 2026-03-10"
```

For each open sub-issue, extract when present: Task ID from title (e.g., `TASK-01`), Type from `**Tipo:**`, Dependencies from `**Dipendenze:**`, prose description from the body, Completion criteria from `**Completamento:**`.

Build the task list with enough structure to schedule execution waves when possible.

> Validation policies (what to do when fields are missing or malformed) are defined by the calling skill, not by this connector file.

---

## READ: read_existing_backlog

Read existing backlog items for idempotency checks when extending the backlog.

```bash
gh issue list --label "airchetipo-backlog" --state all --json number,title,labels,body --limit 200
```

Extract: existing story codes (US-XXX) from titles, last US-XXX code used, existing epics from labels, current issue numbers (for dependency backfilling).

---

## WRITE: save_initial_backlog

Create the initial backlog from a list of stories. This is a multi-step operation.

### Step 1 — Idempotency Check

```bash
gh issue list --label "airchetipo-backlog" --state all --json number,title --limit 200
```

If issues are found, present options: **Skip existing**, **Recreate** (close existing and create new), or **Abort**.

### Step 2 — Create Labels (batch)

Run **Internal: create_labels** with:
- `airchetipo-backlog` — description: "Story generated by AIRchetipo backlog", color: `1D76DB`
- one label per epic: `EP-XXX: [Epic Title]` — description: epic description, color: choose a distinct color per epic

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

As [persona], I want [action], so that [benefit].

## Demonstrates

After implementing this story, the user can: [visible increment]

## Acceptance Criteria

- [ ] [criterion 1]
- [ ] [criterion 2]

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

Run **Internal: backfill_dependencies** for all stories where `Blocked by` is not `-`.

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

**Mutation 1 — Add all issues to the project:**

```bash
gh api graphql -f query='mutation {
  add1: addProjectV2ItemById(input: {projectId: "<PROJECT_NODE_ID>", contentId: "<ISSUE_1_NODE_ID>"}) { item { id } }
  add2: addProjectV2ItemById(input: {projectId: "<PROJECT_NODE_ID>", contentId: "<ISSUE_2_NODE_ID>"}) { item { id } }
}'
```

**Mutation 2 — Set all fields** (Status, Priority, Story Points, Epic) for every item. Use 4 aliased `updateProjectV2ItemFieldValue` calls per item (one per field).

> **Why two mutations?** `addProjectV2ItemById` returns the item ID needed by `updateProjectV2ItemFieldValue`. You cannot reference one alias's output in another within the same request.

> **Batch mutation notes:**
> - GitHub GraphQL has a ~250KB query size limit. For backlogs with 30+ stories, split mutations into chunks of ~20 stories (~80 field updates per mutation).
> - When a batch mutation with 100+ aliases exceeds bash quoting limits, write the query to a temporary file:
>   ```bash
>   cat > /tmp/mutation.graphql <<'GQLEOF'
>   mutation { ... }
>   GQLEOF
>   QUERY=$(cat /tmp/mutation.graphql) && gh api graphql -f query="$QUERY"
>   ```
>   Do NOT use `gh api graphql --input file` — `--input` expects a JSON body, not a raw GraphQL query.

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

1. Use `READ: read_existing_backlog` to get existing story codes, epics, and issue numbers.
2. If new stories introduce a new epic, run **Internal: create_labels** for the missing epic label(s). Then add the new option(s) to the `Epic` field via `updateProjectV2Field` (preserving existing options).
3. Create new issues (follow Step 3 of `save_initial_backlog`, applied only to the new stories).
4. Run **Internal: backfill_dependencies** for any new story where `Blocked by` is not `-`.
5. Collect node IDs and add to project with field values (follow Steps 5-6 of `save_initial_backlog`, applied only to the new stories).

### Summary Format

```text
Storie aggiunte al backlog GitHub.

Project: [project URL]

Aggiunte:
- #NN US-XXX: [title] (EP-XXX | PRIORITY | Npt)
```

---

## WRITE: save_plan

Save an implementation plan for a story. The strategic plan goes into the parent issue body. Tasks become sub-issues.

> With `connector: github`, GitHub is the **single source of truth** for the implementation plan. No local file is written in `{config.paths.planning}/`.

### Step 1 — Detect Epic Label

Read the labels from the parent issue (fetched during story selection). Identify the epic label matching the pattern `EP-XXX`. Save as `$EPIC_LABEL`.

### Step 2 — Update Parent Issue Body

Append the implementation plan to the original story body:

```bash
gh issue edit <NUMBER> --repo "$OWNER/$REPO_NAME" --body "$(cat <<'BODYEOF'
{ORIGINAL_STORY_BODY}

---

## Piano di Implementazione

**Generato da:** AIRchetipo Planning Team | **Data:** {DATE}

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

### Step 3 — Create Sub-Issues (batch)

Create all sub-issues in **one Bash tool call** using a loop. Sub-issue body structure is defined by the calling skill.

```bash
OWNER="..." REPO="..." PARENT=N LABEL="EP-XXX: ..."
NUMS=()

# TASK-01
URL=$(gh issue create --repo "$OWNER/$REPO" \
  --title "TASK-01: {Title}" --label "$LABEL" \
  --body "$(cat <<'EOF'
{sub-issue body — provided by the calling skill}
EOF
)")
NUMS+=($(echo "$URL" | grep -o '[0-9]*$'))

# ... repeat for all tasks in TASK order
echo "Created issues: ${NUMS[*]}"
```

### Step 4 — Link Sub-Issues to Parent (batch)

Link all sub-issues to the parent as native sub-issues in **one Bash tool call**:

```bash
OWNER="..." REPO="..." PARENT=N
NUMS=(N N N N)  # actual issue numbers from Step 3

for CHILD_NUMBER in ${NUMS[*]}; do
  CHILD_ID=$(gh api repos/$OWNER/$REPO/issues/$CHILD_NUMBER --jq '.id')
  gh api -X POST repos/$OWNER/$REPO/issues/$PARENT/sub_issues \
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

To get `<ITEM_ID>`, search the project items fetched during `initialize_connector` for the item matching the target issue number.

| From | To | Typical Trigger |
|---|---|---|
| {config.workflow.statuses.todo} | {config.workflow.statuses.planned} | Plan skill |
| {config.workflow.statuses.planned} | {config.workflow.statuses.in_progress} | Implement skill |
| {config.workflow.statuses.in_progress} | {config.workflow.statuses.review} | Implement skill (after code review) |
| {config.workflow.statuses.review} | {config.workflow.statuses.done} | Human reviewer only |

After updating the project board status, also run **Internal: add_label** (see below) with the new status name as the label. This keeps GitHub issue labels in sync with project board status for visibility in issue lists.

---

## WRITE: complete_task

Mark a task (sub-issue) as completed by closing it. The calling skill determines which sub-issues to close.

```bash
gh issue close <SUB_ISSUE_NUMBER> --repo "$OWNER/$REPO_NAME"
```

---

## WRITE: post_comment

Post a comment on a story issue. The comment content is defined by the calling skill.

```bash
gh issue comment <NUMBER> --repo "$OWNER/$REPO_NAME" --body "$(cat <<'EOF'
{comment text — provided by the calling skill}
EOF
)"
```

---

## Internal: add_label

Add a label to a story issue (creates the label if it doesn't exist). Called internally by `transition_status`; not part of the public contract.

```bash
gh label create "<LABEL_NAME>" --repo "$OWNER/$REPO_NAME" --description "<description>" --color "<color>" --force 2>/dev/null
gh issue edit <NUMBER> --repo "$OWNER/$REPO_NAME" --add-label "<LABEL_NAME>"
```

---

## Internal: create_labels

Batch-create labels in a **single Bash call**. Called internally by `save_initial_backlog` and `append_stories`; not part of the public contract.

```bash
gh label create "label-1" --repo "$OWNER/$REPO_NAME" --description "..." --color "..." --force
gh label create "label-2" --repo "$OWNER/$REPO_NAME" --description "..." --color "..." --force
```

---

## Internal: backfill_dependencies

After creating issues, replace symbolic dependency references (US-XXX) with GitHub issue references (`#NN (US-XXX)`) in the `Blocked by` field. Called internally by `save_initial_backlog` and `append_stories`; not part of the public contract.

Update all affected issue bodies in a **single Bash call**.
