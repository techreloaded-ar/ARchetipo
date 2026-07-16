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

- The 13 public CLI operations are stable. Any incompatible change is a breaking change and must be versioned accordingly.
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
  - `verify_integrate`: spec codes whose worktree integration must be verified.
  - `verify_wiki_bootstrap`: expectations for core DDD pages, optional sources represented as `references/` concepts, `generated` state, issues, and targeted content; it also runs `wiki validate --profile bootstrap`.
  - `verify_review_wiki`: verifies that `archetipo-review` moves the spec to `DONE`, presents dossier pages, commits only the expected pages as `reviewed` together with `index.md` and a `log.md` containing a `Review` entry, and leaves no tracked or untracked Wiki changes in the integrated checkout.
- Pre/post commands are split with `line.split(/\s+/)`. Avoid arguments that require complex shell quoting.

### Scenario execution sequence in `run.mjs`

1. Verify that `agent.command` exists and required `env_required` variables are present.
2. Run `archetipo init --tool <tool> --connector file --yes` in the sandbox as a non-interactive baseline.
3. Verify `.archetipo/config.yaml`, `.archetipo/shared-runtime.md`, and the skills required by the prompts.
4. Overlay the fixture, when configured. The fixture's `.archetipo/config.yaml` is authoritative and determines connector, worktree, paths, and related settings; do not add runner flags for these.
5. Initialize a Git repository in the sandbox on `main`, configure a local identity, and create an empty base commit.
6. Run any `archetipo_pre_commands` using the CLI copied into the sandbox.
7. Run prompts through the agent with interpolated arguments.
8. For `verify_integrate`, capture the branch, worktree, and tip before post-commands using `spec show` and `git rev-parse`.
9. Run any `archetipo_post_commands`.
10. Verify integration: the spec is `DONE`, the pre-integration tip is reachable from `main`, the per-spec branch is deleted, and the worktree directory has been removed and no longer appears in `git worktree list --porcelain`.

### Current scenarios

- `inception-creates-valid-prd`: fixture `fixtures/inception`, prompt `/archetipo-inception`, then `validate prd`; verifies that the skill generates and persists a structurally valid PRD.
- `wiki-bootstrap-codebase-only`: fixture `fixtures/wiki-codebase`, prompt `/archetipo-wiki`; verifies a complete codebase-first DDD map without product documents or automatic approval.
- `wiki-bootstrap-prd-conflict`: fixture `fixtures/wiki-prd-conflict`, prompt `/archetipo-wiki`; verifies the `references/prd` concept, code authority for current state, and a conflict recorded as an issue.
- `from-prd-to-plan`: fixture `fixtures/prd`, prompts `/archetipo-spec` and `/archetipo-plan US-001`; covers PRD -> backlog/spec -> plan.
- `jira-init`: fixture `fixtures/jira-prd`, currently without prompts; uses the `jira` connector configuration.
- `from-plan-to-implement`: fixture `fixtures/plan`, prompt `/archetipo-implement US-001`; worktrees disabled.
- `worktree-from-plan-to-implement-integrate`: fixture `fixtures/worktree-plan`, prompt `/archetipo-implement US-001`, then `spec integrate US-001`; verifies integration.
- `worktree-implement-no-integrate`: fixture `fixtures/worktree-plan`, pre-command `spec start US-001`, then `/archetipo-implement US-001`; leaves the work unintegrated.
- `worktree-review-accepts-wiki`: implements a change with `Wiki Impact`, then `/archetipo-review` presents and approves the generated page, commits review metadata in the worktree, and integrates the spec.

### Available fixtures

- `fixtures/inception`: `file` connector, worktrees disabled, and no initial PRD; used to verify generation through `/archetipo-inception`.
- `fixtures/wiki-codebase`: a small TypeScript/Express service without a PRD, including routes and tests; used for codebase-first Wiki bootstrap.
- `fixtures/wiki-prd-conflict`: a TypeScript/Express service with an intentionally inconsistent PRD (Python/FastAPI/MongoDB); used to verify conflict handling.
- `fixtures/prd`: `file` connector, worktrees disabled, and a `docs/PRD.md` for the match5 product.
- `fixtures/plan`: `file` connector, worktrees disabled, and backlog/spec/plan `US-001`, which requests `hello.txt` containing `Hello from ARchetipo`.
- `fixtures/worktree-plan`: equivalent to `plan`, but with `worktree.enabled: true`, `base: main`, `dir: .archetipo/worktrees`, and `branch_prefix: archetipo/`.
- `fixtures/worktree-wiki-review`: a worktree spec and plan with a generated `overview` page declared in `Wiki Impact`; used to verify the combined code + Wiki gate.
- `fixtures/jira-prd`: `jira` connector with `base_url: https://agilereloaded.atlassian.net/`, `story_type: Task`, `subtask_type: Sub-task`, and `priority_map`; `project_key` and `status_map` are intentionally omitted to let the CLI perform auto-discovery and auto-matching.

### Standalone smoke tests

- `node ./test/e2e/validate-inception-smoke.mjs`: builds the CLI, initializes a file/pi sandbox, writes an invalid PRD, verifies `archetipo validate prd` exits with `0` and returns `kind=validation_result`, `data.ok=false`, `PRD_PLACEHOLDER_LEFT`, and `PRD_MISSING_SECTION`; then writes a valid PRD and verifies `kind=validation_result` with `data.ok=true`. Produces an HTML report. Options: `--workspace-root`, `--cleanup`. Note: the help text mentions `npm run test:validate-inception`, but no corresponding package script currently exists.
- `npm run test:view-delete-smoke`: builds the CLI, initializes a sandbox, adds two specs, seeds plan/review artifacts for `US-901`, starts `archetipo view` on a random port, and verifies through the HTTP API that `DELETE /api/spec/US-901` removes that card, retains `US-902`, subsequently returns 404 for `US-901`, and deletes its spec/plan/review artifacts.
- `npm run test:wiki-smoke`: builds the CLI, inspects a sandbox codebase, initializes the Wiki, creates ordinary `generated` pages and a `type: decision` page, then verifies validation, ADR search by type, catalog generation, selective approval to `reviewed`, and `index.md` regeneration.

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
