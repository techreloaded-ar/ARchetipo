# Reusable project verification

The CLI executes an explicit project recipe; skills decide its content and judge
whether it covers acceptance. View adds no environment implementation. There is
no agent worker engine or automatic choice of framework, database or credentials.

`archetipo execution environment configure --file -` atomically persists a JSON
recipe. `show` reads it. `prepare` runs setup when needed and probes readiness on
every reuse. `check <name> --spec US-001` prepares, then runs or reuses that named
check. `--fresh` forces the check, not reinstallation. `cleanup` runs the declared
owned cleanup commands and invalidates setup and checks before doing so.

```json
{
  "version": 1,
  "inputs": ["package.json", "package-lock.json", "prisma/schema.prisma", "test/support/setup.mjs"],
  "outputs": ["node_modules/.package-lock.json", "node_modules/.prisma/client/index.js"],
  "env_keys": ["NODE_OPTIONS"],
  "prepare": [{"name":"setup", "argv":["node","test/support/setup.mjs"], "timeout_seconds":120}],
  "probe": [{"name":"ready", "argv":["node","test/support/setup.mjs","status"], "timeout_seconds":5}],
  "cleanup": [{"name":"cleanup", "argv":["node","test/support/setup.mjs","stop"], "timeout_seconds":10}],
  "checks": {
    "functional": {
      "name":"functional", "argv":["npm","run","test:functional"], "timeout_seconds":120,
      "inputs":["src","test","package.json","package-lock.json","tsconfig.json"], "cache":false
    }
  }
}
```

The example requires project-owned scripts; it is not a generated universal Node
setup. Readiness must prove usable checkout-local dependencies and the identity
of any services. Setup receives `ARCHETIPO_ENVIRONMENT_ID` (stable per checkout),
`ARCHETIPO_ENVIRONMENT_ROOT` and `ARCHETIPO_ENVIRONMENT_STATE`. Use that identity
to label resources and refuse unknown owners. Service endpoints, synthetic
fixtures and cleanup belong in idempotent project tooling. Never store real
secrets in the recipe or argv. Environment input values are hashed, not recorded.
Declare every environment variable affecting validity in `env_keys`.

All recipe paths are project-relative; symlinks are rejected so a worktree cannot
silently certify a parent's installation. Inputs are files or directories, not
globs. Directory contents include new untracked files. Declare all relevant code,
tests, configuration, dependency and toolchain inputs; exclude generated output,
logs and runtime state. Time/network/external-state-dependent tests remain
`cache:false` unless that state is explicitly controlled and verified. Missing
inputs are fingerprinted as missing, so creating them invalidates evidence.
Glob syntax is rejected instead of treating patterns as missing literal files.
Project-relative executables are resolved from the recipe's checkout, including
when a viewer serves a different worktree from its own working directory. The
resolved tool paths and metadata are fingerprinted; unrelated PATH entries
injected by a native harness are not. Declare PATH in `env_keys` only if its full
value is itself a project input.

Every check records complete output, real exit status, command, duration and an
input fingerprint. Reports are unique and never overwritten. Readiness runs and
setup live under `.archetipo/environment/runs`; scoped check evidence uses the
configured `paths.test_results/<spec>/`. `archetipo/v1` envelopes have
`kind: verification_result`; show/configure return the recipe. Executions return
`passed`, `stable`, `reused`, `report_path` and steps. A reused check links its
original evidence through `reused_report`; it does not invent a fresh execution.

A successful command on changing inputs does not certify the current candidate:
`passed:false, stable:false`. Missing/altered evidence, a fresh failure, new source
or dependencies, changed toolchain/environment inputs, failed readiness and cleanup
invalidate reuse. Adding a check alone does not reinstall dependencies. A
`running` report left by a killed CLI is incomplete, never success.

Commands run without interactive stdin and with explicit timeouts. A small private
subprocess holds a control pipe: even hard-killing the CLI closes the pipe and
cancels the command tree. A second command ownership lock prevents recovery from
starting another writer before the previous tree has terminated. Deliberately
detached preview/services remain the project recipe's scoped cleanup responsibility.
Runtime approval policy remains in force; this is not automatic approval of
arbitrary project commands.

Errors: `E_PRECONDITION` for a missing recipe or corrupt checkpoint;
`E_INVALID_INPUT` for invalid recipe/name/path; `E_CONFLICT` for a live owner;
persistence errors prevent success. A normal envelope with `passed:false` is a
recorded failed verification, not a transport error. Repair the specific cause;
there is one bounded readiness repair, not an unlimited retry loop.

## Validation and performance

`npm run test:harness-benchmark` builds the real CLI and runs an owned local HTTP
service. It measures cold/warm paths, source and dependency invalidation, service
recovery, failed checks, concurrent calls, timeouts, hard process death, descendant
cleanup and restart. Each run preserves JSON/HTML reports and full evidence under
`test/workspaces/harness-benchmark/runs/`. The warm fixture budget is two seconds;
it measures deterministic overhead only. Real agent latency and application setup
are separate measurements, and require native-session trials as well.

For short native-session trials, `test/e2e/fixtures/harness-fast/` provides a
dependency-free HTTP catalog, its product document, baseline tests and
`verification.json`. Copy that fixture to a disposable empty directory, initialize
Git on `main`, run `archetipo init --tool codex --connector file --yes`, then
`archetipo execution environment configure --file verification.json`. Commit the
fixture, configured recipe and installed shared runtime before opening View.
Start an increment requesting bicycle trips alongside motorcycles, a shared
catalog, motorcycle as the default and rejection of unsupported vehicles. Select
the model in View and preserve the complete conversation journal and checkpoints.
This isolates controller behavior from framework installation; repeat the
registered application checks on a real project separately before drawing
conclusions about application delivery speed.
