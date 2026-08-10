# ADR 0002: The automatic Wiki is optional

- Status: accepted
- Date: 2026-08-10

## Context

[ADR 0001](0001-living-project-wiki.md) makes the Living Wiki part of the standard workflow: planning declares `wiki_impact`, implementation fulfils it, acceptance approves the required pages. That is the right default, but it is not free. A project may be too small for a knowledge base, may keep its documentation elsewhere, or may simply not want every spec to carry documentation work — and today the only way to avoid it is to let the workflow keep producing Wiki work and ignore it, which turns a maintained artifact into a stale one.

The obvious mechanism — a gate in the CLI that refuses `archetipo wiki ...` — solves the wrong problem. What costs a project is the *automatic* maintenance inside the workflow, not the existence of the commands. Gating the commands would also make the Wiki unreachable for the on-demand use that a project without automatic maintenance needs most: bootstrapping a map of an unfamiliar repository, or answering one question about it.

## Decision

`wiki.enabled` in `.archetipo/config.yaml` gates the Living Wiki **inside the standard workflow only**. It defaults to `true`, so every existing project keeps today's behaviour with no migration.

When it is `false`, `archetipo-inception`, `archetipo-spec`, `archetipo-plan`, `archetipo-implement`, `archetipo-review`, and the workers `archetipo-autopilot` runs perform no Wiki work at all: no `archetipo wiki` command, no Wiki page read as a source, no `wiki_impact` contract, no Wiki task, no Wiki blocker in a verdict, and no change to existing pages or their review state. Wiki absence is then the configured state, never a gap to report.

The gate never applies to the `archetipo wiki` sub-commands nor to an explicit `/archetipo-wiki` invocation: both always run in full. A project with the automatic Wiki off can still bootstrap, query, refresh, and lint one by hand.

The value is read from one place only: `archetipo config show` reports it as `data.wiki.enabled` with the default already applied. An absent key means enabled, so a skill that looked for the key in the YAML file — or inferred the gate from the existence of a Wiki directory — would read a disabled Wiki into every project that never configured one. For the same reason `config show` now also reports the `worktree` and `e2e` sections: every configured behaviour a skill branches on comes from the envelope, not from parsing the config file.

`wiki.enabled` is stored as a nullable boolean. With a plain boolean, an omitted key would be indistinguishable from an explicit `false` — the exact opposite of the intended default.

## Consequences

- A project chooses whether documentation work is part of every spec, without giving up the ability to build knowledge on demand.
- With the gate off an architectural decision has no `type: decision` page to live in, so `archetipo-plan` records qualifying choices — context, chosen option, alternatives, tradeoffs, verification intent — in the technical solution instead. ADR discipline survives; its persistence does not.
- `archetipo doctor` reports a missing Wiki as a skipped check rather than a gap when the gate is off; an existing Wiki is still validated, because it is still usable.
- `archetipo init --no-wiki` starts a project with the gate off. The `archetipo-wiki` skill is installed either way: turning the automatic maintenance back on is a config change, never a reinstall.
- One gate lives in `.archetipo/shared-runtime.md` and every skill refers to it, so the rule cannot drift between the six skills that observe it.
