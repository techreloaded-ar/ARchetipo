# AIM - AIRchetipo Module

Core orchestration system for AI-driven agile development, providing comprehensive lifecycle management through specialized agents and workflows.

---

## 📚 Complete Documentation

👉 **[AIM Documentation Hub](./docs/README.md)** - Start here for complete guides, tutorials, and references

**Quick Links:**

- **[Quick Start Guide](./docs/quick-start.md)** - New to AIM? Start here (15 min)
- **[Agents Guide](./docs/agents-guide.md)** - Meet your 5 specialized AI agents (45 min)
- **[Scale Adaptive System](./docs/scale-adaptive-system.md)** - How AIM adapts to project size (42 min)
- **[FAQ](./docs/faq.md)** - Quick answers to common questions
- **[Glossary](./docs/glossary.md)** - Key terminology reference

---

## 🏗️ Module Structure

This module contains:

```
aim/
├── agents/          # 5 specialized AI agents (PM, Strategist, Architect, SM, UX Designer)
├── workflows/       # Planning and solutioning workflows across 3 phases
├── teams/           # Pre-configured agent groups
└── docs/            # Complete user documentation
```

### Agent Roster

**Planning & Solutioning:** PM, Strategist, Architect, SM, UX Designer
**Orchestration:** AIRchetipo Master (from Core)

👉 **[Full Agents Guide](./docs/agents-guide.md)** - Roles, workflows, and when to use each agent

### Workflow Phases

**Phase 0:** Discovery (optional) - Research and product brief workflows
**Phase 1:** Planning (required) - PRD and tech-spec workflows
**Phase 2:** Solutioning (required for complex projects) - Architecture and gate check

👉 **[Workflow Guides](./docs/README.md#-workflow-guides)** - Detailed documentation for each phase

---

## 🚀 Getting Started

**New Project:**

```bash
# Install AIM
npx airchetipo@alpha install

# Load Strategist agent in your IDE, then:
*workflow-init
```

**Existing Project (Brownfield):**

```bash
# Document your codebase first
*document-project

# Then initialize
*workflow-init
```

👉 **[Quick Start Guide](./docs/quick-start.md)** - Complete setup and first project walkthrough

---

## 🎯 Key Concepts

### Scale-Adaptive Design

AIM automatically adjusts to project complexity:

- **Quick Flow:** Tech-spec based planning for small features (1-15 stories)
- **Method Track:** Full PRD + architecture for medium projects (10-50 stories)
- **Enterprise Track:** Extended planning with security and compliance (30+ stories)

👉 **[Scale Adaptive System](./docs/scale-adaptive-system.md)** - Complete track breakdown

### Product Discovery to Solutioning

AIM guides you from initial product ideas through complete solution design:

- **Discovery:** Research, brainstorming, and product brief creation
- **Planning:** PRD or tech-spec with epics and stories
- **Solutioning:** Architecture decisions and implementation-ready design

The workflow terminates at solutioning-gate-check, providing a complete handoff package for development.

### Multi-Agent Collaboration

Use party mode to engage all installed agents (from AIM, AIB, custom modules) in group discussions for strategic decisions, creative brainstorming, and complex problem-solving.

👉 **[Party Mode Guide](./docs/party-mode.md)** - How to orchestrate multi-agent collaboration

---

## 📖 Additional Resources

- **[Brownfield Guide](./docs/brownfield-guide.md)** - Working with existing codebases
- **[Quick Spec Flow](./docs/quick-spec-flow.md)** - Fast-track for Level 0-1 projects
- **[Enterprise Agentic Development](./docs/enterprise-agentic-development.md)** - Team collaboration patterns
- **[Troubleshooting](./docs/troubleshooting.md)** - Common issues and solutions
- **[IDE Setup Guides](../../../docs/ide-info/)** - Configure Claude Code, Cursor, Windsurf, etc.

---

## 🤝 Community

- **[Discord](https://discord.gg/gk8jAdXWmj)** - Get help, share feedback (#general-dev, #bugs-issues)
- **[GitHub Issues](https://github.com/airchetipo-org/AIRchetipo/issues)** - Report bugs or request features
- **[YouTube](https://www.youtube.com/@AIRchetipoCode)** - Video tutorials and walkthroughs

---

**Ready to build?** → [Start with the Quick Start Guide](./docs/quick-start.md)
