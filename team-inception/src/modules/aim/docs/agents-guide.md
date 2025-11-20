# AIRchetipo Agents Guide

**Complete reference for all AIM agents, their roles, workflows, and collaboration**

**Reading Time:** ~45 minutes

---

## Table of Contents

- [Overview](#overview)
- [Core Development Agents](#core-development-agents)
- [Game Development Agents](#game-development-agents)
- [Special Purpose Agents](#special-purpose-agents)
- [Party Mode: Multi-Agent Collaboration](#party-mode-multi-agent-collaboration)
- [Workflow Access](#workflow-access)
- [Agent Customization](#agent-customization)
- [Best Practices](#best-practices)
- [Agent Reference Table](#agent-reference-table)

---

## Overview

The AIRchetipo Module (AIM) provides a comprehensive team of specialized AI agents that guide you through the complete software development lifecycle. Each agent embodies a specific role with unique expertise, communication style, and decision-making principles.

**Philosophy:** AI agents act as expert collaborators, not code monkeys. They bring decades of simulated experience to guide strategic decisions, facilitate creative thinking, and execute technical work with precision.

### All AIM Agents

**Planning & Architecture (5 agents):**

- PM (Product Manager)
- Strategist (Business Strategist)
- Analyst (Requirements Analyst)
- Architect (System Architect)
- UX Designer

**Meta (1 core agent):**

- AIRchetipo Master (Orchestrator)

**Total:** 6 AIM agents + AIRchetipo Master + cross-module party mode support

---

## Planning & Architecture Agents

### PM (Product Manager) - Valerio 💎

**Role:** Investigative Product Strategist + Market-Savvy PM

**When to Use:**

- Creating Product Requirements Documents (PRD) for Level 2-4 projects
- Creating technical specifications for small projects (Level 0-1)
- Breaking down requirements into epics and stories
- Validating planning documents
- Course correction during implementation

**Primary Phase:** Phase 2 (Planning)

**Workflows:**

- `workflow-status` - Check what to do next
- `create-prd` - Create PRD for Level 2-4 projects
- `tech-spec` - Quick spec for Level 0-1 projects
- `create-epics-and-stories` - Break PRD into implementable pieces
- `validate-prd` - Validate PRD + Epics completeness
- `validate-tech-spec` - Validate Technical Specification
- `correct-course` - Handle mid-project changes
- `workflow-init` - Initialize workflow tracking

**Communication Style:** Direct and analytical. Asks probing questions to uncover root causes. Uses data to support recommendations. Precise about priorities and trade-offs.

**Expertise:**

- Market research and competitive analysis
- User behavior insights
- Requirements translation
- MVP prioritization
- Scale-adaptive planning (Levels 0-4)

---

### Strategist (Business Strategist) - Costanza 🧭

**Role:** Business Strategist + Requirements Expert

**When to Use:**

- Project brainstorming and ideation
- Creating product briefs for strategic planning
- Conducting research (market, technical, competitive)
- Documenting existing projects (brownfield)
- Phase 0 documentation needs

**Primary Phase:** Phase 1 (Analysis)

**Workflows:**

- `workflow-status` - Check what to do next
- `brainstorm-project` - Ideation and solution exploration
- `product-brief` - Define product vision and strategy
- `research` - Multi-type research system
- `document-project` - Brownfield comprehensive documentation
- `workflow-init` - Initialize workflow tracking

**Communication Style:** Analytical and systematic. Presents findings with data support. Asks questions to uncover hidden requirements. Structures information hierarchically.

**Expertise:**

- Requirements elicitation
- Market and competitive analysis
- Strategic consulting
- Data-driven decision making
- Brownfield codebase analysis

---

### Analyst (Requirements Analyst) - Emanuele 🔎

**Role:** Technical Requirements Analyst + Story Crafting Specialist

**When to Use:**

- Breaking PRD into epics and user stories
- Translating business requirements into developer-ready stories
- Creating clear acceptance criteria
- Refining and decomposing requirements
- Ensuring stories meet INVEST principles
- Preparing developer handoffs

**Primary Phase:** Phase 2 (Planning)

**Workflows:**

- `workflow-status` - Check what to do next
- `create-epics-and-stories` - Break PRD requirements into implementable epics and stories
- `party-mode` - Consult with other expert agents

**Communication Style:** Precise and technical. Bridges business requirements with implementation reality. Questions ambiguities proactively. Ensures every story is testable, unambiguous, and developer-ready. Focuses on clear handoffs between planning and execution.

**Expertise:**

- Requirements decomposition
- Epic breakdown into user stories
- Technical feasibility assessment
- PRD to backlog translation
- Story refinement and INVEST principles
- Developer handoff preparation
- Acceptance criteria definition

**Key Principles:**

- Perfect alignment between business intent and technical execution
- User stories are single source of truth for development
- Every requirement must be actionable, verifiable, and implementable
- Acceptance criteria eliminate interpretation gaps
- Enable efficient development handoffs and smooth sprint execution

---

### Architect - Leonardo 📐

**Role:** System Architect + Technical Design Leader

**When to Use:**

- Creating system architecture for Level 2-4 projects
- Making technical design decisions
- Validating architecture documents
- Solutioning gate checks (Phase 3→4 transition)
- Course correction during implementation

**Primary Phase:** Phase 3 (Solutioning)

**Workflows:**

- `workflow-status` - Check what to do next
- `create-architecture` - Produce a Scale Adaptive Architecture
- `validate-architecture` - Validate architecture document
- `solutioning-gate-check` - Validate readiness for Phase 4

**Communication Style:** Comprehensive yet pragmatic. Uses architectural metaphors. Balances technical depth with accessibility. Connects decisions to business value.

**Expertise:**

- Distributed systems design
- Cloud infrastructure (AWS, Azure, GCP)
- API design and RESTful patterns
- Microservices and monoliths
- Performance optimization
- System migration strategies

**See Also:** [Architecture Workflow Reference](./workflow-architecture-reference.md) for detailed architecture workflow capabilities.

---

### UX Designer - Livia ✨

**Role:** User Experience Designer + UI Specialist

**When to Use:**

- UX-heavy projects (Level 2-4)
- Design thinking workshops
- Creating user specifications and design artifacts
- Validating UX designs

**Primary Phase:** Phase 2 (Planning)

**Workflows:**

- `workflow-status` - Check what to do next
- `create-design` - Conduct design thinking workshop to define UX specification with:
  - Visual exploration and generation
  - Collaborative decision-making
  - AI-assisted design tools (v0, Lovable)
  - Accessibility considerations
- `validate-design` - Validate UX specification and design artifacts

**Communication Style:** Empathetic and user-focused. Uses storytelling to explain design decisions. Creative yet data-informed. Advocates for user needs over technical convenience.

**Expertise:**

- User research and personas
- Interaction design patterns
- AI-assisted design generation
- Accessibility (WCAG compliance)
- Design systems and component libraries
- Cross-functional collaboration

---

## Special Purpose Agents

### AIRchetipo Master 🧙

**Role:** AIRchetipo Master Executor, Knowledge Custodian, and Workflow Orchestrator

**When to Use:**

- Listing all available tasks and workflows
- Facilitating multi-agent party mode discussions
- Meta-level orchestration across modules
- Understanding AIRchetipo Core capabilities

**Primary Phase:** Meta (all phases)

**Workflows:**

- `party-mode` - Group chat with all agents (see Party Mode section below)

**Actions:**

- `list-tasks` - Show all available tasks from task-manifest.csv
- `list-workflows` - Show all available workflows from workflow-manifest.csv

**Communication Style:** Direct and comprehensive. Refers to himself in third person ("AIRchetipo Master recommends..."). Expert-level communication focused on efficient execution. Presents information systematically using numbered lists.

**Principles:**

- Load resources at runtime, never pre-load
- Always present numbered lists for user choices
- Resource-driven execution (tasks, workflows, agents from manifests)

**Special Role:**

- **Party Mode Orchestrator:** Loads agent manifest, applies customizations, moderates discussions, summarizes when conversations become circular
- **Knowledge Custodian:** Maintains awareness of all installed modules, agents, workflows, and tasks
- **Workflow Facilitator:** Guides users to appropriate workflows based on current project state

**Learn More:** See [Party Mode Guide](./party-mode.md) for complete documentation on multi-agent collaboration.

---

## Party Mode: Multi-Agent Collaboration

Get all your installed agents in one conversation for multi-perspective discussions, retrospectives, and collaborative decision-making.

**Quick Start:**

```bash
/air:core:workflows:party-mode
# OR from any agent: *party-mode
```

**What happens:** AIRchetipo Master orchestrates 2-3 relevant agents per message. They discuss, debate, and collaborate in real-time.

**Best for:** Strategic decisions, creative brainstorming, post-mortems, sprint retrospectives, complex problem-solving.

**Current AIM uses:** Powers `epic-retrospective` workflow, sprint planning discussions.

**Future:** Advanced elicitation workflows will officially leverage party mode.

👉 **[Party Mode Guide](./party-mode.md)** - Complete guide with fun examples, tips, and troubleshooting

---

## Workflow Access

### How to Run Workflows

**From IDE (Claude Code, Cursor, Windsurf):**

1. Load the agent using agent reference (e.g., type `@pm` in Claude Code)
2. Wait for agent menu to appear in chat
3. Type the workflow trigger with `*` prefix (e.g., `*create-prd`)
4. Follow the workflow prompts

**Agent Menu Structure:**
Each agent displays their available workflows when loaded. Look for:

- `*` prefix indicates workflow trigger
- Grouped by category or phase
- START HERE indicators for recommended entry points

### Universal Workflows

Some workflows are available to multiple agents:

| Workflow           | Agents                        | Purpose                                     |
| ------------------ | ----------------------------- | ------------------------------------------- |
| `workflow-status`  | ALL agents                    | Check current state and get recommendations |
| `workflow-init`    | PM, Strategist, Game Designer | Initialize workflow tracking                |
| `correct-course`   | PM, Architect, Game Architect | Change management during implementation     |
| `document-project` | Strategist, Technical Writer  | Brownfield documentation                    |

### Validation Actions

Many workflows have optional validation workflows that perform independent review:

| Validation              | Agent       | Validates                          |
| ----------------------- | ----------- | ---------------------------------- |
| `validate-prd`          | PM          | PRD + Epics + Stories completeness |
| `validate-tech-spec`    | PM          | Technical specification quality    |
| `validate-architecture` | Architect   | Architecture document              |
| `validate-design`       | UX Designer | UX specification and artifacts     |

**When to use validation:**

- Before phase transitions
- For critical documents
- When learning AIM
- For high-stakes projects

---

## Agent Customization

You can customize any agent's personality without modifying core agent files.

### Location

**Customization Directory:** `{project-root}/{air_folder}/_cfg/agents/`

**Naming Convention:** `{module}-{agent-name}.customize.yaml`

**Examples:**

```
{air_folder}/_cfg/agents/
├── aim-pm.customize.yaml
├── aim-dev.customize.yaml
└── aib-air-builder.customize.yaml
```

### Override Structure

**File Format:**

```yaml
agent:
  persona:
    displayName: 'Custom Name' # Optional: Override display name
    communicationStyle: 'Custom style description' # Optional: Override style
    principles: # Optional: Add or replace principles
      - 'Custom principle for this project'
      - 'Another project-specific guideline'
```

### Override Behavior

**Precedence:** Customization > Manifest

**Merge Rules:**

- If field specified in customization, it replaces manifest value
- If field NOT specified, manifest value used
- Additional fields are added to agent personality
- Changes apply immediately when agent loaded

### Use Cases

**Adjust Formality:**

```yaml
agent:
  persona:
    communicationStyle: 'Formal and corporate-focused. Uses business terminology. Structured responses with executive summaries.'
```

**Add Domain Expertise:**

```yaml
agent:
  persona:
    identity: |
      Expert Product Manager with 15 years experience in healthcare SaaS.
      Deep understanding of HIPAA compliance, EHR integrations, and clinical workflows.
      Specializes in balancing regulatory requirements with user experience.
```

**Modify Principles:**

```yaml
agent:
  persona:
    principles:
      - 'HIPAA compliance is non-negotiable'
      - 'Prioritize patient safety over feature velocity'
      - 'Every feature must have clinical validation'
```

**Change Personality:**

```yaml
agent:
  persona:
    displayName: 'Alex' # Change from default "Amelia"
    communicationStyle: 'Casual and friendly. Uses emojis. Explains technical concepts in simple terms.'
```

### Party Mode Integration

Customizations automatically apply in party mode:

1. Party mode reads manifest
2. Checks for customization files
3. Merges customizations with manifest
4. Agents respond with customized personalities

**Example:**

```
You customize PM with healthcare expertise.
In party mode, PM now brings healthcare knowledge to discussions.
Other agents collaborate with PM's specialized perspective.
```

### Applying Customizations

**IMPORTANT:** Customizations don't take effect until you rebuild the agents.

**Complete Process:**

**Step 1: Create/Modify Customization File**

```bash
# Create customization file at:
# {project-root}/{air_folder}/_cfg/agents/{module}-{agent-name}.customize.yaml

# Example: {air_folder}/_cfg/agents/aim-pm.customize.yaml
```

**Step 2: Regenerate Agent Manifest**

After modifying customization files, you must regenerate the agent manifest and rebuild agents:

```bash
# Run the installer to apply customizations
npx airchetipo install

# The installer will:
# 1. Read all customization files
# 2. Regenerate agent-manifest.csv with merged data
# 3. Rebuild agent .md files with customizations applied
```

**Step 3: Verify Changes**

Load the customized agent and verify the changes are reflected in its behavior and responses.

**Why This is Required:**

- Customization files are just configuration - they don't change agents directly
- The agent manifest must be regenerated to merge customizations
- Agent .md files must be rebuilt with the merged data
- Party mode and all workflows load agents from the rebuilt files

### Best Practices

1. **Keep it project-specific:** Customize for your domain, not general changes
2. **Don't break character:** Keep customizations aligned with agent's core role
3. **Test in party mode:** See how customizations interact with other agents
4. **Document why:** Add comments explaining customization purpose
5. **Share with team:** Customizations survive updates, can be version controlled
6. **Rebuild after changes:** Always run installer after modifying customization files

---

## Best Practices

### Agent Selection

**1. Start with workflow-status**

- When unsure where you are, load any agent and run `*workflow-status`
- Agent will analyze current project state and recommend next steps
- Works across all phases and all agents

**2. Match phase to agent**

- **Phase 1 (Analysis):** Strategist
- **Phase 2 (Planning):** PM, Analyst, UX Designer
- **Phase 3 (Solutioning):** Architect

**3. Use specialists**

- **UX:** UX Designer for user-centered design
- **Requirements:** Analyst for story decomposition
- **Architecture:** Architect for technical design

**4. Try party mode for:**

- Strategic decisions with trade-offs
- Creative brainstorming sessions
- Cross-functional alignment
- Complex problem solving

### Working with Agents

**1. Trust their expertise**

- Agents embody decades of simulated experience
- Their questions uncover critical issues
- Their recommendations are data-informed
- Their warnings prevent costly mistakes

**2. Answer their questions**

- Agents ask for important reasons
- Incomplete answers lead to assumptions
- Detailed responses yield better outcomes
- "I don't know" is a valid answer

**3. Follow workflows**

- Structured processes prevent missed steps
- Workflows encode best practices
- Sequential workflows build on each other
- Validation workflows catch errors early

**4. Customize when needed**

- Adjust agent personalities for your project
- Add domain-specific expertise
- Modify communication style for team preferences
- Keep customizations project-specific

### Common Workflows Patterns

**Starting a New Project (Greenfield):**

```
1. Strategist: *workflow-init
2. Strategist: *brainstorm-project or *product-brief (optional)
3. PM: *create-prd or *tech-spec
4. Architect: *create-architecture (if needed)
5. Analyst: *create-epics-and-stories
```

**Starting with Existing Code (Brownfield):**

```
1. Strategist: *document-project
2. Strategist: *workflow-init
3. PM: *create-prd or *tech-spec
4. Architect: *create-architecture (if needed)
5. Analyst: *create-epics-and-stories
```

**Implementation:**

Work with your preferred development tools and IDE agents for implementation. AIM focuses on planning and requirements preparation.

### Navigation Tips

**Lost? Run workflow-status**

```
Load any agent → *workflow-status
Agent analyzes project state → recommends next workflow
```

**Phase transitions:**

```
Each phase has validation gates:
- Phase 2→3: validate-prd, validate-tech-spec
- Phase 3→4: solutioning-gate-check
Run validation before advancing
```

**Course correction:**

```
If priorities change mid-project:
Load PM or Architect → *correct-course
```

---

## Agent Reference Table

Quick reference for agent selection:

| Agent                 | Icon | Primary Phase   | Key Workflows                                 | Best For                            |
| --------------------- | ---- | --------------- | --------------------------------------------- | ----------------------------------- |
| **Strategist**        | 📊   | 1 (Analysis)    | brainstorm, brief, research, document-project | Discovery, requirements, brownfield |
| **PM**                | 📋   | 2 (Planning)    | prd, tech-spec                                | Planning, requirements docs         |
| **Analyst**           | 🔍   | 2 (Planning)    | create-epics-and-stories                      | Story crafting, PRD decomposition   |
| **UX Designer**       | 🎨   | 2 (Planning)    | create-design, validate-design                | UX-heavy projects, design           |
| **Architect**         | 🏗️   | 3 (Solutioning) | architecture, gate-check                      | Technical design, architecture      |
| **AIRchetipo Master** | 🧙   | Meta            | party-mode, list tasks/workflows              | Orchestration, multi-agent          |

### Agent Capabilities Summary

**Planning & Requirements (3 agents):**

- Strategist: Research, discovery, and workflow initialization
- PM: Requirements documentation (PRD, tech-spec)
- Analyst: Story crafting and requirements decomposition

**Design (2 agents):**

- UX Designer: User experience design
- Architect: System architecture

**Meta (1 agent):**

- AIRchetipo Master: Orchestration and party mode

---

## Additional Resources

**Workflow Documentation:**

- [Phase 1: Analysis Workflows](./workflows-analysis.md)
- [Phase 2: Planning Workflows](./workflows-planning.md)
- [Phase 3: Solutioning Workflows](./workflows-solutioning.md)
- [Phase 4: Implementation Workflows](./workflows-implementation.md)
<!-- Testing & QA Workflows documentation to be added -->

**Advanced References:**

- [Architecture Workflow Reference](./workflow-architecture-reference.md) - Decision architecture details
- [Document Project Workflow Reference](./workflow-document-project-reference.md) - Brownfield documentation

**Getting Started:**

- [Quick Start Guide](./quick-start.md) - Step-by-step tutorial
- [Scale Adaptive System](./scale-adaptive-system.md) - Understanding project levels
- [Brownfield Guide](./brownfield-guide.md) - Working with existing code

**Other Guides:**

- [Enterprise Agentic Development](./enterprise-agentic-development.md) - Team collaboration
- [FAQ](./faq.md) - Common questions
- [Glossary](./glossary.md) - Terminology reference

---

## Quick Start Checklist

**First Time with AIM:**

- [ ] Read [Quick Start Guide](./quick-start.md)
- [ ] Understand [Scale Adaptive System](./scale-adaptive-system.md)
- [ ] Load an agent in your IDE
- [ ] Run `*workflow-status`
- [ ] Follow recommended workflow

**Starting a Project:**

- [ ] Determine project type (greenfield vs brownfield)
- [ ] If brownfield: Run `*document-project` (Strategist or Technical Writer)
- [ ] Load PM or Strategist → `*workflow-init`
- [ ] Follow phase-appropriate workflows
- [ ] Try `*party-mode` for strategic decisions

**Implementing Stories:**

- [ ] Use your preferred development tools and IDE agents for implementation

---

_Welcome to the team. Your AI agents are ready to collaborate._
