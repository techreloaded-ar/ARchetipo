# One workspace execution recipe and a separate technical session

The workspace owns one reusable execution configuration. Branch overrides are
reserved for a future change and are not supported. `execution environment
publish --asset scripts/archetipo/<file>` atomically registers both explicit JSON
recipes and all named non-secret helpers in `.archetipo/environment-recipe.json`
at the owning checkout. Git's common directory resolves linked worktrees; an
independent workspace nested inside another repository keeps its own reference.
The bundle can be versioned, and is available to existing and future worktrees
before product changes are merged. `configure` keeps its immediate effect on an
already shared recipe; `publish` explicitly includes any new/changed helpers.
Legacy local recipes continue to work until the first common publication.

Execution materializes that reference under the current working directory.
There are no per-branch recipe overrides. Unknown local helper edits cause an
explicit conflict, never silent loss of working code. Runtime checkpoints,
installed dependencies, credentials, assigned ports, databases and reports are
never copied. Preparation fingerprints and original retained evidence govern
reuse; missing/modified evidence invalidates a cached preparation. Common browser
previews require a dynamic `{port}` and environment preparation. Chats on the same
checkout reuse its current preview, while distinct worktrees keep separate
servers, ports, services and data.

View prepares an increment after intent approval and before sending its
implementation turn. Native standalone story implementation uses the same gate
for its conversation's directory. Approval persists before setup, so a setup
failure does not revoke the person's decision. Standalone skills use the same CLI
contract and delegate missing setup to `archetipo-environment` in a separate
technical worker; they never require View.

View's technical configuration is a distinct durable native conversation, tagged
`purpose: environment`. It is absent from the ordinary chat rail and accessible
through Preview's environment details, including ordinary harness approvals and
input requests. Configuration and build commands never enter the development
conversation. The viewer invokes the same verification service as the CLI; it
adds orchestration and presentation, not setup guesses or automatic permissions.
A successful native turn alone is insufficient: the common recipe must exist and
preparation must pass with stable inputs before development continues.

One ordinary Preview button configures if necessary, prepares and starts the
current checkout. It never starts a server merely because a chat was opened.
The menu retains repair, technical details and stop; separate setup/preparation
buttons are removed. Operation state, technical session identity and preparation
reports survive reload. Cancellation stops the technical turn; a crash before
handoff closes an unsubmitted implementation execution without revoking intent
approval. A retry reconciles the technical session through native resume rather
than transcript seeding. Deletion waits for environment preparation ownership
and uses the existing cleanup/deletion contracts.

A fresh project without runnable scaffolding reports that concrete prerequisite;
it does not claim a prepared environment. Framework decisions and service choices
remain the skill's responsibility.
