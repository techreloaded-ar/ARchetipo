# Connector: GitHub Projects

Load this connector only when `.airchetipo/config.yaml` has `backend: github`.

This connector overrides the backlog flow's setup and write-output phases while keeping epic decomposition, story generation, prioritization, and quality rules unchanged.

## Setup

### Step 1 - Auth and Project Discovery

Detect repository owner and repository name:

```bash
gh repo view --json owner,name --jq '{owner: .owner.login, name: .name}'
```

Save:
- `$OWNER`
- `$REPO_NAME`

Then discover available projects:

```bash
gh project list --owner "$OWNER" --format json
```

If this fails because of missing scopes, stop and show:

```text
Non ho i permessi necessari per accedere ai GitHub Projects.

Esegui:
gh auth refresh -s read:project -s project

Poi rilancia airchetipo-inception chiedendo di generare il backlog dal PRD.
```

Look for a project whose title contains `Backlog`.
- If found, ask the user whether to use it
- If not found, create one:

```bash
gh project create --owner "$OWNER" --title "$REPO_NAME Backlog"
```

Save the project number as `$PROJECT_NUMBER`.

### Step 2 - Custom Fields and Status Setup

Read existing fields once:

```bash
gh project field-list $PROJECT_NUMBER --owner "$OWNER" --format json
```

Extract:
- `$PROJECT_NODE_ID`
- `$STATUS_FIELD_ID`
- status option IDs
- `$PRIORITY_FIELD_ID`
- priority option IDs
- `$SP_FIELD_ID`

Create missing fields when needed:

```bash
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Priority" --data-type "SINGLE_SELECT" --single-select-options "HIGH,MEDIUM,LOW"
gh project field-create $PROJECT_NUMBER --owner "$OWNER" --name "Story Points" --data-type "NUMBER"
```

Create the `Epic` field after epics are known.

### Step 3 - Status Options

Ensure the workflow statuses from config exist:

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

## Startup Message

This startup message is mandatory and must be sent before any GitHub setup, issue creation, or final summary.
Do not collapse it into the final confirmation.

Use this startup variant:

```text
AIRCHETIPO - BACKLOG GENERATION (GitHub Projects)

Emanuele e Andrea sono pronti a decomporre il PRD in un backlog prioritizzato.

PRD trovato: [file path]
GitHub Project: [project title] (#N)
Owner: [owner]

Analisi dei requisiti in corso...
```

If the project was just created, also explain how to filter the board to hide sub-issues if needed.

## Write Output

### Step 1 - Idempotency Check

Search for existing backlog issues:

```bash
gh issue list --label "airchetipo-backlog" --state all --json number,title --limit 200
```

If issues already exist, ask whether to:
- skip existing
- recreate
- abort

### Step 2 - Create Labels

Create the shared backlog label and one label per epic in a single shell call.

### Step 3 - Create Epic Field

Create or update the `Epic` single-select field after the epic list is finalized.

### Step 4 - Create Issues

Create one GitHub issue per story.

Each issue body must contain:
- story
- demonstrates
- acceptance criteria
- epic
- priority
- story points
- blocked by
- scope

Use the `airchetipo-backlog` label plus the epic label.

### Step 4b - Backfill Dependencies

After all issues are created, replace story-code dependencies with actual GitHub issue references for stories that have blockers.

### Step 4c - Collect Node IDs

Fetch node IDs in one GraphQL query for the created issues.

### Step 5 - Add to Project and Set Fields

1. Add all issues to the project via a batched GraphQL mutation
2. Then set all required fields in a second batched GraphQL mutation:
   - Status
   - Priority
   - Story Points
   - Epic

Split the field-update mutation into chunks only if the backlog is very large.

## Final Output Summary

After GitHub output is complete, show:

```text
Backlog generated successfully on GitHub Projects.

Project: [project URL]

Summary:
- Epics: N
- User Stories (Issues): N
- Total Story Points: N
- HIGH priority: N stories
- MEDIUM priority: N stories
- LOW priority: N stories
```

Then list the created issues concisely.
