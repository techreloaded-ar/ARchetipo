---
name: archetipo-hermes
description: Install ARchetipo on Hermes and manage ARchetipo projects through Hermes Projects and Kanban. Use it to install or update the ARchetipo CLI and workflow skills, create or clone a software project, switch or list projects, inspect project status, or queue one backlog spec on its project board for autonomous plan and implementation up to REVIEW. ARchetipo remains the source of truth for product workflow; Hermes Projects and Kanban provide the project registry and durable execution queue. Do not use this skill for product discovery, planning, implementation, or review directly — load the corresponding archetipo-* workflow skill instead.
metadata:
  hermes:
    category: development
    requires_toolsets: [terminal]
    config:
      - key: archetipo.projects_root
        description: "Base directory where ARchetipo projects live"
        default: "/workspace/projects"
        prompt: "Where do you want to keep your ARchetipo projects?"
      - key: archetipo.profile
        description: "Hermes profile assigned to ARchetipo Kanban cards"
        default: "default"
        prompt: "Which Hermes profile should execute ARchetipo Kanban cards?"
---

# ARchetipo on Hermes

This skill is the thin integration layer between the two products:

- **ARchetipo** owns the PRD, backlog, plans, workflow statuses, implementation and review rules.
- **Hermes Projects** owns the persistent project registry and active-project selection.
- **Hermes Kanban** owns the durable queue of work being executed.

There is one Hermes Project and one same-slug Kanban board for every ARchetipo project. No specialist Hermes profiles are required: cards are assigned to the configured `archetipo.profile` (default `default`) and force-load the ARchetipo skills they need.

The two status systems have different meanings and must not be synchronized bidirectionally:

- ARchetipo statuses (`TODO`, `PLANNED`, `IN PROGRESS`, `REVIEW`, `DONE`) are the source of truth for product delivery.
- Hermes card statuses describe only execution of a queued job. Completing a card never directly changes an ARchetipo status; the loaded ARchetipo skills do that through the CLI.

Resolve these values once per operation:

- `PROJECTS_ROOT` from `archetipo.projects_root`; default `/workspace/projects`.
- `PROFILE` from `archetipo.profile`; default `default`.
- `PKG` as `$(npm root -g)/@techreloaded/archetipo` when package assets are needed.

Select exactly one operation from the request: **install**, **new**, **switch**, **list**, **status**, or **queue**. Infer it; when truly ambiguous, ask one short question.

## Prerequisites

- The `terminal` toolset must be enabled (`hermes tools`).
- Node.js, npm, Git, and a current Hermes installation must be available in the same execution backend.
- `hermes project --help` and `hermes kanban --help` must succeed. If either command is unavailable, ask the user to update Hermes before continuing.
- On Docker, SSH, Modal, or Daytona, ARchetipo, the project directories and the global skills must all exist in that backend.

## install — install or update ARchetipo

This operation is idempotent and refreshes both the CLI and the global workflow skills.

1. Install the latest package and verify the CLI:

   ```bash
   npm install -g @techreloaded/archetipo@latest
   archetipo --version
   ```

2. Copy the packaged workflow skills into the Hermes global skills directory:

   ```bash
   PKG="$(npm root -g)/@techreloaded/archetipo"
   mkdir -p "$HOME/.hermes/skills/archetipo"
   cp -R "$PKG/skills/." "$HOME/.hermes/skills/archetipo/"
   ```

3. Initialize Kanban storage and the projects root:

   ```bash
   hermes kanban init
   mkdir -p "<PROJECTS_ROOT>"
   ```

4. Run `/skills` and confirm the `archetipo-*` skills appear. If they do not, restart the Hermes session so it rescans `~/.hermes/skills/`.

## new — create or onboard a project

Input: project `<name>` and optionally repository `<link>`.

1. Ensure **install** has succeeded.
2. Derive `SLUG` from `<name>` using Hermes board rules: lowercase, replace runs outside `[a-z0-9_-]` with `-`, trim leading/trailing separators, maximum 64 characters. Reject an empty result.
3. Set `DIR=<PROJECTS_ROOT>/<name>`, then:
   - with `<link>` and no existing directory: `git clone "<link>" "$DIR"`;
   - without `<link>` and no existing directory: `mkdir -p "$DIR" && git -C "$DIR" init`;
   - with an existing directory: continue without deleting or overwriting it.
4. Enter the project and install only its runtime assets. Workflow skills remain global:

   ```bash
   cd "$DIR"
   PKG="$(npm root -g)/@techreloaded/archetipo"
   mkdir -p .archetipo
   cp "$PKG/runtime/shared-runtime.md" .archetipo/shared-runtime.md
   [ -f .archetipo/config.yaml ] || cp "$PKG/runtime/config.yaml" .archetipo/config.yaml
   archetipo config show
   ```

5. Run `hermes kanban boards list --json` and parse the array. If `SLUG` is absent, create its board:

   ```bash
   hermes kanban boards create "$SLUG" \
     --name "<name>" \
     --description "ARchetipo project at $DIR" \
     --switch
   ```

   If it already exists, run `hermes kanban boards switch "$SLUG"`.

6. Register and bind the first-class Hermes Project:
   - If `hermes project show "$SLUG"` fails because the project does not exist:

     ```bash
     hermes project create "<name>" "$DIR" \
       --slug "$SLUG" \
       --primary "$DIR" \
       --description "Software project managed with ARchetipo" \
       --board "$SLUG" \
       --use
     ```

   - If it already exists, repair the binding idempotently:

     ```bash
     hermes project add-folder "$SLUG" "$DIR" --primary
     hermes project bind-board "$SLUG" "$SLUG"
     hermes project use "$SLUG"
     ```

7. Confirm with `hermes project show "$SLUG"`, `hermes kanban boards show`, and `archetipo config show`. Suggest `/archetipo-inception` for a new idea or the appropriate workflow skill for an existing ARchetipo backlog.

## switch — change the active project

Input: project `<name>` or slug.

1. Resolve it with `hermes project show "<slug>"`; read its `primary` path and bound `board` from the output.
2. Verify the primary directory exists and contains `.archetipo/`.
3. Activate all three views:

   ```bash
   hermes project use "<slug>"
   hermes kanban boards switch "<board-slug>"
   cd "<primary-path>"
   archetipo config show
   ```

4. Report the project root, connector and active board. The persisted Hermes Project is the restart-safe selection; `cd` is only the current session's working directory.

## list — show projects

Run:

```bash
hermes project list
hermes kanban boards list
```

Present one row per Hermes Project with its primary directory and bound board. Mark the active project. Do not enumerate directories under `PROJECTS_ROOT` as a second registry; report an unregistered `.archetipo/` directory only if the user explicitly asks for discovery or repair.

## status — inspect the active project and board

Run `hermes project list` to identify the active project, then:

```bash
hermes project show "<active-slug>"
hermes kanban boards show
hermes kanban --board "<board-slug>" list
cd "<primary-path>"
archetipo config show
archetipo metrics
```

Report ARchetipo workflow progress separately from Hermes card execution. A completed Hermes card with a spec still outside `REVIEW` is a mismatch to investigate, not a reason to force-move the spec.

## queue — deliver one spec to REVIEW

Input: spec code `<US-CODE>`. This is deliberately one spec at a time; V1 does not scan or drain the backlog automatically.

1. Resolve the active Hermes Project and its primary directory and board. Enter the primary directory and run:

   ```bash
   archetipo config show
   archetipo spec show "<US-CODE>"
   ```

2. Accept only these starting states:
   - `TODO`: the card must run plan, then implement;
   - `PLANNED`: skip planning and run implement;
   - `IN PROGRESS`: resume implement;
   - `REVIEW` or `DONE`: do not create a card; report that no delivery work is needed.

3. Run `hermes kanban --board "<board-slug>" list --json`. If a non-terminal card titled exactly `[ARchetipo] <US-CODE> → REVIEW` already exists, return that card instead of creating a duplicate. A previous `done` card does not block a new card, allowing a later rework cycle.
4. Create one card assigned to `PROFILE`, linked to the Hermes Project and pinned to the project directory. Always load `archetipo-implement`; load `archetipo-plan` too when the current status is `TODO`:

   ```bash
   hermes kanban --board "<board-slug>" create \
     "[ARchetipo] <US-CODE> → REVIEW" \
     --project "<project-slug>" \
     --workspace "dir:<absolute-primary-path>" \
     --assignee "<PROFILE>" \
     --skill archetipo-implement \
     --skill archetipo-plan \
     --body "Deliver ARchetipo spec <US-CODE> to REVIEW. Inspect its current status first. If TODO, execute /archetipo-plan <US-CODE>, then /archetipo-implement <US-CODE>. If PLANNED or IN PROGRESS, execute or resume only /archetipo-implement <US-CODE>. Stop at REVIEW: never run /archetipo-review, spec integrate, or move the spec to DONE. If an ARchetipo skill reports an explicit blocker, block this card with that reason. Complete the card only after archetipo spec show confirms REVIEW, and include tests and residual risks in the Kanban summary."
   ```

   Omit `--skill archetipo-plan` and the planning sentence when starting from `PLANNED` or `IN PROGRESS`.

5. Report the card id and board. Do not wait synchronously for completion; the Hermes gateway dispatcher owns execution. If the gateway is stopped, explain that the card will remain queued until it starts.

## Safety and extension boundary

- Never copy workflow skills into individual projects.
- Never use a Hermes-managed worktree for an ARchetipo delivery card. The card uses `dir:<project-root>` and `archetipo-implement` owns any per-spec worktree configured in `.archetipo/config.yaml`.
- Never modify an ARchetipo artifact or workflow status directly from this integration skill.
- Never run `archetipo-review` or integrate automatically. Human acceptance remains the boundary between `REVIEW` and `DONE`.
- Never create specialist Hermes profiles automatically. `archetipo.profile` may point to one later without changing the project/board/card contract.
- Future versions may add backlog draining, parallel cards, notifications, budgets, or specialized profiles. V1 stores no custom orchestration state that would conflict with those additions.

## Verification

- **install:** `archetipo --version` succeeds; global `archetipo-*` skill directories exist; `hermes kanban init` succeeds.
- **new/switch:** `hermes project show` reports the correct primary directory and board; the same board is active; `archetipo config show` reports the expected project root.
- **queue:** the card is on the project's board, linked to the project, assigned to `PROFILE`, uses `dir:<project-root>`, and loads only the required ARchetipo skills.
