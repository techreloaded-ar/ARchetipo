# ARchetipo on Hermes

Use ARchetipo as the software-delivery tool inside [Hermes](https://github.com/nousresearch/hermes-agent), while Hermes Projects and Kanban provide the persistent project registry and execution queue.

The integration is a single skill: [`archetipo-hermes`](archetipo-hermes/SKILL.md). It installs ARchetipo and manages a deliberately small V1:

- one first-class Hermes Project per software project;
- one same-slug Hermes Kanban board per project;
- ARchetipo workflow skills installed globally once;
- optional Kanban cards that deliver one ARchetipo spec to `REVIEW`.

No specialist Hermes profiles are required. Cards use the configured profile (default `default`) and load `archetipo-plan` and `archetipo-implement` directly.

## Responsibilities

| Component | Owns |
|---|---|
| ARchetipo | PRD, backlog, plans, product statuses, code workflow and review rules |
| Hermes Project | Project name, primary directory, active selection and board binding |
| Hermes Kanban | Durable execution cards, retries, blocking and run history |

ARchetipo remains the source of truth. A Kanban card tracks execution; it does not replace or mirror the ARchetipo backlog.

## Bootstrap

1. Use a current Hermes version with `hermes project` and `hermes kanban`, and enable the `terminal` toolset:

   ```bash
   hermes tools
   ```

2. Install the integration skill:

   ```bash
   hermes skills install techreloaded-ar/ARchetipo/integrations/hermes/archetipo-hermes
   ```

3. Ask Hermes:

   > install or update ARchetipo

   The skill installs `@techreloaded/archetipo`, copies the packaged workflow skills into `~/.hermes/skills/archetipo/`, and initializes Kanban storage. Run `/skills` to verify discovery; restart the session if Hermes needs to rescan.

## Project lifecycle

Ask naturally:

- *"Create a new ARchetipo project called shopper."*
- *"Clone https://github.com/acme/foo and manage it with ARchetipo."*
- *"Switch to shopper."*
- *"List my ARchetipo projects."*
- *"Show the status of the active project."*

Creating or onboarding a project creates and binds:

```text
Hermes Project: shopper
  primary: /workspace/projects/shopper
  board: shopper

Hermes Kanban board: shopper

ARchetipo project: /workspace/projects/shopper/.archetipo/
```

The project root defaults to `/workspace/projects` and is configurable through the skill setting `archetipo.projects_root`.

## Queue one spec

To let Hermes deliver a spec autonomously:

> Queue US-003 on the active ARchetipo project.

The skill creates one card named `[ARchetipo] US-003 → REVIEW` on the bound project board. The card:

- uses the project's existing checkout rather than a Hermes worktree;
- loads `archetipo-plan` when the spec is `TODO`;
- loads `archetipo-implement` for implementation and tests;
- stops when ARchetipo confirms the spec is in `REVIEW`.

The human acceptance gate remains explicit: use `/archetipo-review US-003` later to approve, request changes, or postpone. V1 never integrates or moves a spec to `DONE` automatically.

The executing profile defaults to Hermes' `default` profile and can be changed with the skill setting `archetipo.profile`. This is a normal Hermes profile, not a specialized ARchetipo worker.

## Why there are two boards of state

They answer different questions:

- ARchetipo status: *where is the product increment in its delivery lifecycle?*
- Hermes card status: *what is happening to this particular execution request?*

If a Hermes card finishes but the spec has not reached `REVIEW`, treat it as a failed or incomplete execution and investigate it. Do not force the ARchetipo status to match the card.

## Connectors and worktrees

Every project chooses its own ARchetipo connector in `.archetipo/config.yaml`: `file`, `github`, or `jira`. Hermes does not branch on the connector.

Queued cards always run in `dir:<project-root>`. When ARchetipo worktrees are enabled, `archetipo-implement` creates and owns the per-spec branch and worktree. This prevents Hermes and ARchetipo from creating nested or competing worktrees.

## Deliberate V1 boundary

V1 queues one explicitly selected spec at a time. It does not drain the backlog, create specialist profiles, parallelize specs, notify external channels, enforce budgets, deploy, or auto-approve reviews.

Those capabilities can be added later without changing the core mapping — Hermes Project ↔ Kanban board ↔ ARchetipo project — or the card contract that stops at `REVIEW`.
