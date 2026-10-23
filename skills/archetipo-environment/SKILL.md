---
name: archetipo-environment
description: Configure, publish or repair the single reusable workspace environment recipe in a technical session, separate from product development.
---

# Workspace environment

Read `.archetipo/shared-runtime.md` exactly once. Work in the current execution
directory; do not assume View is running. Own setup choices, never product code,
backlog changes, intent approval or publication of product increments.

First read the existing environment recipe. If a valid common recipe already
exists and repair was not requested, run only preparation and retain its report;
do not regenerate configuration for another branch. Inspect existing project
scripts and dependency manifests before choosing missing setup.
Use one common workspace recipe. There are no branch overrides. Keep every
non-secret setup helper under `scripts/archetipo/`, with relative paths and local
synthetic data. Declare all persistent services through managed `resources` with
`start`, `probe`, `stop`, `stopped` commands, each with `name`, `argv` and
`timeout_seconds <= 30`; use named `ports`, `ARCHETIPO_PORT_<NAME>`, and verify
`ARCHETIPO_RESOURCE_OWNER` plus `ARCHETIPO_RESOURCE_NAME` before reuse/stop.
Never adopt unrelated services or copy credentials, installed dependencies,
resource ownership records or data from another checkout.

Call only these operations:

- `archetipo execution environment show`: `kind: verification_result`, recipe in
  `data`. Existing local projects work until their first common publication.
- `archetipo execution environment configure --file -`: stdin version 1 JSON
  with real `inputs`, `outputs`, optional `env_keys`, bounded idempotent `prepare`,
  readiness `probe`, optional `cleanup`, `checks`, `ports` and `resources`.
  Returns `kind: verification_result`, configured recipe in `data`.
- For a web application, `archetipo execution preview configure --file -`: stdin
  `{"version":1,"directory":".","argv":["scripts/archetipo/preview.sh","{port}"],"url":"http://127.0.0.1:{port}/","prepare_environment":true}`,
  adapted to the actual project. Bind strictly to the assigned loopback port.
  Returns `kind: preview_recipe`. Do not invent a web interface for a CLI/library.
- `archetipo execution environment publish --asset scripts/archetipo/<helper>`:
  repeat `--asset` for EVERY required non-secret helper and its helper dependencies.
  Returns `kind: environment_recipe` with the atomic common bundle in `data`.
  The CLI resolves the owning workspace even from an increment or ordinary
  worktree. No merge or manual write in the parent checkout is necessary.
- `archetipo execution environment prepare`: returns `kind: verification_result`
  with `data.passed`, `stable`, `reused`, `report_path`, `reused_report` and per-step
  log paths. Require `passed && stable`, inspect original evidence on reuse.

All envelopes use `archetipo/v1`. Branch on `error.code`: `E_PRECONDITION` means
missing configuration/prerequisite; `E_INVALID_INPUT` means fix the explicit
payload/path; `E_CONFLICT` means another operation owns the environment or a helper
has local changes, preserve its work; `E_INTERNAL` means persistence/runtime
failure. A successful envelope with `passed:false` is a failed preparation.
Report concrete missing services, credentials or application scaffolding instead
of claiming readiness. Follow the harness's own approval policy.

In View this skill runs in a separate durable technical native session. Setup
commands and logs belong there and in environment reports, never in the product
conversation. Do not launch the preview server: View continues automatically
through the same standalone preview contract after verifying persisted effects.
Standalone callers can subsequently use `archetipo execution preview run` (kind:
`preview`, foreground process until interrupted). End with a concise setup result.
