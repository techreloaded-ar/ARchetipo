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

4. Parse the `Blocked by` field from the issue body. If it contains issue references (e.g., `#NN (US-XXX)`), fetch those issue bodies in **parallel tool calls** to load blocking story context:
   ```bash
   gh issue view <BLOCKER_NUMBER> --json body,title,number,url
   ```
   If the `Blocked by` field is absent or `-`, treat the story as having no dependencies.

> **Note:** Free-text story creation is not supported with the GitHub backend. If the argument is not a US-XXX code, inform the user to create the issue on GitHub first.

## Write Output

> **Important:** With `backend: github`, GitHub is the **single source of truth** for the implementation plan. No local file is written in `{config.paths.planning}/`. The parent issue body contains the strategic plan (technical solution + test strategy), and sub-issues contain the executable task details. This is what `airchetipo-implement` reads.

### Step 1 — Detect epic label

Read the labels from the parent issue (fetched during story selection). Identify the epic label matching the pattern `EP-XXX`. Save it as `$EPIC_LABEL`.

### Step 2 — Update parent issue body with full plan

Write the **complete implementation plan** into the parent issue body. This replaces the original story body with the story content PLUS the plan. The parent issue body becomes the strategic reference that `airchetipo-implement` reads.

```bash
gh issue edit <NUMBER> --repo "$OWNER/$REPO" --body "$(cat <<'BODYEOF'
{ORIGINAL_STORY_BODY}

---

## 📋 Piano di Implementazione

**Generato da:** AIRchetipo Planning Team
**Data:** {DATE}

### Soluzione Tecnica

{FRASE_INTRODUTTIVA_APPROCCIO_E_MOTIVAZIONE}

- {PUNTO_CHIAVE_1}
- {PUNTO_CHIAVE_2}
- {PUNTO_CHIAVE_3}

### Strategia di Test

{FRASE_INTRODUTTIVA_STRATEGIA}

- {PUNTO_TEST_1}
- {PUNTO_TEST_2}
- {PUNTO_TEST_3}

### Riepilogo Task

- Task totali: {N} ({N} implementazione + {N} test)
- I task dettagliati sono nelle sub-issues associate

{IF_MOCKUP_GENERATED}
### Mockup

> 🎨 I mockup per questa storia sono disponibili in `{config.paths.mockups}/{US-CODE}/`
{/IF_MOCKUP_GENERATED}

_Generato da AIRchetipo Planning Team_
BODYEOF
)"
```

> Include the `### Mockup` section only if `mockup_generated = true`. Omit it entirely otherwise.

### Step 3 — Create ALL sub-issues in a single Bash call

Create all sub-issues in **one Bash tool call** using a loop. Do NOT create them in separate tool calls.

Sub-issues are the **executable task details** that `airchetipo-implement` reads and executes. Their body must be structured for machine parsing.

Use this sub-issue body template:

```
**Parent:** #{PARENT_NUMBER} — {US-CODE}: {Story Title}
**Tipo:** {Impl/Test} | **Dipendenze:** {TASK-XX, TASK-YY or "-" if none}

{DESCRIPTION — 2-5 sentences with enough technical context to implement the task}

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

### Step 4 — Link sub-issues to parent in a single Bash call

After creating all sub-issues, link them to the parent as native sub-issues in **one Bash tool call**:

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

> **Note:** Sub-issues do NOT appear on the project board because `airchetipo-backlog` disables all auto-add workflows during project setup (Step 2b). Only issues explicitly added via `addProjectV2ItemById` appear on the board.

### Step 5 — Add label + move status in parallel

Run these in **parallel tool calls**:

**Call 1:** Add label
```bash
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

🔗 Issue: #NN — piano completo nel body + sub-issues

📋 Sub-issues create: {N}
{list each: - #NNN TASK-XX: {title}}

📊 Riepilogo:
- User Story: {US-CODE}: {title}
- Task totali: {N} ({N} implementazione + {N} test)
- Label: `planned` ✅
- Sub-issues: {N} associate come sub-issue native
- Status nel project: {config.workflow.statuses.planned} ✅

📝 Per modificare il piano: edita il body dell'issue padre (strategia) o le singole sub-issues (task).
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
