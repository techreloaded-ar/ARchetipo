---
name: archetipo-environment
description: Discover a project's real runtime, qualify a candidate recipe and publish or repair its reusable workspace environment in a separate technical session.
---

# Workspace environment

Read `.archetipo/shared-runtime.md` exactly once. Work in the current execution
directory. This skill works standalone and in View; never assume View is running.
Own setup and coverage choices, never product features, backlog or approvals.

## Discover before choosing

A fresh clone has no known stack or environment. Inspect its tracked manifests,
lockfiles, scripts, configuration, source, tests and documentation. Identify the
actual install, generation/migration, readiness, verification and launch commands.
Do not choose a framework, package manager, database or container technology from
the product name or a fixed catalogue. Read existing recipes before replacing one.
A healthy common recipe is reused, not regenerated for each branch.

Run `archetipo execution environment capabilities` INSIDE this session before
setup. The report observes loopback binding and temporary file creation in the
checkout/common workspace. It distinguishes available, unavailable and unknown;
it does not certify network, Docker, credentials or all filesystem operations.
The View process's access is never proof of the agent's access. The selected
permission profile is configuration, not an observation of effective capabilities.

After inspecting the project, repeat capabilities with `--tool <executable>` for
each actual tool required. Check its version, daemon access and any required
endpoint with explicit non-destructive `--probe-file -` commands. An executable
being found does not mean its daemon is usable. Never dump environment values,
read another checkout's secrets, or store credentials in arguments/logs. Missing
capabilities use the harness's normal approval policy; never change the policy,
replace the runtime, bypass sandbox restrictions, or silently retry a forbidden
operation. If it cannot be approved, report the exact missing capability and end
the turn so the person can adjust the technical session and resume it.

Record the inspected files, chosen commands, observed capability report, coverage
rationale and missing prerequisites in the technical summary. Do not invent a
working recipe if application scaffolding or required services are absent.

## Build a candidate, preserve the common revision

Use `environment configure --candidate` and, for an actual web app,
`preview configure --candidate`. These write local candidate files without
updating the shared bundle or acquiring a write lock in the parent checkout.
Normal preparation continues to use the common revision until publication.
During candidate testing always use `--candidate`; otherwise common sync may
reject the edited helpers or restore the previous environment JSON.

Use locked, reproducible installation. Never silently fall back from a failed
locked install to unlocked resolution, swallow an exit status, hide an inconsistent
lockfile or change dependency versions just to make setup pass. Repair an
inconsistent manifest/lockfile explicitly in the working branch, preserve intended
versions and record the diff. No installed dependencies from other checkouts.
When outputs already exist, test installation in a separate explicitly owned
scratch checkout with the relevant manifests and helpers; do not delete existing
user installations to obtain a clean test. Qualification alone does not prove a
pristine installation or semantic preservation of application data.

Keep all non-secret helpers under `scripts/archetipo/`. Declare the complete
helper dependencies explicitly as assets; include them and all setup manifests,
lockfiles and migrations in setup inputs. Checks include their complete code,
test and configuration inputs. Output paths must be real checkout-local files,
not package-manager executable symlinks. Prefer installed tools over commands
that silently download missing tools.
Inputs and outputs name explicit files or directories; glob patterns such as
`*.py` or `tests/**` are rejected. List root files explicitly and use directory
names for recursive coverage, excluding generated caches by choosing narrower
directories or explicit files. The CLI fingerprints resolved executable paths;
unrelated harness PATH entries do not invalidate evidence. Declare additional
environment keys only when the project's behavior actually depends on them.

Register persistent dependencies through managed resources, never a detached
shell or an unrelated service. Each lifecycle command verifies
`ARCHETIPO_RESOURCE_OWNER` and `ARCHETIPO_RESOURCE_NAME`; named ports come from
`ARCHETIPO_PORT_<NAME>`. Distinguish a denied/unreachable daemon from an absent
resource. Verify the owned endpoint as well as identity before reuse or stop.

Preview launchers run in the foreground for the server's whole lifetime. Do not
daemonize, detach, use `nohup`, background the app or create a new process session.
Prefer `exec` in shell launchers; programmatic wrappers wait for the child and
propagate its exit status. The CLI owns the foreground process tree and stops
remaining children when the launcher exits; persistent dependencies belong only
to registered resources with explicit stop and stopped probes.

Readiness must exercise the actual requirements: e.g. usable generated artifacts,
reachable owned dependencies and compatible schema. A successful version command
or the existence of a dependency directory alone is insufficient for a database
application. A CLI or library need not invent a web service or database.

Preparation, migration and seeding must be idempotent and conservative. Add missing
synthetic fixtures, preserve existing rows and the person's edits, and never reset
data at each revision. Cleanup stops only declared owned resources and retains
required demonstration data. Verify stop and subsequent restart against explicit
project probes, plus fixture preservation checks where state exists. Destructive
resets require an explicit request; test destructive scenarios on dedicated owned
synthetic resources. Do not copy resource journals, checkpoints or secrets.

## Qualify, publish, verify

1. Run `environment qualify` with `--asset` repeated for EVERY helper and its
   dependencies. It executes two real preparations, then a warm preparation that
   must reuse the second report and rerun readiness. Require `data.passed: true`;
   read every original report/log, including logs of commands returning zero.
   A successful wrapper can still hide a failed child command. Stable inputs and
   exit codes are machine evidence, not a semantic audit of arbitrary scripts.
2. Repair concrete failures and retry the candidate, at most three attempts per
   unchanged cause. Retain reports and identify permission/prerequisite failures
   separately from setup defects and product defects. Never weaken checks to
   obtain a pass. Ask for a missing decision and end the turn when needed.
3. Publish with `environment publish --qualified` and exactly the same assets.
   Publication requires current retained qualification evidence and atomically
   adopts the exact bundle. A changed helper, recipe, declared setup input or
   missing/tampered log requires requalification. Publish only non-secret setup
   helpers and environment/preview configuration, never product code.
4. Run every project check through `environment check <name>` and retain its
   reports (or `check <name> --candidate` before publication). Select coverage from
   the actual product: applicable compilation/static checks, tests and meaningful
   runtime/persistence/integration checks. Explain absent coverage. Setup
   qualification does not certify the product; all declared checks remain required
   for a product candidate. Check-specific runtime modes must follow the actual runner contract. Do not
   leak the preview's development mode, credentials or live endpoints into test
   commands. Validate the test runner's effective mode and mock configuration;
   isolate external side effects using explicit local doubles or owned services.
   A setup mistake in the test launcher is not a product defect.
   Existing product failures remain visible and must not
   cause an endless rewrite of a working recipe.

`during_preview: true` promises no dependency changes, shared build-output writes
or destructive data changes while the app runs. Leave it false otherwise. Tests'
scratch outputs stay separate from preview output. Do not enable it to bypass an
ownership conflict. Reuse healthy services and demo data across revision rounds.

## CLI contract

All envelopes use `schema: archetipo/v1`; success carries `kind` and `data`,
failure carries `kind: error` and `error.code`.

- `archetipo execution environment capabilities [--tool <executable> ...]
  [--probe-file -]`: `kind: environment_capabilities`. Optional stdin is an array
  of `{ "name":"tool-version", "argv":["<actual-tool>","<version-argument>"],
  "timeout_seconds":10 }`. Names must be safe and unique; timeout <= 30 seconds.
  Data includes directories, observed write/loopback capabilities, tool paths,
  external network `unknown`, per-probe reports and `report_path`. Inspect each
  probe's `passed`; a successful envelope does not imply every probe passed.
- `archetipo execution environment show`: `kind: verification_result`, effective
  recipe in `data`. Missing common publication retains legacy local recipes.
- `archetipo execution environment configure --candidate --file -`: stdin version
  1 JSON with real `inputs`, `outputs`, optional `env_keys`, bounded idempotent
  `prepare`, readiness `probe`, optional `cleanup`, `checks`, `ports`, `resources`.
  Each command has `name`, explicit `argv`, `timeout_seconds` (1..1800); each
  resource has `name`, `start`, `probe`, `stop`, `stopped` (timeouts <= 30).
  Checks have command fields, explicit `inputs`, `cache`, optional `during_preview`.
  `prepare`, `probe` and `cleanup` are ARRAYS of commands; `checks` is an OBJECT
  keyed by check name, never an array. For example (replace commands and inputs
  with those discovered in the actual project):

  ```json
  {"version":1,"inputs":["package.json"],"outputs":[],"prepare":[],"probe":[{"name":"runtime","argv":["node","--version"],"timeout_seconds":10}],"checks":{"tests":{"name":"tests","argv":["npm","test"],"timeout_seconds":120,"inputs":["package.json","src","test"],"cache":true,"during_preview":false}}}
  ```

  This is a shape example, not a default recipe or sufficient application
  readiness probe. Require a successful envelope before qualification; failed
  configuration leaves no new candidate. Repair the payload before continuing.
  Returns `kind: verification_result`, configured local recipe in `data`.
- `archetipo execution preview configure --candidate --file -`: for web apps only,
  stdin `{ "version":1, "directory":".", "argv":["scripts/archetipo/preview.sh",
  "{port}"], "url":"http://127.0.0.1:{port}/", "prepare_environment":true }`,
  adapted to the actual product. Returns `kind: preview_recipe`. Bind strictly
  to the assigned loopback port. Do not launch the preview server in this skill.
- `archetipo execution environment qualify --asset scripts/archetipo/<file> ...`:
  `kind: environment_qualification`, data `passed`, `bundle_digest`, `input_key`,
  three original preparation `reports` and `report_path`. It does not certify a
  clean dependency install, product checks or data preservation by itself.
- `archetipo execution environment publish --qualified --asset
  scripts/archetipo/<file> ...`: `kind: environment_recipe`, atomic common bundle
  in `data`. Workspace resolution is through Git, including from a worktree.
  A denied parent write requires harness approval, not manual edits elsewhere.
- `archetipo execution environment prepare [--candidate] [--fresh]`:
  `kind: verification_result`, `passed`, `stable`, `reused`, `report_path`,
  `reused_report` and per-step log paths. Fresh executes preparation again.
- `archetipo execution environment check <name> [--candidate] [--fresh]`:
  `kind: verification_result` with the same evidence fields. A `passed:false`
  result is a failure even when the envelope/command succeeds.
- `archetipo execution environment cleanup`: `kind: verification_result`,
  executes registered stop adapters. For stop/restart verification, publish the
  qualified bundle first, stop only its resources, then prepare it again and
  inspect real readiness and fixture-preservation checks.

Branch on `error.code`: `E_PRECONDITION` means missing/stale qualification,
configuration or prerequisite; `E_INVALID_INPUT` means fix the explicit payload
or paths; `E_CONFLICT` means another owner, changed candidate or helper conflict,
preserve its work; `E_INTERNAL` means persistence/runtime failure. Read the
concrete original error/log to distinguish permission problems; do not branch on
connector type or retry the same denied operation.

In View, setup stays in the separate durable technical session. View verifies
qualified publication, prepares and starts preview through the standalone CLI
contracts. Standalone users can subsequently invoke
`archetipo execution preview run` (`kind: preview`, foreground process).
End with the setup result, original report references, coverage and unresolved
blocks. Never call a product ready merely because its environment was prepared.
