# Backend: GitHub Projects v2

> This file is loaded when `.airchetipo/config.yaml` has `backend: github`.
> It overrides the I/O phases of the plan skill while keeping domain logic identical.

> **PERFORMANCE:** All GitHub operations in this file (issue creation, sub-issue linking, board cleanup) must be batched. Use a **single Bash tool call per step** with a loop script. Never issue one tool call per sub-issue — this is the single biggest performance optimization.

## Setup

### Step 1 — Auth check & owner detection

Run these two commands in **parallel tool calls**:

```bash
gh repo view --json owner --jq '.owner.login'
```

```bash
gh repo view --json name --jq '.name'
```

Save as `$OWNER` and `$REPO`.

Then test GitHub Projects auth:
```bash
gh project list --owner "$OWNER" --limit 1 --format json
```
If this fails with a scope/permission error, tell the user to run `gh auth refresh -s read:project -s project` and stop.

### Step 2 — Project discovery & data fetch

1. Find the Backlog project:
   ```bash
   gh project list --owner "$OWNER" --format json
   ```
   Look for a project whose title contains "Backlog". If not found, stop.

2. Save `$PROJECT_NUMBER` and fetch field metadata + items in **parallel tool calls**:

   ```bash
   gh project field-list $PROJECT_NUMBER --owner "$OWNER" --format json
   ```

   ```bash
   gh project item-list $PROJECT_NUMBER --owner "$OWNER" --format json -L 200
   ```

   Save field IDs and option IDs (Status options matching `{config.workflow.statuses}`: todo, planned, in_progress, review, done; plus Priority, Story Points, Epic).

3. Filter items to: Status == {config.workflow.statuses.todo} AND does NOT have label `planned`. If no eligible items, inform user and stop.

## Read Backlog (Story Source)

### Story selection from GitHub Project

1. **If a story code was passed as argument** (e.g., "US-005"):
   - Search for it among the filtered items by title prefix
   - If not found, list available stories

2. **If NO argument was passed:**
   - Among eligible items, select highest Priority (HIGH > MEDIUM > LOW)
   - Among equal priorities, select the lowest story number

3. Read the full issue body:
   ```bash
   gh issue view <NUMBER> --json body,title,labels,number,url
   ```

> **Note:** Free-text story creation is not supported with the GitHub backend. If the argument is not a US-XXX code, inform the user to create the issue on GitHub first.

## Write Output

After saving the planning document in `{config.paths.planning}/{US-CODE}.md`:

### Step 1 — Detect epic label

Read the labels from the parent issue (fetched during story selection). Identify the epic label matching the pattern `EP-XXX`. Save it as `$EPIC_LABEL`.

### Step 2 — Create ALL sub-issues in a single Bash call

Create all sub-issues in **one Bash tool call** using a loop. Do NOT create them in separate tool calls.

Use this compact sub-issue body template:

```
**Parent:** #{PARENT_NUMBER} — {US-CODE}: {Story Title}
**Tipo:** {Impl/Test} | **Dipendenze:** {deps}

{DESCRIPTION — 2-3 sentences}

**Completamento:**
- [ ] {criterion 1}
- [ ] {criterion 2}

_Sub-issue da AIRchetipo Planning_
```

The bash script pattern:

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
{sub-issue body}
EOF
)")
NUMS+=($(echo "$URL" | grep -o '[0-9]*$'))

# TASK-02
URL=$(gh issue create --repo "$OWNER/$REPO" \
  --title "TASK-02: {Title}" \
  --label "$LABEL" \
  --body "$(cat <<'EOF'
{sub-issue body}
EOF
)")
NUMS+=($(echo "$URL" | grep -o '[0-9]*$'))

# ... repeat for all tasks

echo "Created issues: ${NUMS[*]}"
```

**Important:** Create sub-issues in TASK order (TASK-01 first, then TASK-02, etc.) to maintain logical ordering.

### Step 3 — Link sub-issues + cleanup in a single Bash call

After creating all sub-issues, link them to the parent AND remove from project board in **one Bash tool call**:

```bash
OWNER="..."
REPO="..."
PARENT=N
PROJECT_NUMBER=N

for CHILD_NUMBER in ${NUMS[*]}; do
  # Link as native sub-issue
  CHILD_ID=$(gh api /repos/$OWNER/$REPO/issues/$CHILD_NUMBER --jq '.id')
  gh api -X POST /repos/$OWNER/$REPO/issues/$PARENT/sub_issues \
    -F "sub_issue_id=$CHILD_ID" \
    -H "X-GitHub-Api-Version: 2026-03-10"

  # Remove from project board if auto-added
  ITEM_ID=$(gh project item-list $PROJECT_NUMBER --owner "$OWNER" --format json -L 200 \
    --jq ".items[] | select(.content.number == $CHILD_NUMBER) | .id")
  if [ -n "$ITEM_ID" ]; then
    gh project item-delete $PROJECT_NUMBER --owner "$OWNER" --id "$ITEM_ID"
  fi
done
```

### Step 4 — Update parent issue + label + status in parallel

Run these in **parallel tool calls**:

**Call 1:** Update parent issue body + add label
```bash
CURRENT_BODY=$(gh issue view <NUMBER> --repo "$OWNER/$REPO" --json body --jq '.body')

gh issue edit <NUMBER> --repo "$OWNER/$REPO" --body "$(cat <<BODYEOF
${CURRENT_BODY}

---

## 📋 Piano di Implementazione

**File:** \`{config.paths.planning}/{US-CODE}.md\`

**Riepilogo:**
- Task totali: {N} ({N} implementazione + {N} test)

_Generato da AIRchetipo Planning Team_
BODYEOF
)"

gh label create "planned" --repo "$OWNER/$REPO" --description "Story has an implementation plan" --color "0E8A16" --force 2>/dev/null
gh issue edit <NUMBER> --repo "$OWNER/$REPO" --add-label "planned"
```

**Call 2:** Move status to PLANNED on project board
```bash
gh project item-edit --project-id "<PROJECT_NODE_ID>" --id "<ITEM_ID>" --field-id "<STATUS_FIELD_ID>" --single-select-option-id "<PLANNED_OPTION_ID>"
```

To get `<ITEM_ID>`, search the project items fetched in Setup Step 2 for the item matching this issue number.

## Output Summary Format

The GitHub-specific completion message:

```
✅ Pianificazione completata!

📁 {config.paths.planning}/{US-CODE}.md
🔗 Issue: #NN — body aggiornato con piano e sub-issues

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

> **Note:** `gh project item-list --format json` may return JSON with unescaped control characters in `content.body` that break external `jq`. Always use `gh`'s built-in `--jq` flag instead of piping to the system `jq` binary.

### Parsing IDs Flow

1. `gh project list --owner "$OWNER" --format json` → project number + node ID
2. `gh project field-list $N --owner "$OWNER" --format json` → field IDs + option IDs
3. `gh project item-list $N --owner "$OWNER" --format json -L 200` → items with field values

### Item List Limits

Always use `-L 200` with `gh project item-list` to avoid the default limit of 30 items.

### Status Transitions

| From | To | Trigger |
|---|---|---|
| {config.workflow.statuses.todo} | {config.workflow.statuses.planned} | This skill (after planning completes) |
| {config.workflow.statuses.planned} | {config.workflow.statuses.in_progress} | Implementation skill |
| {config.workflow.statuses.in_progress} | {config.workflow.statuses.review} | Implementation skill (after code review) |
| {config.workflow.statuses.review} | {config.workflow.statuses.done} | Human reviewer — not automated |
