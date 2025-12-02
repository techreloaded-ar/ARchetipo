# Backlog Format Guide

> **Scope:** This guide defines the OUTPUT FORMAT ONLY for backlog artifacts.
> For INVEST principles and story quality standards, see `user-story-best-practices.md`
> For agent workflows and operational guidance, see agent definitions

This document defines the format and conventions for the project backlog, using a 2-level structure: a main index and separate files for each story.

**Templates:**
- Initialize index from `.opencode/templates/backlog.md`
- Initialize story files from `.opencode/templates/story-template.md`

## File Structure

```
docs/
├── backlog.md                    # Main index with overview
└── stories/
    ├── US-001-description.md     # Individual story files
    ├── US-002-description.md
    └── ...
```

## Index Format: `docs/backlog.md`

### Template Structure

```markdown
# Product Backlog

## 📦 EP-XXX: Epic Name

Brief description of epic and strategic objective.

**Stories:**
- [ ] [US-XXX](stories/US-XXX-slug.md) - Brief title | **PRIORITY** | Xpt
- [~] [US-YYY](stories/US-YYY-slug.md) - Brief title | **PRIORITY** | Xpt
- [x] [US-ZZZ](stories/US-ZZZ-slug.md) - Brief title | **PRIORITY** | Xpt ✅ YYYY-MM-DD

---

**Legend:**
- `[ ]` = TODO
- `[~]` = IN PROGRESS
- `[x]` = DONE
- `[!]` = BLOCKED
```

### Index Conventions

**Epic:**
- Format: `## 📦 EP-XXX: Epic Name`
- Numeric ID with 3-digit zero-padding: EP-001, EP-002, ..., EP-999
- Descriptive and concise name

**Story in list:**
- Markdown checkbox for state: `[ ]`, `[~]`, `[x]`, `[!]`
- Relative link: `[US-XXX](stories/US-XXX-slug.md)`
- Brief title (max 60 characters)
- Inline metadata separated by `|`:
  - **Priority:** HIGH | MEDIUM | LOW
  - Estimate: story points (1-8)
- Optional completion date: `✅ YYYY-MM-DD`

**Example:**
```markdown
- [~] [US-001](stories/US-001-book-data-entry.md) - Book data entry | **HIGH** | 3pt
```

## Story Format: `docs/stories/US-XXX-slug.md`

### Complete Template

```markdown
# US-XXX: User Story Title

**Epic:** EP-XXX | **Priority:** HIGH/MEDIUM/LOW | **Estimate:** Xpt | **Status:** TODO/IN PROGRESS/DONE/BLOCKED

## User Story

As [role/persona]
I want [functionality/action]
So that [benefit/value]

## Acceptance Criteria

- ✓ [Criterion 1: happy path scenario]
- ✓ [Criterion 2: error handling]
- ✓ [Criterion 3: edge case]

## Dev Notes

**YYYY-MM-DD** - Event/Task completed
- Implementation note
- Technical decision
- Issue encountered
```

### Story Conventions

**Header Metadata:**
- **Epic:** Parent epic ID (e.g., EP-001)
- **Priority:** HIGH | MEDIUM | LOW
- **Estimate:** Story points (1-8, Fibonacci scale)
- **Status:** TODO | IN PROGRESS | DONE | BLOCKED

**User Story:**
- Structure: As [role] I want [action] So that [value]

**Acceptance Criteria:**
- Minimum 3 scenarios:
  1. Happy path (normal case)
  2. Error handling/validation
  3. Edge case/boundary condition
- Format: bullet list with `✓` prefix
- Describe expected behavior, not implementation

**Dev Notes:**
- Optional section, added during development
- Entry per date: `**YYYY-MM-DD** - Context`
- Content: technical decisions, issues, workarounds, test notes

**File Naming:**
- Pattern: `US-XXX-slug-description.md`
- XXX: 3-digit zero-padded number
- slug: keywords separated by dash (kebab-case)
- Example: `US-001-book-data-entry.md`

## States and Workflow

### Story States

**In backlog.md (checkbox):**
- `[ ]` **TODO** - Story in backlog, not started
- `[~]` **IN PROGRESS** - Story development started
- `[x]` **DONE** - Story completed, acceptance criteria verified
- `[!]` **BLOCKED** - Blocked by external dependencies

**In story file (Status field):**
- **TODO** - Not started
- **IN PROGRESS** - In development
- **DONE** - Completed
- **BLOCKED** - Blocked

### Workflow Transitions

**Story:**
```
TODO → IN PROGRESS → DONE
  ↓         ↓
BLOCKED ←────┘
```

## Complete Examples

See implementation plan for complete examples of:
- Index backlog.md with multiple epics
- Story file US-001 during development
- State transition workflow
- Git commit strategy
