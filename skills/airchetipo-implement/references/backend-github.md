# Backend: GitHub Projects v2

> This file is loaded when `.airchetipo/config.yaml` has `backend: github`.
> It overrides the I/O phases of the implement skill while keeping domain logic identical.

---

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
🔧 **Ugo:** Non ho i permessi necessari per accedere ai GitHub Projects.

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
🔧 **Ugo:** Non trovo un GitHub Project con "Backlog" nel titolo.

Esegui prima `airchetipo-inception` chiedendo di generare il backlog dal PRD su GitHub Projects.
```

3. Save `$PROJECT_NUMBER` and fetch field metadata:
   ```bash
   gh project field-list $PROJECT_NUMBER --owner "$OWNER" --format json
   ```
   Save field IDs and option IDs (Status options matching `{config.workflow.statuses}`: todo, planned, in_progress, review, done; plus Priority, Story Points, Epic).

---

## Read Backlog (Story Source)

> **Important:** With `backend: github`, GitHub is the **single source of truth**. The implementation plan lives in the parent issue body (strategic plan) and its sub-issues (executable tasks). No local planning file is required.

### Step 3 — Fetch and filter items

1. Fetch all items:
   ```bash
   gh project item-list $PROJECT_NUMBER --owner "$OWNER" --format json -L 200
   ```

2. Filter to items where:
   - Status == {config.workflow.statuses.planned}

3. If no eligible items found, inform the user and **stop**:
```
🔧 **Ugo:** Non ci sono story pronte per l'implementazione.

Per essere implementabile, una story deve essere in stato "{config.workflow.statuses.planned}" nel project.

Puoi:
- Eseguire `/airchetipo-plan` per pianificare una story
- Specificare una story diversa come argomento
```

### Step 4 — Story selection

1. **If a story code was passed as argument** (e.g., "US-005"):
   - Search for it among the filtered items by title prefix
   - If not found in eligible items, check if it exists at all and explain why it's not eligible

2. **If NO argument was passed:**
   - Among eligible items, select the one with highest Priority (HIGH > MEDIUM > LOW)
   - Among equal priorities, select the lowest story number

3. Read the full issue body:
   ```bash
   gh issue view <NUMBER> --json body,title,labels,number,url
   ```

### Step 4b — Load implementation plan from GitHub

The implementation plan is split across two GitHub sources. Read both:

1. **Strategic plan from parent issue body:** The body (fetched in Step 4.3) contains sections "Soluzione Tecnica" and "Strategia di Test" under `## 📋 Piano di Implementazione`. Parse these sections to understand the technical approach and test strategy.

2. **Task list from sub-issues:**
   ```bash
   gh api /repos/$OWNER/$REPO/issues/$PARENT_NUMBER/sub_issues \
     -H "X-GitHub-Api-Version: 2026-03-10"
   ```
   This returns all sub-issues with their full body in a single API call.

3. **For each open sub-issue**, parse:
   - **Task ID and title:** From issue title (e.g., `TASK-01: Setup data model`)
   - **Type:** From body field `**Tipo:** Impl` or `**Tipo:** Test`
   - **Dependencies:** From body field `**Dipendenze:** TASK-01, TASK-03` (or `-` if none)
   - **Description:** The prose section in the body
   - **Completion criteria:** The `**Completamento:**` checklist items

4. **Build the ordered task list** sorted by TASK-XX number, with dependency graph from the parsed dependencies.

5. **Validation:** If a sub-issue has unexpected format (missing "Tipo", malformed "Dipendenze"), log a warning but continue with reasonable defaults:
   - Missing Tipo → default to `Impl`
   - Missing/malformed Dipendenze → default to no dependencies
   - Missing TASK-XX in title → assign next available number

6. **If no open sub-issues found**, inform the user and **stop**:
```
🔧 **Ugo:** L'issue #{PARENT_NUMBER} non ha sub-issues aperte.

La story è in stato PLANNED ma non ha task da implementare. Puoi:
- Eseguire `/airchetipo-plan {US-CODE}` per ri-pianificare
- Creare manualmente le sub-issues su GitHub
```

### Step 4c — Load mockup references

After loading the implementation plan from the parent issue body:

1. Scan the plan body for a mockup section (look for `### Mockup` or a line containing `🎨` and a path to `{config.paths.mockups}/`)
2. If found, extract the mockup directory path (e.g., `{config.paths.mockups}/{US-CODE}/`)
3. Check if the directory exists locally and list its contents
4. If mockup files are found, record their paths — they become **mandatory references** for any UI implementation task (same rules as SKILL.md Phase 0 Step 6)
5. If the directory does not exist or is empty, do NOT block — the mockup may still be generating

---

## Status: Move to {config.workflow.statuses.in_progress}

### Step 5 — Move to {config.workflow.statuses.in_progress}

Update the item's Status to {config.workflow.statuses.in_progress}:
```bash
gh project item-edit --project-id "<PROJECT_NODE_ID>" --id "<ITEM_ID>" --field-id "<STATUS_FIELD_ID>" --single-select-option-id "<IN_PROGRESS_OPTION_ID>"
```

The session announcement should include the issue reference:

```
**Issue:** #NN — spostata a "{config.workflow.statuses.in_progress}" ✅
```

---

## Write Output (Completion)

After code review passes (end of Phase 5):

### 1. Run the full test suite

One final time to confirm everything works.

### 2. Close all sub-issues

Close all native sub-issues of the parent story:

```bash
# List the sub-issues of the parent
SUB_ISSUES=$(gh api /repos/$OWNER/$REPO/issues/$PARENT_NUMBER/sub_issues \
  -H "X-GitHub-Api-Version: 2026-03-10" --jq '.[].number')

# Close each sub-issue
for ISSUE_NUM in $SUB_ISSUES; do
  gh issue close $ISSUE_NUM
done
```

Save the count of closed sub-issues for the output summary.

### 3. Move to {config.workflow.statuses.review} on the project board

```bash
gh project item-edit --project-id "<PROJECT_NODE_ID>" --id "<ITEM_ID>" --field-id "<STATUS_FIELD_ID>" --single-select-option-id "<REVIEW_OPTION_ID>"
```

### 4. Post a summary comment on the issue

```bash
gh issue comment <NUMBER> --body "$(cat <<'EOF'
## ⚡ Implementazione Completata

**Stato:** {config.workflow.statuses.review}

**Riepilogo:**
- Task completati: {N}/{N}
- Sub-issues chiuse: {N}/{N}
- Test scritti: {N}
- Code review: superata ✅
- Cicli di review: {N}

**File creati/modificati:**
- `path/to/new-file.ts` (nuovo)
- `path/to/modified-file.ts` (modificato)
- `path/to/test-file.test.ts` (nuovo)

_Implementato da AIRchetipo Implementation Team_
EOF
)"
```

### 5. Update labels

```bash
gh label create "in-review" --description "Implementation complete, awaiting human review" --color "D93F0B" --force
gh issue edit <NUMBER> --remove-label "planned" --add-label "in-review"
```

---

## Output Summary Format

The GitHub-specific completion message replaces the file-based one:

```
✅ Implementazione completata!

**User Story:** {US-CODE}: {title}
**Issue:** #NN
**Stato nel project:** {config.workflow.statuses.review} 🟣

**Riepilogo implementazione:**
- Task completati: {N}/{N}
- Sub-issues chiuse: {N} ✅
- Test scritti: {N}
- Code review: superata ✅
- Cicli di review: {N}

**File creati/modificati:**
- `path/to/new-file.ts` (nuovo)
- `path/to/modified-file.ts` (modificato)
- `path/to/test-file.test.ts` (nuovo)

⚠️ **Story in {config.workflow.statuses.review}.** Un reviewer umano deve verificare prima di spostarla in {config.workflow.statuses.done}.
```

---

## Technical Reference

### Parsing IDs Flow

All `item-edit` commands require node IDs. The flow is:

1. `gh project list --owner "$OWNER" --format json` → project number + node ID
2. `gh project field-list $N --owner "$OWNER" --format json` → field IDs + option IDs
3. `gh project item-list $N --owner "$OWNER" --format json -L 200` → items with field values

Always use `--format json` to get machine-parseable output.

### Item List Limit

Always use `-L 200` with `gh project item-list` to avoid the default limit of 30 items.

### Status Transitions

| From | To | When |
|---|---|---|
| {config.workflow.statuses.planned} | {config.workflow.statuses.in_progress} | Implementation starts (Setup, Step 5) |
| {config.workflow.statuses.in_progress} | {config.workflow.statuses.review} | Implementation completes (Write Output, Step 2) |
| {config.workflow.statuses.review} | {config.workflow.statuses.done} | **Human reviewer** — not automated |
