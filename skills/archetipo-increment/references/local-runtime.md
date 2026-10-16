# Local environment for an increment

These instructions apply only to workers executing a product increment, identified
by a non-null `data.increment` in `archetipo config show`. They do not change the
default behavior of standalone planning or implementation outside an increment.

Develop on the increment's dedicated branch and worktree, created from `main`.
Run the application, tests and preview locally using the project's tools and
available services. After human approval of the verified result, publication
opens or reuses one PR targeting `main`; never merge it automatically.

Preparing the environment needed to verify and demonstrate the approved increment
is part of delivery:

- Planning includes concrete setup tasks when app startup, local services, test
  data or test accounts are missing. Do not merely list these as external
  prerequisites. Preserve the approved product scope and existing acceptance
  requirements.
- Implementation executes the necessary setup, including when resuming a plan
  that omitted it. Handle missing local setup within the existing verification
  task and record its commands and evidence; do not rewrite the backlog or bypass
  the phase skill’s CLI operations. Reuse available local services
  without altering unrelated instances, create dedicated test databases/data and
  accounts, and choose ports that do not conflict with existing processes.
- Keep configuration and setup files in the increment worktree. Do not use
  production credentials or data, modify unrelated environments, or require
  global machine changes. If a necessary step requires privileges or information
  you do not have, report that specific blocker.
- Do not stop merely because the environment is not configured. First inspect the
  project's startup and authentication mechanisms, attempt the permitted local
  setup, verify service readiness, and run the required checks against that
  environment. Do not replace a required real browser/API/database flow with
  mocks or lower its acceptance gate.
- Record repeatable setup/test commands and the observed results. Keep track of
  services started for the increment; stop temporary test services when finished
  and identify any preview process intentionally left running for human review.
  If blocked, report what you attempted, the observed failure and the minimum
  action needed to proceed.
