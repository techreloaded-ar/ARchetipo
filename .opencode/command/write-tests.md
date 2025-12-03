---
name: write-tests
agent: tester-agent
---

@.opencode/context/requirements/backlog-format-guide.md
@.opencode/context/development/coding-standards.md

**Implementation Request:** $ARGUMENTS


## Implementation Workflow

### Phase 0: Awareness

**Auto-detect test framework:**

**1. Check project configuration files:**

| File | Framework | Command |
|------|-----------|---------|
| `package.json` with `scripts.test` | Node.js (npm/yarn/pnpm) | `npm test` or `yarn test` or `pnpm test` |
| `pom.xml` | Maven (Java) | `mvn test` |
| `build.gradle` / `build.gradle.kts` | Gradle (Java/Kotlin) | `gradle test` or `./gradlew test` |
| `go.mod` | Go | `go test ./...` |
| `pytest.ini` / `pyproject.toml` with pytest | Python (pytest) | `pytest` |
| `setup.py` / `tox.ini` | Python (unittest) | `python -m pytest` or `python -m unittest` |
| `Cargo.toml` | Rust | `cargo test` |
| `Gemfile` with rspec | Ruby (RSpec) | `bundle exec rspec` |
| `Gemfile` with minitest | Ruby (Minitest) | `rake test` |
| `composer.json` with phpunit | PHP (PHPUnit) | `./vendor/bin/phpunit` |

**2. Check documentation files:**
- Read `README.md` sections: "Running Tests", "Development", "Testing", "Getting Started"
- Read `CONTRIBUTING.md` for test instructions
- Read `Makefile` or `justfile` for test targets
- Look for commands like `make test`, `just test`

**3. Ask user if not detected:**
```
⚠️ I couldn't auto-detect the test command for this project.

Please specify how to run tests:
Examples: "npm test", "pytest", "gradle test", "go test ./...", "make test"

Test command:
```

**Save test command for future tasks** in this session.


### Phase 1: Initialization

#### 1. Determine Test Scope

**If a user story was specified as argument ($ARGUMENTS):**
- Open `docs/backlog.md` and locate the story described in $ARGUMENTS.
- Read the story's **Technical Notes** section carefully to understand which files need to be tested.
- Copy the acceptance criteria and definition of done into your working notes.
- Highlight any non-functional requirements (performance, accessibility, security) that might require additional tests.
- The files to test are those mentioned in the Technical Notes or those affected by the story implementation.

**If no argument was provided:**
- Run `git rev-parse --abbrev-ref HEAD` to get the current branch name.
- Determine the expected story branch pattern (for example `feature/<story-id>`) by checking if the current branch matches known patterns from `docs/backlog.md`.
- **If the current branch matches an expected story branch:**
  - Run `git diff main...HEAD` to get all changes introduced by this branch compared to main.
  - The files to test are those modified in this diff.
  - Try to infer the story context from the branch name and backlog to understand acceptance criteria.
- **If the current branch does NOT match a story branch:**
  - Run `git status --short` to identify pending (unstaged or staged) files.
  - Run `git diff HEAD` to get changes in tracked files.
  - The files to test are those with pending changes (modified, staged, or untracked).
  - Tests should cover the changes visible in these pending files.

#### 2. Decide Whether New Tests Are Needed
- Compare the identified scope (acceptance criteria if available, or code changes) with the current automated test coverage.
- If every criterion is already validated by existing tests and no regressions are possible, document the rationale, inform stakeholders that no new tests are required, and move directly to Phase 4 (Verification Loop).
- Otherwise, proceed with the remaining phases to design and implement the necessary tests.

### Phase 2: Change Review

#### 4. Load Developer-Agent Commits
- Pull the latest commits produced by the @developer-agent onto the current story branch (`git pull --ff-only` or an explicit `git fetch` + `git merge`/`git rebase`).
- If the pull introduces conflicts, resolve them immediately and document any unresolved files so the workflow can pause until the branch is clean.
- Confirm that the branch now contains every implementation change tied to the user story before continuing.

#### 5. Identify Test Surfaces
- Use `git status` and focused diffs to enumerate the files touched by the @developer-agent so you know which components require validation.
- Map each acceptance criterion to specific test types, files, or frameworks that must be updated.
- Plan whether you will extend existing specs or introduce new test files, preferring to augment current suites when practical.

### Phase 3: Test Authoring

#### 6. Design Test Cases
- For every acceptance criterion, outline at least one positive and one negative path, plus edge cases that correspond to risky inputs observed in the diff.
- Ensure tests clearly reflect user-facing behaviour rather than implementation details whenever possible.

#### 7. Implement or Update Tests
- Follow repository coding standards and place tests beside their respective components or in the prescribed test directories.
- Keep assertions specific and deterministic; mock only the dependencies necessary to isolate the behaviour introduced by the developer-agent.

#### 8. Self-Review the Test Suite
- Verify naming, structure, and fixture reuse comply with `.opencode/context/development/coding-standards.md`.
- Double-check that failures would produce actionable messages for the developer-agent.

### Phase 4: Verification Loop

#### 9. Execute Automated Tests
- Run the most targeted suite first (e.g., individual file, `npm run test:<suite>`), then escalate to the full command (`npm test`) once local checks succeed.

#### 10. Diagnose Failures and Iterate
- If tests fail due to mistakes in the newly written tests (incorrect expectations, flaky setup, missing fixtures), correct them immediately and rerun the affected suites.
- If failures indicate regressions in production code, summarize the failing scenarios, notify the @developer-agent, and wait for the implementation fix.
- After receiving new code, resync the branch (repeat Phase 1) and return to the start of Phase 4 until all tests pass.

### Phase 5: Finalization

#### 11. Confirm Completion
- When the entire automated suite passes, record the results along with any notes about skipped or unnecessary tests.
- Explicitly state that acceptance criteria are covered and reference the evidence (test files, command output) so reviewers can verify quickly.

#### 12. Commit and Handover
- Stage only the test files and supporting fixtures you created or modified, then craft a commit message describing which acceptance criteria are now enforced.
- Share the commit hash and testing summary with stakeholders, then mark the command as complete.

<!-- BOZZA WORKFLOW
- caricare le modifiche fatte dal developer-agent sul branch corrente

- scrivere i test per coprire i criteri di accettazione della storia, considerango le modifiche fatte dal developer-agent 

- se non ci sono test da scrivere, segnalare che non è necessario scrivere test e procedere alla verifica

- Lanciare i test per vedere se passano
- Se non passano, verificare il motivo:
  - Se il problema sono i test scritti male, intervenire automaticamente e correggere
  - Se il problema è il codice di produzione, chiamare il developer-agent e chiedere di implementare le modifiche necessarie
  - Tornare a lanciare i test per vedere se passano
- Se i test passano, segnalare che il task è completo
- Fare una commit con i test
- completare il comando

-->

