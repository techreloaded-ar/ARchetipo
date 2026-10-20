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
- Invoke each CLI operation on its own, parse its envelope, then decide the next action. Never concatenate a CLI verification with a raw test command, another CLI call, a pipe, or `exit 0` that hides its result. Parallel calls keep separate outputs.
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

A skill may run inside a worker/subagent rather than in the main session. Complete and observe everything you start before returning from that phase.

Use the runtime's documented lifecycle rather than inferring it from a worker's name:

- With a blocking spawn tool, consume the result inline. Omit an optional `name` when that runtime routes named workers as detached teammates.
- With a spawn-and-wait tool pair, retain the newly created worker ID and explicitly wait for its terminal result. Required `name` or `task_name` fields are allowed. A spawn acknowledgement, progress message or wait timeout is not completion; continue waiting in the same active turn. Do not finish the phase expecting a later notification to resume it.
- If nested delegation cannot be awaited reliably, use the phase skill's inline path. A caller requiring phase isolation must retain that boundary.
- Run commands in the foreground. If a command tool yields a session handle, await it with that tool's documented wait/poll operation until it terminates. Never leave a required command running when returning the phase result.
- Ignore unsolicited messages from unrelated or earlier workers. Use only results correlated with the worker IDs you created for this phase; verify workflow state with `archetipo` commands.

"Waiting for X to complete" is never a valid way to end a turn: observe its terminal outcome first, or report a concrete blocker.

## Local verification environment

Preparing what is needed to verify an approved task belongs to delivery, regardless
of how the skill was invoked. Inspect the project's startup, authentication and
existing test tools. Plan and execute missing local setup, dedicated synthetic
accounts/data and service readiness checks before declaring an environment blocker.
Use available services without altering unrelated instances; keep configuration
inside the working directory. Never use production credentials or data. Report a
specific blocker only after permitted setup has been attempted and cannot proceed.
Record repeatable commands and cleanup; identify any preview intentionally left up.
Required real browser/API/database checks cannot be replaced with mocks.

Reuse a service only when it was explicitly supplied for this verification or its
ownership by this working directory and test scope can be verified. A familiar
name, a spec-code prefix, a loopback port or synthetic-looking data is not proof
of ownership. Namespace newly created resources by the working directory and
scope; retain a verifiable owner label or equivalent identifier. If ownership or
credentials are unavailable, create a fresh isolated resource. Verification does
not authorize changing credentials, dropping data or reconfiguring an unrelated
existing instance. Do not search other checkouts for secrets to repair setup.
Before running installers or generators, check that their target dependencies and
outputs belong to this working directory; resolving modules through an ancestor
checkout or symlink does not authorize mutating that checkout's installation.

Make required setup reproducible across fresh workers. Reuse the project's setup
tooling, or add a small idempotent test setup command when it is missing. It must
locate only owned resources, choose an available local endpoint, provide synthetic
configuration, check readiness and clean up only its own data/resources. Reference
that command and its ownership/readiness/cleanup contract in the CLI-persisted
plan when present, otherwise in the delivery summary with retained evidence. Keep the recipe with the project tests; shell history, temporary notes and
an untracked environment file alone are not a handoff. A later phase reads this
recipe before discovering or creating another environment.

Retain verification output in the configured spec test-results directory, with
the exact command, actual result and tested source identified. A console-only
success or an output file left solely in the system temporary directory is not
durable acceptance evidence. Missing evidence is unfinished verification work,
to repair before handing the phase to its caller.

## Reusable project verification

Use a single project recipe at `.archetipo/environment.json` for repeatable setup
and named checks. Read and persist it through `execution environment show` and
`configure`; use `prepare` and `check` under the actual code working directory.
Prepare once before baseline commands or generators. A missing recipe is a
bounded project-onboarding task, not a reason to rediscover the environment in
all later phases. Describe explicit argv, timeouts, local readiness probes,
synthetic service ownership and cleanup; never put secret values in argv/config.
Do not infer a framework or reuse an ancestor's dependencies. A readiness probe
must verify usable local outputs and owned services, not merely that a port opens.

The CLI probes readiness on reuse, serializes setup, keeps immutable full logs,
and can reuse a passing check only for identical declared inputs. Include all
relevant code, tests, configuration, lockfiles and toolchain files in its inputs;
undeclared clocks, network state or external data make a check non-cacheable.
Register the exact required project scripts as named checks, including the final
suite; do not register a one-file subset as the complete gate. When a named check
already runs the entire required suite, its valid report satisfies both focused
and final verification: do not run the same suite again through a raw alias.
If another required command adds coverage, register it through `configure` before
executing it. Raw diagnostic commands never substitute for a required gate.
Inspect `passed`, `stable`, `report_path` and, on reuse, `reused_report`; a process
exit of zero is not sufficient. Failed or interrupted work never counts as a pass.
Read the original evidence behind a reused report. Use `--fresh` when reproducing
a new concern, not simply because another phase has started.

When a registered check is denied by the runtime sandbox (for example local
network/listener permission), request the runtime's one-time permission for the
same `archetipo ... execution environment check ...` command and rerun it intact.
Do not fall back to its raw node/npm/pytest command or copy console text into a
made-up passing report. An authorized diagnostic run may help debugging but never
replaces the registered check's durable successful evidence. Preserve approval
policy; do not request a broader profile or global allow rule to avoid this wait.

Reuse the healthy owned environment across tasks and phases. Do not rebuild,
reinstall, reseed or rerun every check for a handoff with unchanged relevant
inputs. Run affected checks after edits; run the complete required gate once for
the final candidate. Keep build separate from fixture-only retries when inputs
permit it. Cleanup owned services at the end of delivery or an explicit stop;
identify any intentionally retained preview. Never relax permissions to hide
approval waits: use stable, bounded project commands and respect runtime policy.

Pass the human's detected communication language explicitly at every delegation
boundary. Do not redetect it from an English technical handoff.

## Nested skill completion

A skill invoked inline by another active skill returns its phase result to that
caller. A completed phase is not permission to end the native turn: continue the
caller's remaining contract. Only the outermost invocation returns control to
the person at its declared terminal state or a concrete blocker. This generic
rule does not give a base phase knowledge of any particular composing skill.

## Resumable execution checkpoints

A composing skill may persist its own execution state through the CLI. Base skills
complete their own phase and need no knowledge of their caller. The checkpoint is
bookkeeping: specs, tasks, Git and native runtime state remain authoritative.

Each composing skill declares the checkpoint commands, payloads and errors it
actually uses. Keep the executable invocation separate from the normalized scope;
internal selection sentinels are not command arguments. Revision checks prevent
stale writers from replacing observed progress; reconcile before retrying a write.

Keep the run, scope and retry counters across interruptions. Store worker IDs
before waiting. On resume, establish that earlier workers are terminal before
starting another writer; reconcile partial edits and rerun affected checks.
A View may start a bounded reconciliation turn on the same native session after
transport loss for an unfinished conversation-bound run. It never replays an
uncertain submission, interrupts a human wait, or approves publication. Standalone
invocation uses the same checkpoint and does not require View. A blocked run needs
an explicit request to resume; a completed run is immutable history.

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

When `data.wiki.enabled` is `false`, the standard workflow skills — `archetipo-inception`, `archetipo-spec`, `archetipo-plan`, `archetipo-implement`, `archetipo-review`, including delegated invocations — perform **no Wiki work at all**: they run no `archetipo wiki` command, read no Wiki page as a source, write no `wiki_impact` contract, plan no Wiki task, and leave any existing pages and their review state untouched. Wiki absence is then the configured state, never a gap to report and never a blocker on a verdict.

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
