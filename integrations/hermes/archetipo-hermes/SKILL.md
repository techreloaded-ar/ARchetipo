---
name: archetipo-hermes
description: Install ARchetipo on a Hermes instance and manage multiple projects conversationally. Use it to install the ARchetipo CLI and workflow skills, to create a new software project (a fresh directory under the projects root, ready for the spec-driven workflow), to clone and onboard an existing repository, or to switch the active project. After this skill has set up a project, the installed ARchetipo workflow skills (/archetipo-inception, /archetipo-spec, /archetipo-plan, /archetipo-implement, /archetipo-review, and /archetipo-wiki when the package ships it) operate on it. Do not use this skill for the product work itself — it only handles installation and project lifecycle.
metadata:
  hermes:
    category: development
    requires_toolsets: [terminal]
    config:
      - key: archetipo.projects_root
        description: "Base directory where ARchetipo projects live"
        default: "/workspace/projects"
        prompt: "Where do you want to keep your ARchetipo projects?"
---

# ARchetipo on Hermes

This skill installs ARchetipo and manages the lifecycle of your projects on a Hermes instance. It is the only ARchetipo skill that is Hermes-specific; everything else (the actual spec-driven workflow) is handled by the tool-agnostic `archetipo-*` workflow skills, which this skill installs once.

The mental model is simple and rests on two facts:

- **Hermes keeps the working directory across turns** (for the lifetime of the process). A `cd` persists.
- **ARchetipo resolves the active project from the current working directory**: `archetipo config show` walks up from the cwd to `.archetipo/config.yaml` (or applies built-in defaults) and returns `data.project_root`.

Therefore **the active project is just the current working directory**. Switching project = `cd` into it. Nothing else to track.

Two locations, do not confuse them:

| What | Where | How often |
|---|---|---|
| Workflow skills `archetipo-*` | `~/.hermes/skills/archetipo/` (**global**) | installed **once** |
| `.archetipo/config.yaml` + `shared-runtime.md` + backlog/plans | `<projects_root>/<name>/.archetipo/` (**per project**) | one per project |

## Prerequisites

- The `terminal` toolset must be enabled (`hermes tools`). Every operation below runs shell commands.
- Node.js and npm are present in the Hermes environment (the standard install bundles them).
- Resolve `PROJECTS_ROOT` from the `archetipo.projects_root` setting; default `/workspace/projects`.
- The installed npm package ships the skills and runtime templates, so resolve the package directory once when needed:
  ```bash
  PKG="$(npm root -g)/@techreloaded/archetipo"
  ```

Select exactly one operation from the request: **install**, **new**, **switch**, or **list**. Infer it; when unsure, ask a single short question.

## install

Run once per Hermes environment (idempotent — safe to repeat to upgrade).

1. Install the CLI if missing, then verify:
   ```bash
   command -v archetipo >/dev/null 2>&1 || npm install -g @techreloaded/archetipo
   archetipo --version
   ```
2. Copy the workflow skills into the Hermes skills directory (this is what makes `/archetipo-*` available — no `archetipo init` is used for this):
   ```bash
   PKG="$(npm root -g)/@techreloaded/archetipo"
   mkdir -p "$HOME/.hermes/skills/archetipo"
   cp -R "$PKG/skills/." "$HOME/.hermes/skills/archetipo/"
   ```
3. Ensure the projects root exists:
   ```bash
   mkdir -p "<PROJECTS_ROOT>"
   ```
4. Tell the user the workflow skills are installed and that Hermes may need to re-scan them: run `/skills` to confirm `archetipo-wiki`, `archetipo-spec`, etc. appear. If they do not, restart the Hermes session/process so it re-reads `~/.hermes/skills/`.

## new — create or onboard a project

Input: a project `<name>`, and optionally a repository `<link>` to clone.

1. Ensure ARchetipo is installed (run **install** first if `archetipo --version` fails).
2. Resolve the target directory and create/clone it:
   ```bash
   DIR="<PROJECTS_ROOT>/<name>"
   # with a repo link:
   git clone "<link>" "$DIR"
   # without a link (new empty project):
   mkdir -p "$DIR" && git -C "$DIR" init
   ```
   If `$DIR` already exists, treat it as an existing project and continue (do not clobber it).
3. Make it the active project (persistent cwd):
   ```bash
   cd "<PROJECTS_ROOT>/<name>"
   ```
4. Prepare the per-project ARchetipo assets. **Do not copy the workflow skills here** — they are already global. Only the runtime contract and (optionally) the config template:
   ```bash
   PKG="$(npm root -g)/@techreloaded/archetipo"
   mkdir -p .archetipo
   cp "$PKG/runtime/shared-runtime.md" .archetipo/shared-runtime.md
   [ -f .archetipo/config.yaml ] || cp "$PKG/runtime/config.yaml" .archetipo/config.yaml
   ```
   The default connector is `file` (specs and backlog are versioned in the repo). `config.yaml` is optional — without it the CLI applies the same defaults — so copying it just makes the connector explicit and editable later.
5. Record the active project so it survives a process restart:
   ```bash
   echo "<PROJECTS_ROOT>/<name>" > "$HOME/.hermes/skills/archetipo/active-project"
   ```
6. Confirm and hand off:
   ```bash
   archetipo config show
   ```
   Announce the active project and `data.project_root`, then suggest the next step based on the skills actually installed (check `ls ~/.hermes/skills/archetipo/`): for a cloned/existing codebase, `/archetipo-wiki bootstrap` when the `archetipo-wiki` skill is present, otherwise `/archetipo-inception`; for a brand-new idea, `/archetipo-inception`.

## switch — change the active project

Input: a project `<name>` (or its path).

1. Verify it is an ARchetipo project and switch into it:
   ```bash
   test -d "<PROJECTS_ROOT>/<name>" || { echo "not found"; }
   cd "<PROJECTS_ROOT>/<name>"
   archetipo config show
   echo "<PROJECTS_ROOT>/<name>" > "$HOME/.hermes/skills/archetipo/active-project"
   ```
2. Report `data.project_root` and the active connector. From now on every `/archetipo-*` command operates on this project because it reads the cwd.

## list — show projects

1. Enumerate initialized projects and show the active one:
   ```bash
   for d in "<PROJECTS_ROOT>"/*/; do [ -d "$d/.archetipo" ] && echo "$(basename "$d")"; done
   pwd
   cat "$HOME/.hermes/skills/archetipo/active-project" 2>/dev/null
   ```
2. Present the list, mark the active project, and offer to `switch` or create a `new` one.

## Resume after a restart

The cwd only persists for the lifetime of the Hermes process. At the start of a session, if the cwd is not inside a project (`archetipo config show` returns the default root, i.e. no `.archetipo/config.yaml` above the cwd), read `~/.hermes/skills/archetipo/active-project` and offer to resume it (`switch`), or run **list** so the user can choose.

## Pitfalls

- **Do not re-copy the workflow skills per project.** They live once in `~/.hermes/skills/`. `new` only writes `.archetipo/` runtime assets.
- **Never invent a projects root.** Always use the configured `archetipo.projects_root` (or its default), and create it before use.
- **Secrets never go in `config.yaml`.** The `github` connector needs an authenticated `gh`; the `jira` connector reads `JIRA_API_TOKEN`/`JIRA_EMAIL` from the environment — put those in `~/.hermes/.env`. The default `file` connector needs nothing.
- **Remote backends.** On Docker/SSH/Modal/Daytona, the CLI, the global skills, and `projects_root` must live in the backend where Hermes runs commands, not on the host.
- **Do not switch the connector during creation.** New projects start on `file`; changing to `github`/`jira` is a later, explicit action (edit `.archetipo/config.yaml`).

## Verification

- After **install**: `archetipo --version` succeeds and `ls ~/.hermes/skills/archetipo/` lists the `archetipo-*` skill directories.
- After **new**: `pwd` is `<PROJECTS_ROOT>/<name>`, `.archetipo/shared-runtime.md` exists, and `archetipo config show` returns `data.project_root` equal to that directory with the expected connector.
- After **switch**: `archetipo config show` reports the target project's `data.project_root`.
