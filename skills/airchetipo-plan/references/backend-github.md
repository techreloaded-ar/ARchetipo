# Backend: GitHub Projects v2

> This file is loaded when `.airchetipo/config.yaml` has `backend: github`.
> It overrides the I/O phases of the plan skill while keeping domain logic identical.

## Setup

### Step 1 — Auth check & owner detection

1. Detect repository owner:
   ```bash
   gh repo view --json owner --jq '.owner.login'
   ```
   Save as `$OWNER`.

2. Test GitHub Projects auth:
   ```bash
   gh project list --owner "$OWNER" --limit 1 --format json
   ```
   If this fails with a scope/permission error, show fix and **stop**:

```
🔎 **Emanuele:** Non ho i permessi necessari per accedere ai GitHub Projects.

Esegui questo comando per abilitare lo scope necessario:
\`\`\`
gh auth refresh -s read:project -s project
\`\`\`

Poi rilancia la skill.
```

### Step 2 — Project discovery

1. Find the Backlog project:
   ```bash
   gh project list --owner "$OWNER" --format json
   ```
   Look for a project whose title contains "Backlog".

2. If not found, show message and **stop**:
```
🔎 **Emanuele:** Non trovo un GitHub Project con "Backlog" nel titolo.

Esegui prima `/airchetipo-backlog-gh` per creare il project e le issue.
```

3. Save `$PROJECT_NUMBER` and fetch field metadata:
   ```bash
   gh project field-list $PROJECT_NUMBER --owner "$OWNER" --format json
   ```
   Save field IDs and option IDs (Status options matching `{config.workflow.statuses}`: todo, planned, in_progress, review, done; plus Priority, Story Points, Epic).

### Step 3 — Fetch and filter items

1. Fetch all items:
   ```bash
   gh project item-list $PROJECT_NUMBER --owner "$OWNER" --format json -L 200
   ```

2. Filter to items where:
   - Status == {config.workflow.statuses.todo}
   - Does NOT have label `planned`

3. If no eligible items found, inform the user and **stop**:
```
🔎 **Emanuele:** Non ci sono story in "{config.workflow.statuses.todo}" senza label `planned` nel project.

Tutte le story sono già pianificate o in lavorazione.
```

## Read Backlog (Story Source)

### Story selection from GitHub Project

1. **If a story code was passed as argument** (e.g., "US-005"):
   - Search for it among the filtered items by title prefix
   - If not found, list available stories and let the user choose

2. **If NO argument was passed:**
   - Among eligible items, select the one with highest Priority (HIGH > MEDIUM > LOW)
   - Among equal priorities, select the lowest story number

3. Read the full issue body:
   ```bash
   gh issue view <NUMBER> --json body,title,labels,number,url
   ```

> **Note:** Free-text story creation is not supported with the GitHub backend. If the argument is not a US-XXX code, inform the user to create the issue on GitHub first.

## Write Output

After saving the planning document in `{config.paths.planning}/{US-CODE}.md`:

### Step 1 — Detect epic label

Read the labels from the parent issue (fetched during story selection). Identify the epic label matching the pattern `EP-XXX`. Save it as `$EPIC_LABEL` — it will be applied to all sub-issues.

### Step 2 — Create sub-issues for each TASK

For each TASK-XX defined in the implementation plan, create a GitHub issue and associate it as a native sub-issue of the parent:

```bash
gh issue create \
  --title "TASK-XX: {Task Title}" \
  --label "$EPIC_LABEL" \
  --body "$(cat <<'TASKEOF'
**Parent Story:** #{PARENT_ISSUE_NUMBER} — {US-CODE}: {Story Title}

| Campo | Valore |
|---|---|
| **Tipo** | {Implementazione / Test} |
| **Dipendenze** | {nessuna / TASK-YY} |
| **Effort stimato** | {S / M / L} |

## Descrizione

{DETAILED_TASK_DESCRIPTION}

## File Coinvolti

- `{file_path}` — {crea/modifica}: {cosa fare}

## Criteri di Completamento

- [ ] {COMPLETION_CRITERION_1}
- [ ] {COMPLETION_CRITERION_2}

---
_Sub-issue generata da AIRchetipo Planning Team_
TASKEOF
)"
```

Save the created issue number for each sub-issue.

After creating each sub-issue, associate it as a native sub-issue of the parent:

```bash
# Get the database ID of the child issue (required by the REST API, different from the issue number)
CHILD_ID=$(gh api /repos/$OWNER/$REPO/issues/$CHILD_NUMBER --jq '.id')

# Add as native sub-issue
gh api -X POST /repos/$OWNER/$REPO/issues/$PARENT_NUMBER/sub_issues \
  -f "sub_issue_id=$CHILD_ID" \
  -H "X-GitHub-Api-Version: 2026-03-10"
```

**Important:** Create sub-issues in TASK order (TASK-01 first, then TASK-02, etc.) to maintain logical ordering.

### Step 3 — Update the parent issue body

1. Read the current body:
   ```bash
   gh issue view <NUMBER> --json body --jq '.body'
   ```

2. Build the updated body by appending the plan section. Native sub-issues appear automatically in the GitHub UI, so no tasklist block is needed:

   ```bash
   UPDATED_BODY=$(cat <<BODYEOF
   ${CURRENT_BODY}

   ---

   ## 📋 Piano di Implementazione

   **File:** \`{config.paths.planning}/{US-CODE}.md\`

   **Riepilogo:**
   - Task totali: {N} ({N} implementazione + {N} test)
   - Effort stimato: {total}

   _Generato da AIRchetipo Planning Team_
   BODYEOF
   )
   ```

3. Update the issue:
   ```bash
   gh issue edit <NUMBER> --body "$UPDATED_BODY"
   ```

### Step 4 — Add `planned` label and move Status to {config.workflow.statuses.planned}

```bash
gh label create "planned" --description "Story has an implementation plan" --color "0E8A16" --force
gh issue edit <NUMBER> --add-label "planned"
```

Move the item's Status to {config.workflow.statuses.planned} on the project board:
```bash
gh project item-edit --project-id "<PROJECT_NODE_ID>" --id "<ITEM_ID>" --field-id "<STATUS_FIELD_ID>" --single-select-option-id "<PLANNED_OPTION_ID>"
```

To get the `<ITEM_ID>`, search the project items fetched in Setup Step 3 for the item matching this issue number.

## Output Summary Format

The GitHub-specific completion message:

```
✅ Pianificazione completata!

📁 {config.paths.planning}/{US-CODE}.md
🔗 Issue: #NN — body aggiornato con link al piano e tasklist

📋 Sub-issues create: {N}
{list each: - #NNN TASK-XX: {title}}

📊 Riepilogo:
- User Story: {US-CODE}: {title}
- Task totali: {N} ({N} implementazione + {N} test)
- Label: `planned` ✅
- Sub-issues: {N} associate come sub-issue native
- Status nel project: {config.workflow.statuses.planned} ✅
```

## Technical Reference

### Parsing IDs Flow

1. `gh project list --owner "$OWNER" --format json` → project number + node ID
2. `gh project field-list $N --owner "$OWNER" --format json` → field IDs + option IDs
3. `gh project item-list $N --owner "$OWNER" --format json -L 200` → items with field values

### GraphQL Queries

The `gh project item-edit` command uses GraphQL internally. The key mutation is:

```graphql
mutation {
  updateProjectV2ItemFieldValue(input: {
    projectId: "<PROJECT_NODE_ID>"
    itemId: "<ITEM_ID>"
    fieldId: "<STATUS_FIELD_ID>"
    value: { singleSelectOptionId: "<PLANNED_OPTION_ID>" }
  }) {
    projectV2Item { id }
  }
}
```

### Item List Limits

Always use `-L 200` with `gh project item-list` to avoid the default limit of 30 items.

### Status Transitions

| From | To | Trigger |
|---|---|---|
| {config.workflow.statuses.todo} | {config.workflow.statuses.planned} | This skill (after planning completes) |
| {config.workflow.statuses.planned} | {config.workflow.statuses.in_progress} | Implementation skill |
| {config.workflow.statuses.in_progress} | {config.workflow.statuses.review} | Implementation skill (after code review) |
| {config.workflow.statuses.review} | {config.workflow.statuses.done} | Human reviewer — not automated |
