# Backend: GitHub Projects v2

> This file is loaded when `.airchetipo/config.yaml` has `backend: github`.
> It overrides the I/O phases of the backlog skill while keeping domain logic identical.
>
> **Performance principle:** Minimize API round-trips. Use single Bash calls with loops for batch operations, GraphQL mutations with aliases for bulk field updates, and avoid redundant reads. Every extra `gh` command adds 200-500ms of network latency.

## Setup

### Step 1 — Auth & Project Discovery (single pass)

Detect owner and discover the project in one flow:

```bash
gh repo view --json owner,name --jq '{owner: .owner.login, name: .name}'
```

Save `$OWNER` and `$REPO_NAME`. Then list projects (this also verifies auth):

```bash
gh project list --owner "$OWNER" --format json
```

If this fails with a scope/permission error, show this message and **stop**:

```
🔎 **Emanuele:** Non ho i permessi necessari per accedere ai GitHub Projects.

Esegui questo comando per abilitare lo scope necessario:
\`\`\`
gh auth refresh -s read:project -s project
\`\`\`

Poi rilancia `/airchetipo-backlog`.
```

From the project list, look for a project whose title contains "Backlog".
- **If found:** Ask the user for confirmation: "Ho trovato il project '[title]' (#N). Vuoi usare questo?"
- **If not found:** Create a new project:
  ```bash
  gh project create --owner "$OWNER" --title "$REPO_NAME Backlog"
  ```

Save the project number as `$PROJECT_NUMBER` for all subsequent commands.

### Step 2 — Custom Fields & Status Setup

Read existing fields once — this is the only field-list read needed for the entire Setup phase:

```bash
gh project field-list $PROJECT_NUMBER --owner "$OWNER" --format json
```

From this response, extract and save:
- `$PROJECT_NODE_ID` — the project's node ID
- `$STATUS_FIELD_ID` + existing status option IDs
- `$PRIORITY_FIELD_ID` + option IDs (if Priority field exists)
- `$SP_FIELD_ID` (if Story Points field exists)

Create missing fields (only if not already present):

```bash
# Run in a single Bash call — skip commands for fields that already exist
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Priority" --data-type "SINGLE_SELECT" --single-select-options "HIGH,MEDIUM,LOW"
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Story Points" --data-type "NUMBER"
```

**Epic** field: created AFTER Phase 2, once epics are known.

### Step 2b — Remove auto-add workflows

After extracting field metadata, delete the "Auto-add sub-issues to project" workflow (and any other enabled auto-add workflows). This prevents GitHub from automatically adding sub-issues created by `airchetipo-plan` to the board — only explicit `addProjectV2ItemById` calls should add items.

The GraphQL API does not support disabling workflows (`updateProjectV2Workflow` does not exist) — use `deleteProjectV2Workflow` instead.

```bash
# Find and delete all enabled workflows (auto-add sub-issues, etc.)
WORKFLOWS=$(gh api graphql -f query='query {
  node(id: "'$PROJECT_NODE_ID'") {
    ... on ProjectV2 {
      workflows(first: 20) {
        nodes { id name enabled }
      }
    }
  }
}' --jq '.data.node.workflows.nodes[] | select(.enabled == true) | .id')

for WF_ID in $WORKFLOWS; do
  gh api graphql -f query='mutation {
    deleteProjectV2Workflow(input: {
      workflowId: "'$WF_ID'"
    }) {
      deletedWorkflowId
    }
  }'
done
```

> **Note:** This step is idempotent — running it on a project with no enabled workflows is a no-op (the query returns nothing). Built-in disabled workflows (Item closed, Pull request merged, etc.) are left untouched.

### Step 3 — Status Options Setup

The default Status field only has Todo / In Progress / Done. Add the missing statuses from `{config.workflow.statuses}` via GraphQL. The `updateProjectV2Field` mutation replaces ALL options, so always include existing ones.

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

Save the option IDs directly from the mutation response — no need to re-read the field list.

### Announce startup (GitHub variant)

```
📋 AIRCHETIPO - BACKLOG GENERATION (GitHub Projects)

🔎 Emanuele and 💎 Andrea are ready to decompose your PRD into a prioritized backlog.

PRD found: [file path]
GitHub Project: [project title] (#N)
Owner: [owner]

Analyzing requirements...
```

---

## Write Output

### Step 1 — Idempotency Check

Search for existing issues with the `airchetipo-backlog` label:
```bash
gh issue list --label "airchetipo-backlog" --state all --json number,title --limit 200
```

If issues are found, present options to the user:
```
🔎 **Emanuele:** Ho trovato [N] issue esistenti con label `airchetipo-backlog`.

Opzioni:
1. **Skip existing** — creo solo le story nuove
2. **Recreate** — chiudo le vecchie e ne creo di nuove
3. **Abort** — annullo l'operazione

Cosa preferisci?
```

### Step 2 — Create Labels (batch)

Create all labels in a **single Bash call**:

```bash
gh label create "airchetipo-backlog" --description "Story generated by AIRchetipo backlog" --color "1D76DB" --force
gh label create "EP-001: [Epic Title]" --description "[Epic one-line description]" --color "[color]" --force
gh label create "EP-002: [Epic Title]" --description "[Epic one-line description]" --color "[color]" --force
# ... one line per epic
```

### Step 3 — Create Epic Field

Create the Epic custom field with all options:
```bash
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Epic" --data-type "SINGLE_SELECT" --single-select-options "EP-001: [Title],EP-002: [Title],..."
```

If the Epic field already exists, update it via GraphQL `updateProjectV2Field` mutation to add any new options while preserving existing ones.

Extract the Epic field ID and option IDs directly from the **create/update response**. If the response does not include option IDs, do a single field-list read:
```bash
gh project field-list $PROJECT_NUMBER --owner "$OWNER" --format json
```

### Step 4 — Create Issues (batch loop)

Create all issues in a **single Bash call** using a loop. Collect the issue URL and node ID from each creation for later use:

```bash
# Create all issues in one Bash execution, saving URLs and node IDs
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

**Epic:** EP-XXX — [Epic Title]
**Priority:** HIGH | **Story Points:** N
**Blocked by:** -
**Scope:** MVP

_Created by AIRchetipo backlog_
EOF
)"

# Repeat for each story (all in the same Bash call)
gh issue create --title "US-002: [Story Title]" ...
gh issue create --title "US-003: [Story Title]" ...
# ...
```

### Step 4b — Backfill Blocked by References

After creating all issues, for stories that have dependencies (`Blocked by` is not `-`), update their issue body to replace `-` with the actual GitHub issue references. Use the issue numbers collected from Step 4 to build `#NN (US-XXX)` references.

Run in a **single Bash call** (only for stories with dependencies):

```bash
# Only for stories with Blocked by != "-"
gh issue edit <NUMBER> --repo "$OWNER/$REPO" --body "$(updated body with #NN (US-XXX) references in the Blocked by field)"
# ... repeat for each story with dependencies
```

> **Note:** This adds at most N extra API calls, but only for the subset of stories that have dependencies (typically a small subset of the backlog).

### Step 4c — Collect Node IDs

After creating all issues, collect their node IDs in a single query:

```bash
gh api graphql -f query='query {
  repository(owner: "$OWNER", name: "$REPO_NAME") {
    issues(labels: ["airchetipo-backlog"], last: N, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes { id number title }
    }
  }
}'
```

### Step 5 — Add to Project + Set All Fields (batch GraphQL)

Use a **single GraphQL mutation** to add all issues to the project and set all custom fields. GraphQL aliases allow batching multiple operations in one HTTP request:

```bash
gh api graphql -f query='mutation {
  # --- Add all issues to project ---
  add1: addProjectV2ItemById(input: {projectId: "<PROJECT_NODE_ID>", contentId: "<ISSUE_1_NODE_ID>"}) { item { id } }
  add2: addProjectV2ItemById(input: {projectId: "<PROJECT_NODE_ID>", contentId: "<ISSUE_2_NODE_ID>"}) { item { id } }
  add3: addProjectV2ItemById(input: {projectId: "<PROJECT_NODE_ID>", contentId: "<ISSUE_3_NODE_ID>"}) { item { id } }
  # ... one addN per issue
}'
```

From the response, extract each item ID (`add1.item.id`, `add2.item.id`, ...). Then set all fields in a **second GraphQL mutation**:

```bash
gh api graphql -f query='mutation {
  # --- Issue 1: set Status, Priority, Story Points, Epic ---
  s1status: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_1_ID>", fieldId: "<STATUS_FIELD_ID>", value: {singleSelectOptionId: "<TODO_OPTION_ID>"}}) { projectV2Item { id } }
  s1priority: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_1_ID>", fieldId: "<PRIORITY_FIELD_ID>", value: {singleSelectOptionId: "<HIGH_OPTION_ID>"}}) { projectV2Item { id } }
  s1sp: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_1_ID>", fieldId: "<SP_FIELD_ID>", value: {number: 3}}) { projectV2Item { id } }
  s1epic: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_1_ID>", fieldId: "<EPIC_FIELD_ID>", value: {singleSelectOptionId: "<EPIC_OPTION_ID>"}}) { projectV2Item { id } }

  # --- Issue 2: set Status, Priority, Story Points, Epic ---
  s2status: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_2_ID>", fieldId: "<STATUS_FIELD_ID>", value: {singleSelectOptionId: "<TODO_OPTION_ID>"}}) { projectV2Item { id } }
  s2priority: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_2_ID>", fieldId: "<PRIORITY_FIELD_ID>", value: {singleSelectOptionId: "<MEDIUM_OPTION_ID>"}}) { projectV2Item { id } }
  s2sp: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_2_ID>", fieldId: "<SP_FIELD_ID>", value: {number: 2}}) { projectV2Item { id } }
  s2epic: updateProjectV2ItemFieldValue(input: {projectId: "<PROJECT_NODE_ID>", itemId: "<ITEM_2_ID>", fieldId: "<EPIC_FIELD_ID>", value: {singleSelectOptionId: "<EPIC_OPTION_ID>"}}) { projectV2Item { id } }

  # ... repeat for each issue (4 field updates per issue, all in one mutation)
}'
```

> **Why two mutations instead of one?** The `addProjectV2ItemById` returns the item ID needed by `updateProjectV2ItemFieldValue`. GraphQL mutations within a single request execute sequentially but you cannot reference one alias's output in another alias's input. So: mutation 1 adds all items → extract item IDs from response → mutation 2 sets all fields.

> **Mutation size limit:** GitHub GraphQL has a ~250KB query size limit. For backlogs with 30+ stories, split the field-update mutation into chunks of ~20 stories (80 field updates per mutation). This is rarely needed for typical backlogs.

---

## Output Summary Format

After all issues are created, output:

```
✅ Backlog generated successfully on GitHub Projects!

🔗 Project: [project URL]

📊 Summary:
- Epics: N
- User Stories (Issues): N
- Total Story Points: N
- HIGH priority: N stories
- MEDIUM priority: N stories
- LOW priority: N stories

📋 Issues created:
- #NN US-001: [title] (HIGH, 3pt)
- #NN US-002: [title] (HIGH, 2pt)
- ...
```

---

## Technical Reference

### API Call Budget

The optimized flow uses approximately:

| Phase | Calls | Notes |
|---|---|---|
| Setup (auth + project + fields + status) | 4-6 | Down from 8 — merged owner+project discovery, use mutation responses |
| Idempotency check | 1 | Unchanged |
| Labels | 1 | Single Bash call with all labels |
| Epic field | 1-2 | Create + optional field-list re-read |
| Issue creation | N | One `gh issue create` per story (unavoidable via CLI) |
| Dependency backfill | 0-N | One `gh issue edit` per story with dependencies (typically few) |
| Fetch node IDs | 1 | Single GraphQL query |
| Add to project | 1 | Single GraphQL mutation with aliases |
| Set all fields | 1-2 | Single GraphQL mutation (split if 30+ stories) |

**Total for 20 stories: ~30 API calls** (down from ~136 in the unoptimized version).

### Parsing IDs Flow

1. `gh repo view --json owner,name` → `$OWNER`, `$REPO_NAME`
2. `gh project list --owner "$OWNER" --format json` → project number + node ID
3. `gh project field-list $N --owner "$OWNER" --format json` → field IDs + option IDs
4. Status mutation response → status option IDs
5. Epic field create/update response → epic option IDs
6. `gh api graphql` (issues query) → issue node IDs
7. `addProjectV2ItemById` mutation response → item IDs

Always use `--format json` or GraphQL to get machine-parseable output.

### Item List Limit

Always use `-L 200` with `gh project item-list` to avoid the default limit of 30 items.

### GraphQL for Status Options

The `updateProjectV2Field` mutation replaces ALL options. Always read existing options first and include them in the mutation to avoid data loss.
