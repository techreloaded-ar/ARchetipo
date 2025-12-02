---
name: implement-story
agent: developer-agent
subtask: true
---

@.opencode/context/requirements/backlog-format-guide.md
@.opencode/context/development/coding-standards.md

**Implementation Request:** $ARGUMENTS


## Implementation Workflow

### Phase 1: Initial Alignment

#### 1. Confirm the story with the user

- Read `docs/backlog.md` to identify TODO stories (`- [ ] [US-XXX](stories/US-XXX-slug.md)`).
- If the user already specified the ID, pull the title from the backlog and ask for a quick confirmation.
- If no story was specified, propose the first TODO story showing title and priority, then ask explicitly: "Should I proceed with US-XXX? (y/n)".
- If no TODO items exist, respond with "No TODO stories found. All done! 🎉" and exit.

#### 2. Load and validate the story file

- Open `docs/stories/US-XXX-slug.md` and gather:
  - User Story
  - Acceptance Criteria (GHERKIN)
  - Architecture Notes
  - Tasks (`- [ ] TK-XXX: ...`)
- If the story has no tasks, stop and ask the user how to proceed (do not create tasks automatically).

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

- In Step-by-step mode: after Phase 5 (test outcome), ask before moving to next task
- When you need clarification or encounter a blocker
- When explicitly asking for approval (e.g., git operations, out-of-scope changes)

#### Step 1: Mark the task as "in progress" and announce it

- Track the status locally (`taskProgress[TK-XXX] = "in-progress"`) without editing the markdown yet.
- Tell the user: `🔨 Starting TK-XXX · [short description]`.
- **IMPORTANT:** In YOLO mode Immediately proceed to Step 2 (Analysis) without waiting for user confirmation. The announcement is informational only.
- In Step-by-step mode, the pause happens AFTER task completion (Phase 5), not before starting.

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

### Phase 3: Git Report (no automatic commits)

- After each finished task run `git status -sb` and show the short list of touched files.
- When helpful, add `git diff --stat` for context, but avoid dumping full diffs.
- Store these two outputs (status + stat) in a local object `pendingChangesSummary` so they can be reused later.
- Suggest a Conventional Commit-style message (e.g., `feat(US-XXX): ...`) and ask for confirmation before doing anything.
- If the user declines the commit, keep `pendingChangesSummary` intact; if the user approves, clear it once the commit succeeds.
- Execute Git commands only after the user explicitly tells you to.

---

### Phase 4: Testing

#### Step 1: Decide whether to involve the tester

1. Present a recap: story, completed tasks, touched files, intended test command.
2. Mention whether a commit was created. If not, append the stored `pendingChangesSummary` (status + diff stat) so the tester sees local changes.
3. Ask: "Should I call @tester-agent to generate/update the tests? (y/n)".
4. If yes:
   - Build a detailed prompt for `/write-tests` that includes story title, relevant acceptance criteria, modified files, any utilities to mock, and—when no commit exists—the `pendingChangesSummary` so the tester knows what is pending.
   - Run `/write-tests "...context..."` and wait for it to finish.
5. If no, note that tests will be provided by the user or are already up to date.

#### Step 2: Run the test suite

- Identify the correct command (from package.json or the docs). If unclear, ask the tester.
- Execute the tests and keep a concise output plus the full log path (if long).
- When multiple suites exist, start from the one closest to the changes and broaden coverage only if required.

### Phase 5: Test Outcome Handling

#### Case A: Tests PASS ✅

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
   🧪 Tests: command XYZ → PASS
   ```

4. In Step-by-step mode ask if you should continue; otherwise move on automatically.

---

#### Case B: Tests FAIL ❌ (first attempt)

**Actions:**

1. Keep `taskProgress[TK-XXX] = "in-progress"` (do not update the story file until a decision is made).
2. Record a Dev Notes block (single write) containing:

   ```markdown
   ### TK-XXX · YYYY-MM-DD

   **Status:** ❌ Tests failing
   **Implemented:**
   - Summary from `localTaskNotes`
   **Files:**
   - path/to/file1.ext (created/modified)
   **Test output (excerpt):**
   [main error details]
   ```

3. Summarize for the user:

   ```text
   ❌ TK-XXX: tests failing (command XYZ)
   Error: [short description]

   How should we proceed?
   1. Try another approach (describe it)
   2. User will handle it / mark for review
   3. Mark the task as ⚠️ blocked
   ```

4. Apply the chosen path:
   - **1. New attempt:** collect the additional guidance, implement it, rerun the tests.
   - **2. Manual fix:** leave the task as in-progress and explain how to resume later.
   - **3. Blocked:** update the story file with `- [!] TK-XXX: ... ⚠️ Blocked - reason` and log it in Dev Notes.

---

### Phase 6: Story Completion

**Trigger:** All entries in `taskProgress` are `done`.

#### Actions

1. Ask the user for final confirmation before touching tracking files.
2. Update the story front matter (`Status: TODO/IN PROGRESS → DONE`) and the backlog entry (`- [ ]` → `- [x]`) with a single write per file.
3. Suggest the commit message `chore(US-XXX): mark story as done in backlog`, but keep it as a recommendation only (no `git commit`).
4. Provide a closing summary:

   ```text
   ✅ Story US-XXX completed
   📊 Tasks: X / X
   📁 Key files: ...
   📌 Next steps: (potential follow-ups)
   ```

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

#### 4. Test Framework Not Detected
```
⚠️ I couldn't auto-detect the test command for this project.

Searched in:
- package.json, pom.xml, build.gradle, Cargo.toml, go.mod, etc.
- README.md, CONTRIBUTING.md, Makefile

Please specify how to run tests:
Examples: "npm test", "pytest", "gradle test", "make test"

Test command:
```

#### 5. Git Operation Failed
```
❌ Git operation failed: <error message>

Please resolve this manually and then:
- Continue: /implement-story US-XXX (will resume from where it stopped)
- Or fix the git issue and retry
```

#### 6. Branch Already Exists
```
⚠️ Branch feature/US-XXX-slug already exists

Options:
1. Use existing branch (y)
2. Specify different branch name (n)

Your choice:
```

#### 7. File Write Error
```
❌ Couldn't update file <path>: <error>

Retrying once...

<If retry fails>
❌ File write failed after retry. Please check file permissions.
```
