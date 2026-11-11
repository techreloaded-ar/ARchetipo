# ARchetipo

English · [Italiano](README.it.md)

A product workspace for understanding a problem, building and trying a working
solution, learning from use, and handing a verified version to developers.

Chat and the running app are central. Enterprise backlog, stories, evolving
specifications, optional plans and documentation preserve project traceability.
They do not impose an implementation/review ceremony for every change.

## Try this branch

Development builds need Go and Node/npm. Local work environments need no cloud service.

```sh
npm run install:dev
cd /path/to/project
archetipo init --tool codex --connector file
archetipo view
```

This installs the current branch; the public npm package may contain an earlier
version. Configure Codex or Claude with native sessions and your model access for
the viewer chat. Other supported coding tools can invoke installed skills standalone.

An existing app must be Git-tracked. A new app can start from a workspace holding
only ARchetipo assets; an empty Git baseline is created without committing arbitrary
untracked files. Work starts from local `main`, or the committed current branch
when `main` is unavailable.

## Product work

Describe the problem in chat, or open an existing backlog story and start work
from it. A dedicated worktree, branch, environment and native conversation are
created. The agent maintains an evolving Markdown specification, builds the app
and records real verification evidence. Try the preview, chat about changes or
select an element and comment on it. Revisions reuse the same work environment.

The specification panel preserves versions, linked stories, observations and
decisions. Export its Markdown dossier for inspection outside ARchetipo. Explicit
business approval publishes the tested candidate as one PR. Developers may change,
rewrite, integrate and deploy it. The viewer does not merge or deploy.
An investigation can also conclude with a documented decision and no code change.

Stories stay in the original enterprise backlog. Story → work → PR references are
navigable in both directions; specification revisions retain original requirement
snapshots. Zero story points means unestimated. Publication and merge do not
silently change story statuses. The business-approved commit remains distinct
from later developer commits. Removing an environment retains the business dossier
and story references, alongside a Git recovery ref for committed work.

## Architecture and current limits

The CLI owns deterministic persistence, connectors, Git and resource ownership.
Skills own judgement and content; the viewer orchestrates and presents the same
contracts. Native conversations survive tab changes and runtime reconnections.

Local project recipes manage ports, services and test data. Git worktrees provide
working separation, not an operating-system security sandbox. The local baseline
requires neither Kubernetes nor Docker. On supported macOS hosts, `archetipo view
--isolated` instead runs the viewer, agents, recipes and previews inside a Docker
environment that cannot mount the person's files and blocks private-network
egress. Its release-matched multi-architecture image is published on GHCR. Stop it
without losing work with `archetipo view stop --isolated`; permanently delete its
workspace, credentials and unpushed work only with `archetipo view remove
--isolated --yes`. Windows host support remains unavailable until its dedicated
boundary acceptance has passed. Preview remains open during chat, edits and checks
explicitly declared safe beside the app. Setup, incompatible builds and cleanup
remain exclusive. Readiness and publication require intact machine reports for
every declared check, matching the current candidate.

The classic backlog path remains available and never goes through an increment:
a TODO story is planned with **Pianifica** (`archetipo-plan`), a planned story is
carried out with **Implementa** (`archetipo-implement`) directly in the project
checkout and ends in REVIEW. There **Rivedi** (`archetipo-review`) prepares the
acceptance dossier, and the person decides in the story's Revisione tab: approve
and close, or request changes and send it back to TODO with the feedback. A
per-story worktree is created only with `worktree.enabled: true`, and only then
does an approval integrate its branch; no PR is opened, and DONE is always the
person's decision.

This branch retires the `autopilot` and `design` skills and
internal increment delivery backlogs. Product
increments start after the person approves the current specification revision.
`init` removes those two managed skills when updating, preserving personal skills.
Historical work remains inspectable. Optional PRD, story planning and Wiki tools
remain available, alongside the artifact-management CLI.

See [product-work contract](docs/increments.md) and [repository instructions](AGENTS.md).
