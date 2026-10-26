# Technical review method

Assess a bounded delivery from the approved intent, selected stories, supplied
product context, applicable repository instructions (including `AGENTS.md`), the
combined diff and original evidence. Within the delivery checkout (`data.workdir`),
read `.archetipo/CUSTOM_INSTRUCTIONS.md` and `docs/DESIGN.md` when present. These
are exact, case-sensitive paths relative to that checkout; do not search for
namesakes or case variants elsewhere. Follow additional documents explicitly
referenced by these files. Their absence alone is not a blocker. Use the versioned
documents in this checkout, never uncommitted copies from another checkout.
Inputs include absolute project/code roots, review scope and independence choice,
explicit diff base, execution constraints, remaining correction budget and the
implementation handoff.
The caller owns approvals, checkpoints, budgets and publication. Inherit its
already-loaded shared runtime; a fresh reviewer reads it exactly once. Use the
workspace language for human-facing output. Do not invoke other delivery skills.

When an independent reviewer is required, use its own working context when the
harness supports it; do not inherit the discovery/implementation transcript
merely to obtain the review inputs. Read
the approved source stories, relevant product decisions, applicable instructions,
combined diff and original reports by reference. A concise implementer handoff is
an index, not evidence or a substitute for those sources. Reuse unchanged material
already read during this assessment; expand targeted reads for concrete concerns.
Return findings and original evidence paths rather than copying reports or full
source into the caller's conversation.

## Assess completeness and risk

Compare the combined behavior with the approved outcome, rather than reviewing
each story in isolation. Check relevant correctness, architecture, security and
regression risks; expand the scope if evidence reveals a concrete concern.
Read the implementer's original reports and confirm they cover the final inputs.
Confirm the shared fake-service rules: no test-mode switch, simulated branch or
bypass route in product code, fakes declared as resources, and every simulated
integration listed in the preview recipe. A violation is a blocker.
Add focused real checks where evidence is missing or a risk needs reproduction,
using the existing browser/API/database tools and owned environment. Independence
means a separate assessment, not unconditional repetition of every test.

Ask explicitly: can every actor complete their stated goal end to end? Is
essential information or an action missing between the implemented steps?
Inspect the underlying approved intent and supplied product context when needed.
A gap in the stories can also be a gap in delivery: identify the concrete goal
and missing behavior. If the intended requirement remains ambiguous, return the
question to the caller instead of inventing scope or silently accepting it.

Do not edit application code during review. Return all blocking findings together,
with reproduction, expected/observed behavior and affected stories. Distinguish
executed checks, code inspection and reused evidence; state any untested area.
No full criteria matrix, formal dossier, new setup tooling or complete suite
replay is required solely for review. Rechecks cover fixes, changed inputs and
connected flows; a later failure revokes earlier evidence for the same candidate.

When automatic Wiki is enabled, read the required-page list and affected claims.
Require generated, accurate, issue-free required pages. Run bootstrap validation
for an existing Wiki or required page changes; an absent Wiki with no documented
impact does not require creating one. Before acceptance, reread `wiki status` and
validate to detect changes;
approve only the exact required-ready IDs. Reconfirm evidence-changed pages only
after checking their claims remain accurate. Reread status to confirm approved
IDs are reviewed, validate the CLI-produced log and commit the exact accepted
page files plus generated index/log on the existing branch. Leave unrelated Wiki
changes untouched. Failed validation, approval or persistence blocks completion.
With the gate disabled, run no Wiki commands and introduce no Wiki blocker.

## Persist the assessment

For stories in REVIEW with sufficient implementation/evidence and no blocker,
run `spec move <code> --to done`. This method supports in-context delivery only;
if per-spec worktrees are enabled, return the configuration mismatch rather than
substituting a raw merge or silently bypassing integration.

For blockers that can be corrected within the caller's remaining budget, use
`spec request-changes <code> --file <feedback.json>` with
`{"comments":[{"body":"All findings: reproduction, expected, observed"}]}`;
`file` and `line` are optional real anchors. It moves REVIEW to TODO and persists
technical feedback. Do not require a new plan to address that feedback. If no
correction round remains, leave the story in REVIEW and return the blocker.
Never start a new run to reset the caller's budget. Reread `spec show <code>`
after transitions and verify their persisted effects.

Return the assessment, independence actually achieved, evidence reused/added,
blocking findings, residual limits and observed story states. DONE means technical
completion under this scope; it is neither proof of exhaustive independent tests
nor authorization to publish. Internal methods emit no standalone action receipt.

## CLI contract

Use `archetipo -C <project-root> config show` (`kind: setup`) and
`spec show <code>` (`kind: spec`, `data.spec`, `tasks`, `plan_body`, `workdir`).
`spec move` and `spec request-changes` return `kind: write_result` in the standard
`archetipo/v1` envelope. Persist workflow state only through the CLI, without
connector branches. Read warnings and the result before a dependent operation.

For additional verification use `execution environment show`,
`configure --file <recipe.json>`, `prepare` and
`check <name> --spec <code> [--fresh]` under the code root. Configure accepts the
version 1 recipe (`version`, `inputs`, `outputs`, `env_keys`, `prepare`, `probe`,
`cleanup`, `checks`; commands: `name`, `argv`, `timeout_seconds`; checks also:
`inputs`, `cache`). These return `kind: verification_result`; show/configure
return the recipe, prepare/check return `data.passed`, `stable`, `reused`,
`report_path`, `reused_report` and full step logs/exit codes. Require
`passed && stable`, follow reused evidence to its original report and use
`--fresh` only for a concrete need. Do not rebuild a healthy environment.

Conditional Wiki commands use `wiki --project-root <code-root>`:
`status [--require-generated <id>...]` returns `wiki_status`,
`validate --profile bootstrap` returns `validation_result` (`data.ok` required),
`approve <id>...` returns `wiki_approve_result` (`data.approved` must equal the
requested count), and `reconfirm <id>...` returns `wiki_reconfirm_result`.
Only the CLI writes review metadata and generated index/log files.

Handle `E_INVALID_INPUT`/`E_INVALID_USAGE`/`E_VALIDATION` by repairing input;
`E_NOT_FOUND`/`E_PRECONDITION` by resolving the exact prerequisite;
`E_CONFLICT` by rereading state or awaiting a live environment owner.
`E_CONNECTOR`/`E_NETWORK`/`E_AUTH_SCOPE` report external blockers;
`E_INTERNAL` or persistence failures block a completed assessment.
An envelope with `passed: false` remains a failed check.
