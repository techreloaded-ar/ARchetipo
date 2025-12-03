---
name: implement-story
agent: developer-agent
---

@.opencode/context/requirements/backlog-format-guide.md
@.opencode/context/development/coding-standards.md

**Implementation Request:** $ARGUMENTS


## Implementation Workflow

### Phase 1: Initial Alignment

### Phase 1: Initialization

#### 1. Parse Backlog and Select Story

**Read backlog structure:**
- Read `docs/backlog.md` to identify epics and stories
- Look for PLANNED stories with checkbox `[P]` - queste hanno task pronti per lo sviluppo

**Story Selection:**
- **If user provided story ID** (e.g., "US-005"): Use that story regardless of status
- **If no story specified**: Auto-select first PLANNED story (checkbox `[P]`) in backlog
- **If no PLANNED stories**: Report "Nessuna storia PLANNED trovata. Le storie devono avere task prima dell'implementazione. Usa `/plan-story` per generare i task." and exit

#### 2. Read and Validate Story File

**Read story file:**
- Parse `docs/stories/US-XXX-slug.md`
- Extract sections:
  - User Story (As a... I want... So that...)
  - Acceptance Criteria (GHERKIN scenarios)
  - Architecture Notes (technical guidance)
  - Tasks list (implementation tasks)

**Validate story:**
- Check Status field in story file metadata
- **If Status is TODO:**
  - Report error: "❌ Storia US-XXX ha Status=TODO e non è stata pianificata. Esegui `/plan-story US-XXX` prima per generare i task e aggiornare lo status a PLANNED."
  - Exit
- **If Status is PLANNED or IN PROGRESS:** Proceed normally
- **If Status is DONE:**
  - Report error: "❌ Storia US-XXX è già DONE. Niente da implementare."
  - Exit
- **If Status is BLOCKED:**
  - Report warning: "⚠️ Storia US-XXX è BLOCKED. Procedere comunque? (y/n)"
  - Wait for user confirmation
- Ensure story has at least one task in Tasks section

#### 3. Set the execution mode

Always ask how to proceed before starting implementation:

```
📋 US-XXX · [Story Title]
Total tasks: X

1. 🚀 YOLO: execute every task sequentially
2. 🐢 Step-by-step: stop after every task and wait for you

Choice (default 1):
```

- If the user does not answer within a reasonable timeout, stay in YOLO mode and post a reminder.
- In Step-by-step mode, ask "Move to the next task? (y/n)" once a task completes.

#### 4. Manage the branch only on request

- Suggest the name `feature/US-XXX-slug`, but ask: "Do you want me to create/switch to this branch? (y/n)".
- Only run `git checkout -b feature/US-XXX-slug` (or `git checkout feature/US-XXX-slug`) when the user explicitly says yes.
- If the answer is no, keep the current branch and continue.
- Never create commits automatically—just report the working tree status.

---

### Phase 2: Task Implementation Loop

**For each task in the story file:**

**CRITICAL:** Execute Steps 1-3 continuously without pausing for user input. The only pause points are:

- In Step-by-step mode: after task completion, ask before moving to next task
- When you need clarification or encounter a blocker
- When explicitly asking for approval (e.g., git operations, out-of-scope changes)

#### Step 1: Mark the task as "in progress" and announce it

- Track the status locally (`taskProgress[TK-XXX] = "in-progress"`) without editing the markdown yet.
- Tell the user: `🔨 Starting TK-XXX · [short description]`.
- **IMPORTANT:** In YOLO mode Immediately proceed to Step 2 (Analysis) without waiting for user confirmation. The announcement is informational only.
- In Step-by-step mode, the pause happens AFTER task completion (Phase 4), not before starting.

#### Step 2: Analyze the task requirements

**Minimum context to collect:**

- Task description → goal and deliverable.
- Architecture Notes → components, APIs, data models, required libraries.
- Acceptance Criteria → scenarios to cover (happy path + failure + edge cases).
- PRD/guides in the context → structural conventions, coding style, feature flags.

**Expected analysis output:**

- List of files to create/modify.
- Dependencies or scripts to touch.
- Questions/blocks to raise with the user (if any).

#### Step 3: Implement in traceable micro-steps

- Follow the architectural guidance and keep each change focused on a single goal.
- Update a local `localTaskNotes` structure describing what you are doing (later reused in Dev Notes).
- Keep the diff clean: no unsolicited refactors and no automatic commits.
- Surface any out-of-scope work and request approval before touching it.

---

### Phase 3: Automatic Commit After Each Task

// turbo
- After each finished task run `git add .` to stage all changes.
- Run `git status -sb` to show the short list of touched files.
- When helpful, add `git diff --stat --cached` for context, but avoid dumping full diffs.
- Automatically create a commit with a Conventional Commit-style message:
  - Format: `feat(US-XXX/TK-YYY): [task description]`
  - Example: `feat(US-005/TK-003): Add user authentication endpoint`
- Run `git commit -m "feat(US-XXX/TK-YYY): [task description]"`
- Report to the user what was committed.

---

### Phase 4: Task Completion

**Actions:**

1. Set `taskProgress[TK-XXX] = "done"` and prepare every markdown update in memory (checkbox + Dev Notes).
2. Write the story file once applying:
   - `- [ ]` → `- [x] TK-XXX: ... ✅ YYYY-MM-DD` (current date).
   - If `## Dev Notes` only has the placeholder, replace it with a real section; otherwise append:

     ```markdown
     ### TK-XXX · YYYY-MM-DD

     **Implemented:**
     - Bullet 1 from `localTaskNotes`
     - ...
     ```

3. Report to the user:

   ```text
   ✅ TK-XXX completed
   📁 Files touched: file1.ts, file2.ts
   ```

4. **Only In Step-by-step mode** ask if you should continue; otherwise **move on automatically**.

---

### Phase 5: Story Completion

**Trigger:** All entries in `taskProgress` are `done`.

#### Actions

1. Provide a closing summary:

   ```text
   🎉 Story US-XXX completed!
   📊 Tasks: X / X (all committed)
   📁 Total commits: X
   📌 Next steps: Run tests with `/write-tests US-XXX` or write acceptance tests
   ```

2. All changes have been committed incrementally (one commit per task).
3. The feature branch is ready for review or merging.

---

## Error Handling

### Error Categories and Responses

#### 1. Story Not Found
```
❌ Error: Story US-XXX not found in docs/stories/

Available TODO stories:
- US-001: Story title 1
- US-002: Story title 2
- US-005: Story title 5

Please specify which story to implement.
```

#### 2. No TODO Stories
```
🎉 No TODO stories found in backlog. All done!

If you need to implement a specific story, specify its ID:
/implement-story US-XXX
```

#### 3. Story Has No Tasks
```
❌ Error: Story US-XXX has no tasks defined

Please add tasks to the story before implementing.
You can edit: docs/stories/US-XXX-slug.md
```

#### 4. Git Operation Failed
```
❌ Git operation failed: <error message>

Please resolve this manually and then:
- Continue: /implement-story US-XXX (will resume from where it stopped)
- Or fix the git issue and retry
```

#### 5. Branch Already Exists
```
⚠️ Branch feature/US-XXX-slug already exists

Options:
1. Use existing branch (y)
2. Specify different branch name (n)

Your choice:
```

#### 6. File Write Error
```
❌ Couldn't update file <path>: <error>

Retrying once...

<If retry fails>
❌ File write failed after retry. Please check file permissions.
```
