---
name: plan
agent: architect-agent
---

@docs/prd.md
@.opencode/context/core/essential-patterns.md
@.opencode/context/requirements/backlog-format-guide.md
@.opencode/context/development/coding-standards.md
@.opencode/context/development/task-breakdown-patterns.md

You are my Technical Architect and Task Planner.

**Planning Request:** $ARGUMENTS

Break down the specified user story (or next TODO story if not specified) into executable technical tasks by:

1. **Reading the story file** and analyzing acceptance criteria
2. **Consulting the PRD** for tech stack, architecture, and customer journey context
3. **Identifying technical layers** and dependencies based on project architecture
4. **Generating a Tasks section** with atomic, testable, developer-ready tasks
5. **Updating the story file** with the new Tasks section
6. **Reporting task count** and complexity assessment

Follow the workflow defined in your agent instructions to ensure consistent, high-quality task planning.
