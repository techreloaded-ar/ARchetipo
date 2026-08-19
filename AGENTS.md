# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Project

ARchetipo is a set of skills for AI coding agents (Claude Code, Codex, Cursor, Gemini CLI, OpenCode, and GitHub Copilot) that supports the software project ideation, analysis, and planning process.

## Repository structure

```text
skills/                  # Main skills (one directory per skill)
  <skill-name>/
    SKILL.md             # Skill definition
    references/          # Supporting files loaded by the skill
skills-extra/            # Extra skills (same structure)
.archetipo/              # Files installed in the target project (mirrors target structure)
  config.yaml            # Configuration template for the target project
  shared-runtime.md      # Shared rules (Language Policy, Persona, etc.)
cli/                     # Go module implementing the `archetipo` CLI
  cmd/archetipo/         # Binary entry point
  internal/
    cli/                 # Cobra subcommands (public CLI surface)
    domain/              # Shared data types
    connector/           # Interface and implementations (filefs, github)
    config/              # `.archetipo/config.yaml` loader
    iox/                 # JSON envelope for stdin/stdout/stderr
npm/                     # npm package (@techreloaded/archetipo + 6 platform packages)
scripts/                 # npm package build and publishing scripts
```

## Connector architecture

Skills do not manage persistence directly and do not perform connector operations by interpreting instructions. The flow is always:

1. The skill reads `.archetipo/shared-runtime.md` for the JSON envelope, error rules, and invocation discipline.
2. The skill invokes `archetipo <subcmd>` (the Go binary installed globally through `npm i -g @techreloaded/archetipo`).
3. The CLI reads `.archetipo/config.yaml`, selects the connector (`file` or `github`), and performs the operation deterministically.

Skills must explicitly include only the CLI subcommands they actually use, together with their payloads, expected envelopes, and relevant `error.code` values. There is no separate file describing the entire protocol.

## Rules for skill authors

- Call only the subcommands the skill actually uses.
- Content templates (PRDs, story bodies, plan bodies, and sub-issue bodies) are produced by the skill and passed to the CLI through stdin. The CLI persists the payload without enriching it.
- Validation and post-processing of JSON output belong in the skill.
- No-op subcommands are explicit. For example, `comment post` returns `ok: true` with the `file` connector as well. A skill must never branch on connector type.
- Branch on the JSON envelope's `error.code`, not on `message`.
- Load `.archetipo/shared-runtime.md` **exactly once** when the skill starts.

## Rules for CLI changes

- The 14 public CLI operations are stable. Any incompatible change is a breaking change and must be versioned accordingly.
- Keep the conformance suite (`cli/internal/connector/conformance/`) green for all implementations: file, github, and inmemory.
- All GitHub connector GraphQL queries live in `cli/internal/connector/github/templates.go`. Add snapshot tests before modifying them.
- Distribution: the binary and skills share one repository tag. For `v*` tags, `release.yml` runs GoReleaser to produce binaries in `cli/dist/`; `scripts/build-npm.mjs` then copies them into the six `@techreloaded/archetipo-{os}-{arch}` packages and copies the skills into the main `@techreloaded/archetipo` package; finally, `scripts/publish-npm.mjs` publishes all seven npm packages.
- **Before delivering changes**, run the same checks as CI locally:

  ```bash
  cd cli
  gofmt -l .          # must produce no output
  go vet ./...        # must report no errors
  go build ./...      # must compile cleanly
  go test ./...       # all tests must pass
  golangci-lint run --timeout 5m ./...   # 0 issues
  ```

  If `golangci-lint` is not installed, run `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`.

## Git commits

- Never add `Co-authored-by` trailers or otherwise mark commits as co-authored.
- Preserve the repository's existing commit style and do not add AI attribution to commit messages.

## E2E tests (`test/e2e/`)

This repository includes a local Node.js E2E harness that exercises the CLI built from source and, for selected scenarios, a real AI agent.

### Main runner

- Command: `npm run test:e2e` (equivalent to `node ./test/e2e/run.mjs`).
- Useful options: `--scenario <id>` / `--scenarios <id1,id2>`, `--config <path>`, and `--timeout-ms <ms>`.
  - Example: `npm run test:e2e -- --scenario worktree-from-plan-to-implement-integrate`.
- The runner always builds the Go CLI into `test/e2e/.bin/archetipo` (`go build -o ... ./cmd/archetipo`), so Go must be installed.
- For every scenario, it creates a sandbox under `test/workspaces/<scenario>/runs/<timestamp>/sandbox`, copies the CLI into `sandbox/bin/`, sets `ARCHETIPO_DATA_DIR` to the repository root, and prepends `sandbox/bin` to `PATH`.
- Every run produces `report.html` and `summary.json` in its run directory. Generated workspaces and the E2E binary (`test/workspaces/*`, `test/e2e/.bin/`) are ignored and must not be committed.
- The default timeout is 20 minutes per step, with a heartbeat every 30 seconds for long-running steps.
- Authentication or credential errors are classified as `skip`; genuine failures and timeouts are classified as `fail`.

### `run.yaml` format

`test/e2e/run.yaml` contains two sections:

- `agents` defines executable backends with `tool`, `command`, `model`, `args`, and optional `env_required`.
  - `args` supports `{model}`, `{prompt}`, and `{sandboxDir}` interpolation.
  - Supported tools and installed skill roots are: `claude -> .claude/skills`, `codex -> .agents/skills`, `gemini -> .gemini/skills`, `opencode -> .opencode/skills`, `copilot -> .github/skills`, and `pi -> .pi/skills`.
- `scenarios` maps each scenario to an agent and may contain:
  - `fixture`: a directory overlaid onto the sandbox after `archetipo init`.
  - `prompts`: prompts or skills invoked through the agent; the skill name is derived from the `/...` prefix and is also used to verify that installation copied the skill.
  - `env_required`: overrides for the agent's environment requirements.
  - `archetipo_pre_commands`: CLI commands run before prompts.
  - `archetipo_post_commands`: CLI commands run after prompts.
  - `verify_integrate`: spec codes whose worktree integration must be verified. It captures the branch tip before post-commands, so it fits only scenarios where a post-command performs the integration.
  - `verify_spec_status`: a spec-code-to-status map asserted from `spec show` after the post-commands. Use it when the agent itself performs the final transition and no post-command exit code can carry the proof.
  - `verify_worktree_cleanup`: entries of `spec`, `branch`, and `worktree` asserting that an integration the agent already performed cleared the spec metadata and removed both branch and worktree. It is `verify_integrate`'s counterpart without the pre-integration tip comparison.
  - `verify_wiki_bootstrap`: expectations for core DDD pages, optional sources represented as `references/` concepts, `generated` state, issues, and targeted content; it also runs `wiki validate --profile bootstrap`.
  - `verify_review_wiki`: first commits configured fixture evidence, then seeds CLI approvals and commits only their page metadata plus Wiki `index.md`/`log.md`. It captures that seeded-review commit, requires every seeded page to be persisted reviewed, fresh, structurally valid, and free of `WIKI_EVIDENCE_CHANGED` before prompts, then verifies `archetipo-review` from machine effects: the exact expected persisted page set is reviewed, required-ready and explicitly reconfirmed affected-only pages appear in the dedicated approval commit, reconfirmed pages preserve their semantic content hash while advancing evidence review metadata and clearing `WIKI_EVIDENCE_CHANGED`, context-only references remain fresh and outside `wiki affected`, configured implementation artifacts have exact committed content, the spec reaches `DONE`, branch/worktree metadata and resources are removed, validation is clean, and the integrated checkout has no tracked or untracked Wiki changes.
  - Every known `verify_review_wiki` field is type-checked before the runner builds the CLI; all configured filesystem paths use one Windows/macOS/Linux-safe project-relative grammar. Unknown extension keys are retained for forward compatibility.
- Pre/post commands are split with `line.split(/\s+/)`. Avoid arguments that require complex shell quoting.

### Scenario execution sequence in `run.mjs`

1. Verify that `agent.command` exists and required `env_required` variables are present.
2. Run `archetipo init --tool <tool> --connector file --yes` in the sandbox as a non-interactive baseline.
3. Verify `.archetipo/config.yaml`, `.archetipo/shared-runtime.md`, and the skills required by the prompts.
4. Overlay the fixture, when configured. The fixture's `.archetipo/config.yaml` is authoritative and determines connector, worktree, paths, and related settings; do not add runner flags for these.
5. Initialize a Git repository in the sandbox on `main`, configure a local identity, stage only `seed_baseline_paths` when configured, and commit the generated fixture baseline. Installed skills and the copied binary remain untracked.
6. For focused review fixtures, approve `seed_reviewed_pages` only after their evidence is Git-tracked, stage only those reviewed page files plus Wiki `index.md`/`log.md`, commit the seeded-review baseline, and capture its exact hash in scenario context and reports.
7. Before any pre-command or prompt, run Wiki status and validation and require each seeded page to be persisted reviewed, fresh, structurally valid, and free of `WIKI_EVIDENCE_CHANGED`.
8. Run any `archetipo_pre_commands` using the CLI copied into the sandbox.
9. Run prompts through the agent with interpolated arguments.
10. For `verify_integrate`, capture the branch, worktree, and tip before post-commands using `spec show` and `git rev-parse`.
11. Run any `archetipo_post_commands`.
12. Verify integration: the spec is `DONE`, the pre-integration tip is reachable from `main`, the per-spec branch is deleted, and the worktree directory has been removed and no longer appears in `git worktree list --porcelain`.
13. Assert any `verify_spec_status` expectation from `spec show`, then any `verify_worktree_cleanup` entry: cleared `branch`/`worktree`/`fork_base` metadata, a deleted branch, and a worktree absent from both disk and `git worktree list --porcelain`.
14. For focused Wiki review scenarios, compare unchanged review metadata to the captured seeded-review baseline, verify acceptance from persisted/committed effects (including exact artifact content and exact approval commit paths), then assert branch/worktree cleanup directly without relying on a natural-language verdict.

### Current scenarios

- `inception-creates-valid-prd`: fixture `fixtures/inception`, prompt `/archetipo-inception`, then `validate prd`; verifies that the skill generates and persists a structurally valid PRD.
- `wiki-bootstrap-codebase-only`: fixture `fixtures/wiki-codebase`, prompt `/archetipo-wiki`; verifies a complete codebase-first DDD map without product documents or automatic approval.
- `wiki-bootstrap-prd-conflict`: fixture `fixtures/wiki-prd-conflict`, prompt `/archetipo-wiki`; verifies the `references/prd` concept, code authority for current state, and a conflict recorded as an issue.
- `from-prd-to-plan`: fixture `fixtures/prd`, prompts `/archetipo-spec` and `/archetipo-plan US-001`; covers PRD -> backlog/spec -> plan.
- `jira-init`: fixture `fixtures/jira-prd`, currently without prompts; uses the `jira` connector configuration.
- `from-plan-to-implement`: fixture `fixtures/plan`, prompt `/archetipo-implement US-001`; worktrees disabled.
- `worktree-from-plan-to-implement-integrate`: fixture `fixtures/worktree-plan`, prompt `/archetipo-implement US-001`, then `spec integrate US-001`; verifies integration.
- `worktree-implement-no-integrate`: fixture `fixtures/worktree-plan`, pre-command `spec start US-001`, then `/archetipo-implement US-001`; leaves the work unintegrated.
- `autopilot-worktree-full`: fixture `fixtures/worktree-plan`, prompt `/archetipo-autopilot US-001`; verifies the full autonomous run — plan, implement, autonomous acceptance — reaches `DONE` and leaves no branch or worktree behind.
- `autopilot-in-context-full`: fixture `fixtures/plan` (worktrees disabled), prompt `/archetipo-autopilot US-001`; verifies the same run reaches `DONE` through `spec move` rather than integration.
- `worktree-review-accepts-wiki`: creates a generated-evidence baseline commit, then a separate seeded-review metadata commit and verifies both seeded pages are fresh before prompting. It implements an exact greeting change with one required generated page, one affected-only tracked behavioral page, and one reviewed reference whose shared hub source is context; review approves the required page, explicitly reconfirms the verified-accurate affected-only page, preserves the fresh reference, commits exactly the accepted Wiki paths, integrates the spec, and removes its branch/worktree.

### Available fixtures

- `fixtures/inception`: `file` connector, worktrees disabled, and no initial PRD; used to verify generation through `/archetipo-inception`.
- `fixtures/wiki-codebase`: a small TypeScript/Express service without a PRD, including routes and tests; used for codebase-first Wiki bootstrap.
- `fixtures/wiki-prd-conflict`: a TypeScript/Express service with an intentionally inconsistent PRD (Python/FastAPI/MongoDB); used to verify conflict handling.
- `fixtures/prd`: `file` connector, worktrees disabled, and a `docs/PRD.md` for the match5 product.
- `fixtures/plan`: `file` connector, worktrees disabled, and backlog/spec/plan `US-001`, which requests `hello.txt` containing `Hello from ARchetipo`.
- `fixtures/worktree-plan`: equivalent to `plan`, but with `worktree.enabled: true`, `base: main`, `dir: .archetipo/worktrees`, and `branch_prefix: archetipo/`.
- `fixtures/worktree-wiki-review`: a worktree spec and plan with a required generated `overview`, a reviewed behavioral page tracking the changed greeting hub, and a reviewed PRD reference that marks that hub context; used to verify the reason-aware code + Wiki gate.
- `fixtures/jira-prd`: `jira` connector with `base_url: https://agilereloaded.atlassian.net/`, `story_type: Task`, `subtask_type: Sub-task`, and `priority_map`; `project_key` and `status_map` are intentionally omitted to let the CLI perform auto-discovery and auto-matching.

### Standalone smoke tests

- `npm run test:e2e:unit`: credential-free Node tests for the known-field/unknown-extension manifest contract and the two-commit focused Wiki baseline, including exact commit paths and pre-prompt freshness.
- `node ./test/e2e/validate-inception-smoke.mjs`: builds the CLI, initializes a file/pi sandbox, writes an invalid PRD, verifies `archetipo validate prd` exits with `0` and returns `kind=validation_result`, `data.ok=false`, `PRD_PLACEHOLDER_LEFT`, and `PRD_MISSING_SECTION`; then writes a valid PRD and verifies `kind=validation_result` with `data.ok=true`. Produces an HTML report. Options: `--workspace-root`, `--cleanup`. Note: the help text mentions `npm run test:validate-inception`, but no corresponding package script currently exists.
- `npm run test:view-delete-smoke`: builds the CLI, initializes a sandbox, adds two specs, seeds plan/review artifacts for `US-901`, starts `archetipo view` on a random port, and verifies through the HTTP API that `DELETE /api/spec/US-901` removes that card, retains `US-902`, subsequently returns 404 for `US-901`, and deletes its spec/plan/review artifacts.
- `npm run test:view-execution-smoke`: builds the CLI, initializes a sandbox with two specs (one left `TODO`, one moved to `PLANNED` by a real `spec plan`), starts `archetipo view` on a random port with a sentinel `ARCIPELAGO_TOKEN`, and verifies through the HTTP API that the provider list exposes every registered provider with its configurable fields and no credential — `arcipelago` with its five fields, and the local `codex` with the `spec.plan` capability, exactly `command`, `exec_args`, `model` and `timeout_seconds`, and the local `claude` with the same capability and exactly `command`, `model`, `print_args` and `timeout_seconds`, none of them with a field whose name asks for a credential — that each provider reports a boolean `available` and that an unavailable one states a non-empty `unavailable_reason` (the smoke observes availability instead of requiring it, so it passes with or without Codex and Claude Code installed, whether or not they are logged in, and never touches `PATH`), that a valid `PUT /api/execution/provider/default` reaches `.archetipo/config.yaml` and is reported afterwards, that an invalid one answers `400` naming the offending `field` while leaving both file and previous default untouched, and that the spec detail exposes exactly the actions the process Template admits — an empty list once the spec is moved to `DONE`.
- `npm run test:view-plan-smoke`: builds the CLI, initializes a sandbox with two `TODO` specs, starts a fake ARcipelago hub on `127.0.0.1` and `archetipo view` on a random port with a sentinel `ARCIPELAGO_TOKEN`, saves that hub as the workspace default execution provider, and verifies through the HTTP API that pressing the `plan` action starts exactly one execution (a second `POST /api/spec/US-901/execution` answers `409` naming the running one, and only one record file exists), that the non-terminal state is observable while the board keeps being served, that the run the hub really planned ends `SUCCEEDED` with the spec `PLANNED` and its plan readable, that the run the hub fails ends `FAILED` with the remote reason and leaves its spec `TODO` without a plan, and that a reload finds both executions again without starting a second one. Only the hub is fake: the viewer, the connector, the ARcipelago provider and its receipt verification are the real ones, and the smoke requires no credentials and no traffic outside `127.0.0.1`.
- `npm run test:view-run-smoke`: builds the CLI, initializes a sandbox with one `TODO` spec, starts a fake ARcipelago hub on `127.0.0.1` and `archetipo view` on a random port with a sentinel `ARCIPELAGO_TOKEN`, saves that hub as the workspace default execution provider, presses the `plan` action and then binds the remote task to a run, and verifies through the four run routes of the viewer that the ordered history is applied without visible duplication (a frame re-sent on the live stream appears once, and `after_id=<last_id>` is empty), that a broken stream is resumed from the cursor the projection already holds (the second subscription the hub records carries `afterId=4`, and the history stays gap-free), that a message is delivered to the hub and enters the timeline only once the hub republishes it, that a pending approval opened mid-run on a healthy stream is exposed with its options verbatim without any reconnection (the subscription count the hub records is unchanged) and its answer is recorded at the hub and re-read as no longer pending, that a cancel reports the state the hub confirms and a run the hub closed is never reopened by a retry (`409 run_not_active`), and that six refused commands (`404` and `409 run_not_active` on each of the three commands) leave the projection byte-identical. The credential travels to the hub — the event stream included — and never appears in any viewer response. Only the hub is fake: the viewer, the connector, the ARcipelago provider, its SSE consumer and the server-side run follower are the real ones, the smoke requires no credentials and no traffic outside `127.0.0.1`, and it contains no arbitrary sleep — the hub progresses only when the test commands it and every wait polls a viewer route. The four viewer routes it covers are:
  - `GET /api/execution/{id}/run?after_id=<n>`: the server-side projection of the run behind an execution — `run` (or `null` when the remote work has no run yet), the events after the cursor, `last_id`, the pending `approvals`, `connected`, `truncated` and an optional `notice`. The projection is maintained by one server-side follower per execution: it holds the single SSE consumer towards the hub, resumes it from its own high-water `id` after a drop, and re-reads the pending approvals on a timer, because the event stream carries no approval-bearing frame and an approval opened mid-run would otherwise stay invisible until the stream broke.
  - `POST /api/execution/{id}/run/messages` with `{"message":"…"}`: delivers an operator message and answers `202` with the projection, which deliberately does not yet contain the message.
  - `POST /api/execution/{id}/run/approvals/{approvalId}` with `{"option_id":"…"}`: answers one pending approval and returns `202` with the projection re-read after the answer.
  - `POST /api/execution/{id}/run/cancel`: asks the runner to close the session and returns `202` with the state the provider reports, never a locally derived terminal one.
  A refusal is rendered from the provider's reason and never from its message: `not_found` answers `404`, `run_not_active`, `runner_offline` and `unauthorized` answer `409`, and `unsupported` answers `400`.
- `npm run test:wiki-smoke`: builds the CLI, inspects a sandbox codebase, initializes the Wiki, creates ordinary and decision/reference pages, then verifies validation, unsafe-path errors, root-source affected matching, ADR search, cataloging, selective approval, tracked-versus-context freshness, explicit reconfirmation, distinct unreadable-evidence findings, and index regeneration. This credential-free smoke runs unchanged in the Ubuntu/macOS/Windows CI test matrix after Node setup.

When adding or modifying E2E tests, prefer explicit fixtures with a complete `.archetipo/config.yaml`, use `env_required` for external credentials, keep generated reports out of commits, and update this section whenever runner semantics change.

## Installation for end users

Primary path for any system with Node.js:

```bash
npm i -g @techreloaded/archetipo     # Global CLI in PATH
archetipo init [--tool …] [--connector …]
```

The Node shim in `npm/archetipo/bin/archetipo.js` resolves the binary package for the current platform, sets `ARCHETIPO_DATA_DIR`, and spawns the Go binary. Bundled skills live in `npm/archetipo/skills/` and are copied by `archetipo init` into `.{tool}/skills/` in the project.

## Operational notes

- `.archetipo/config.yaml` in this repository is a **template** copied into the target project as `.archetipo/config.yaml`.
- The `file` connector is the default and uses local Markdown files. The `github` connector requires an authenticated `gh` CLI.
- See the E2E tests section above for local E2E instructions.
