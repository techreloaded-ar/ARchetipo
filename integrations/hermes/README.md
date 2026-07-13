# ARchetipo on Hermes

Run ARchetipo inside a [Hermes](https://github.com/nousresearch/hermes-agent) instance and manage multiple projects conversationally — with **zero changes to the ARchetipo CLI**.

Everything is driven by a single, self-contained Hermes skill: [`archetipo-hermes`](archetipo-hermes/SKILL.md). It installs the ARchetipo CLI, installs the tool-agnostic workflow skills once into `~/.hermes/skills/`, and manages the project lifecycle (create, clone, switch, list).

## How it works

Two facts make this simple:

- Hermes keeps the working directory across turns (for the lifetime of the process).
- ARchetipo resolves the active project from the current working directory (`archetipo config show` → `data.project_root`).

So **the active project is just the current working directory**. Switching project is a `cd`.

The workflow skills are plain files shipped inside the npm package (`skills/` in the package root). The `archetipo-hermes` skill copies them from `$(npm root -g)/@techreloaded/archetipo/skills/` into `~/.hermes/skills/archetipo/` — no CLI subcommand is involved.

| What | Where | How often |
|---|---|---|
| Workflow skills `archetipo-*` | `~/.hermes/skills/` (global) | installed once |
| `.archetipo/config.yaml` + `shared-runtime.md` + backlog/plans | `<projects_root>/<name>/.archetipo/` (per project) | one per project |

## Bootstrap

1. Enable the `terminal` toolset in Hermes:
   ```bash
   hermes tools
   ```
2. Install this skill (easiest path — Hermes installs a skill straight from a repo path or URL):
   ```bash
   hermes skills install techreloaded-ar/ARchetipo/integrations/hermes/archetipo-hermes
   ```
   Alternatively, copy `archetipo-hermes/SKILL.md` into `~/.hermes/skills/archetipo-hermes/`.
3. In Hermes, ask it to install ARchetipo:
   > install ARchetipo

   The skill runs `npm install -g @techreloaded/archetipo` and copies the workflow skills into `~/.hermes/skills/`. Run `/skills` to confirm they appear (restart the session if Hermes needs to re-scan).

## Everyday use

- **Create a new project:** *"create a new project called shopper"* → a fresh directory under the projects root, initialized and made active. Then `/archetipo-inception`.
- **Onboard an existing repo:** *"let's work on https://github.com/acme/foo"* → cloned under the projects root, initialized, made active. Then `/archetipo-wiki bootstrap` (when the `archetipo-wiki` skill is installed) or `/archetipo-inception`.
- **Switch project:** *"switch to shopper"* → `cd` into it; every `/archetipo-*` command now operates there.
- **List projects:** *"what projects do I have?"*

The projects root defaults to `/workspace/projects` and is configurable via the `archetipo.projects_root` skill setting.

## Connectors and where specs live

Each project is self-contained and chooses its own connector in `<project>/.archetipo/config.yaml` (default `file`):

- `file` — backlog/specs/plans as YAML in the repo (`.archetipo/backlog.yaml`, `.archetipo/plans/`). No setup.
- `github` — specs as GitHub Issues + Projects v2. Requires an authenticated `gh`.
- `jira` — specs as Jira issues. Requires `JIRA_API_TOKEN` / `JIRA_EMAIL` (put them in `~/.hermes/.env`).

PRD, Wiki, mockups and test results are always files in the repo (`docs/…`), for every connector.
