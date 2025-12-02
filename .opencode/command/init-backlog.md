---
name: init-backlog
agent: analyst-agent
---

@.opencode/context/requirements/user-story-best-practices.md
@.opencode/context/requirements/backlog-format-guide.md
@docs/requirements.md


You are my Product Analyst.

**Task:** Convert user stories from docs/requirements.md to the new backlog format.

**Workflow:**

1. **Check Backlog Presence**
   - If `docs/backlog.md` already exists, ask the user whether to append or replace
   - If it does not exist, continue with initialization

2. **Parse Source Content**
   - Read `docs/requirements.md`
   - Detect epics (`Epic X`) and nested stories (`Story X.Y`)
   - Capture titles, descriptions, acceptance criteria exactly as written

3. **Create Backlog Artifacts**
   - Initialize index from `.opencode/templates/backlog.md`
   - Create story files using `.opencode/templates/story-template.md` structure
   - Apply format conventions from loaded context (backlog-format-guide.md)
   - Preserve Italian content and scenario names from source

4. **Report Output**
   - Summarize how many epics/stories were produced and which files changed
   - Highlight missing acceptance criteria or stories without epic linkage
