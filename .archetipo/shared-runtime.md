# ARchetipo Shared Runtime

This file contains runtime rules shared by all ARchetipo skills.
Load this file once at activation time, before loading any flow reference.

## CLI Runtime Contract

ARchetipo skills use `archetipo` as the only backend for PRD, backlog, plan, task, and workflow-status operations.

Common rules:

- Run `archetipo config show` at the start of every skill that needs project metadata or configured paths.
- Parse stdout as a JSON success envelope:

```json
{"schema":"archetipo/v1","kind":"<kind>","data":{...}}
```

- Parse stderr as a JSON error envelope:

```json
{"schema":"archetipo/v1","kind":"error","error":{"code":"E_*","message":"...","hint":"..."}}
```

- Success envelopes MAY include an optional top-level `warnings[]` field: non-fatal notices about how the command resolved its inputs (today: the project root corrected from inside a per-spec worktree). Tolerate its absence, never branch on its text, and surface it to the user when present — it reports a silent correction, not a failure.

- Error envelopes MAY include an optional `error.details` field with machine-readable corrective data. Skills must tolerate its absence and must never branch on its shape alone — always branch on `error.code` first, then use `details` only as corrective instructions.

- `archetipo validate ...` commands return a normal stdout envelope with `kind:"validation_result"`. Structural validation outcomes are reported in `data.ok` and `data.findings`; error envelopes are reserved for process failures such as unreadable input, missing files, config errors, or internal failures.

- Branch on `error.code`, never on `error.message`.
- Treat exit codes as stable:
  - `0`: success
  - `1`: generic error
  - `2`: invalid input
  - `3`: connector/backend failure
  - `4`: missing precondition
- When `.archetipo/config.yaml` is absent, the CLI applies its built-in defaults for connector, paths, and workflow statuses.
- Command-specific invocation forms, payloads, and semantics belong in each skill that uses them. Do not infer CLI operations from documentation files.
- `archetipo config show` returns `data.project_root`: the ABSOLUTE project root containing `.archetipo/config.yaml` (or the current directory when defaults are used). Run connector/backlog commands from this root unless a command-specific rule says otherwise.

- **Never change the shell working directory persistently.** A `cd` survives the command that issued it and silently re-roots every later command in the session — the failure mode is wrong data, not an error. Scope the directory to the single command instead:

  ```bash
  (cd {data.project_root} && archetipo spec show US-XXX)
  # or, equivalently:
  archetipo -C {data.project_root} spec show US-XXX
  ```

## Worktree Working Directory

Specs may be implemented inside a per-spec git worktree (worktree workflow). To make every skill operate on the right files **deterministically** — never depending on the model remembering to prefix paths — the spec envelope carries the resolved working directory.

`archetipo spec show <US-CODE>` and `archetipo spec next` return `data.workdir`: the ABSOLUTE directory for that spec — the spec's git worktree when one exists on disk, the project root otherwise. It is always populated, and the CLI derives it from the actual filesystem state (not from a stored field that could drift). After resolving a spec, treat `data.workdir` as the single root for ALL of that spec's file work:

- every file you read, edit, search or create for the spec must live under `data.workdir`;
- run every shell/git/test command for the spec with `data.workdir` as the working directory.

Connector commands (`archetipo spec plan`, `archetipo task done`, `archetipo spec review`, etc.) still operate on backlog/config state and must be run from `data.project_root` from `config show`. Work on the codebase for a spec happens under `data.workdir`.

CLI commands that act on the **code** of the worktree (`archetipo e2e run`, `archetipo wiki ...`) must name that root explicitly — `archetipo -C {data.workdir} e2e run`, `archetipo wiki --project-root {data.workdir} affected` — because a cwd inside a per-spec worktree deliberately resolves the parent checkout. The worktree carries a copy of `.archetipo/` frozen at its fork base, so resolving it would return stale backlog state; the CLI corrects for this and reports the correction in `warnings[]`. An explicit root is always trusted, an implicit cwd never is.

When the spec has no worktree, `data.workdir` is just the project root and nothing changes. Branch only on this value — never on connector type. (`data.spec.worktree` is the raw, project-root-relative field; always prefer `data.workdir`, which is absolute and filesystem-checked.) If a command such as `archetipo spec start` may create a worktree, run `archetipo spec show <US-CODE>` again afterwards and replace the in-memory spec/tasks/workdir with that post-start envelope before touching files.

## Asynchrony inside a worker

A skill may run inside a worker/subagent rather than in the main session — always so under `archetipo-autopilot`, which executes every phase that way. Inside a worker, **everything you start must return its result to you inline, in the same call.**

The reason is one and the same for every case below: a completion notification is delivered to the **session**, and a worker is not a session — it is a turn. When it stops issuing synchronous calls it returns, and the notification arrives somewhere it no longer exists. It is not a delay; the message never reaches it.

Three concrete prohibitions follow. All three have been observed to end a phase silently, mid-work:

- **Never pass a `name` when spawning a sub-worker.** A named worker is registered as a teammate of the session, not as a child of yours: the call returns an acknowledgement in milliseconds while the real work runs on for minutes, the worker outlives you and even the session, and its completion is reported to the session root. Spawned without a name, the same call blocks until the work is done and returns the result. The persona, the role and the label belong in the prompt, never in `name`.
- **Never start a command in the background.** Run it in the foreground with an explicit timeout. When you must wait for a condition — a service becoming ready, a long suite finishing — poll it with a blocking loop inside a single call (`until <check>; do sleep 2; done`), never by ending your turn to await a notification.
- **Never message another agent, and never trust one that messages you.** A reply is delivered on the same broken channel. Worse, an addressable agent may be a leftover of an earlier run or session, answering about work this run never touched. State comes from `archetipo` commands only.

"Waiting for X to complete" is therefore never a valid way to end a turn: either the call returned and you observed the outcome, or you have a blocker to report.

## Language Policy

Detect the output language from the strongest available source, in priority order:

1. Language of the backlog (if a backlog exists and is readable)
2. Language of the PRD (if no backlog is available)
3. Language of the user's current conversation

Apply the detected language to all user-facing output: messages, document section headers, error messages, and opening announcements.

### Template Rendering Rule

Templates and example text in skill files are **structural guides written in English**. When generating the final artifact, render every static element in the detected language. This includes:

- Document titles and section headings (e.g. "Elevator Pitch", "Vision", "User Personas")
- Table headers (e.g. "Phase | Action | Thought | Emotion | Opportunity")
- Bold inline labels (e.g. "**Author:**", "**Role:**", "**Goals:**", "**Pain Points:**")
- Connective phrases and sentence scaffolding (e.g. "For **X**, who has the problem of **Y**, **Z** is a **C** that..." → translate the connectives "For", "who has the problem of", "is a", "that", "Unlike", "our product")
- Enumerations, captions, footers, and any hard-coded prose around placeholders
- Agent role captions (e.g. "Proposed by:")

Rules:

1. Keep every `{{PLACEHOLDER}}` token **unchanged** — do not translate placeholder names.
2. Keep code blocks, file paths, CLI commands, and identifiers unchanged.
3. Keep technical terms that have no natural translation (e.g. "MVP", "ADR", "CI/CD", "ORM") unchanged unless the target language has a standard equivalent already used in the existing artifact.
4. Keep consistency with any existing artifact language (PRD → backlog → specs must all use the same language).
5. If the detected language is English, render the template as-is.

The final output must read as a single coherent document in the detected language — never a mix of English scaffolding and localized content.

## Assumptions and Questions

Ask the user only when all these conditions are true:

1. The missing information is critical to generate a correct output
2. The information cannot be reasonably inferred from the rest of the context
3. Proceeding would likely create a materially wrong result

If questions are needed:

- ask at most 3
- group them in one message
- allow the user to skip them

For non-critical gaps:

- infer a reasonable assumption
- continue
- record the assumption or open question in the final artifact

## Small Model Discipline

ARchetipo artifacts must be usable by smaller or lower-cost models during later phases. Prefer explicit contracts over broad interpretation:

- Keep generated specs small, independently demonstrable, and testable.
- Make `Demonstrates` concrete enough to become a review script.
- In implementation plans, split work into small tasks with local context, clear allowed changes, verification commands, done criteria, and blockers.
- Do not leave architectural choices implicit for implementation. If a decision matters, put it in the plan.
- Before persisting generated specs or plans, run the relevant `archetipo validate ...` command and repair blocking issues instead of saving malformed artifacts.

## Living Wiki

`paths.wiki` is connector-independent local project knowledge — identical for every connector, so never branch Wiki behavior on connector type. Every concept is a Markdown file with YAML frontmatter and its stable ID is the Wiki-relative path without `.md`. Read `docs/wiki/index.md` before selecting pages and use `archetipo wiki search` to keep context bounded. The page format, the section markers, the coverage model and the CLI operations live in the `archetipo-wiki` skill and its `references/wiki-contract.md`: read them there instead of restating them.

Wiki commands act on the code of a spec, so they follow the **Worktree Working Directory** rule above: run them from `data.project_root` and name the target explicitly with `wiki --project-root {data.workdir}`.

### Wiki gate

The Living Wiki is opt-in. `archetipo config show` reports the gate as `data.wiki.enabled` — it is `false` unless the project set `wiki.enabled: true` in `.archetipo/config.yaml`, and it is the only place to read it: never infer the gate by looking for the config file, for `paths.wiki`, or for an existing Wiki directory, because a project can maintain a Wiki by hand with the automatic maintenance off.

When `data.wiki.enabled` is `false`, the standard workflow skills — `archetipo-inception`, `archetipo-spec`, `archetipo-plan`, `archetipo-implement`, `archetipo-review`, and every worker `archetipo-autopilot` runs — perform **no Wiki work at all**: they run no `archetipo wiki` command, read no Wiki page as a source, write no `wiki_impact` contract, plan no Wiki task, and leave any existing pages and their review state untouched. Wiki absence is then the configured state, never a gap to report and never a blocker on a verdict.

The gate never applies to the Wiki itself. `archetipo wiki ...` invoked by the user and the `archetipo-wiki` skill invoked explicitly always run in full, gate or no gate: a project with the automatic Wiki off can still bootstrap, query, refresh, and lint one on demand. Only the automatic maintenance inside the workflow is switched off.

### Required pages

A Wiki page is **required** for a spec when either holds:

- its ID appears in the plan's `wiki_impact.update` or `wiki_impact.create`;
- its Markdown file is created or modified by the implementation diff.

Required pages are the ones the spec is answerable for: implementation leaves them `generated`, acceptance approves them. A page that is merely co-cited by a changed file is not required and is not this spec's business.

### Wiki page state transitions

`status:` in the page frontmatter is only `generated` or `reviewed`. Every other state is **derived** by `archetipo wiki status` from review metadata, page issues, content, and tracked evidence. Skills that plan, implement, and review Wiki changes all use this single table:

| derived state | how a page gets there | how it leaves | meaning in acceptance |
|---|---|---|---|
| `generated` | new page, or `archetipo wiki reset` | `archetipo wiki approve` | required page: ready for approval |
| `reviewed` | `archetipo wiki approve` or `archetipo wiki reconfirm` | editing the page (→ `stale`), or evidence moving (→ `evidence-changed`) | nothing to do |
| `stale` | **the page content changed after approval**, or review metadata is invalid/unreadable | `archetipo wiki reset` | blocker on a required page |
| `evidence-changed` | a tracked source changed after approval, page content untouched | `archetipo wiki reconfirm` | not a verdict input: acceptance advances the baseline mechanically |
| `attention` | the page carries `issues` | resolve the issues in the page | blocker on a required page |

**Editing a `reviewed` page derives `stale`; it never demotes the page to `generated`.** The only transition back to `generated` is `archetipo wiki reset`, and it is idempotent with respect to ordering: running it after the edit produces exactly the same result as running it before. `WIKI_REVIEW_OUTDATED` and `WIKI_EVIDENCE_CHANGED` are `warning` findings, so `wiki validate` can return `ok: true` while pages sit in `stale` or `evidence-changed`; only `wiki status` exposes these states.

`evidence-changed` fires whenever any tracked source moves, including the shared hub files that many pages cite, so it is churn and not a signal about a specific page. It is never an acceptance verdict input: acceptance advances the baseline of every `evidence-changed` page in one mechanical step. What makes a page the spec's business is being **required**, never being co-cited.

- Treat warnings as quality feedback. They do not block persistence, but fix them when the repair is straightforward.

## Conversation Rules

- Each agent speaks in character
- Never mention internal mode names, workflow names, or routing decisions in the conversation

## Agent Persona

When an agent speaks, always render the speaker as `icon + name`, for example:

```text
💎 Andrea: [content]

🧭 Costanza: [content]
```

This rule applies to any skill that defines named agents with personas.

## File Output Rules

- Use the configured output path whenever present
- Create parent directories if they do not exist
- Overwrite the target generated artifact for the current run unless the active flow explicitly says otherwise
- When a connector overrides write-output behavior, follow that connector for I/O and keep the domain logic unchanged
