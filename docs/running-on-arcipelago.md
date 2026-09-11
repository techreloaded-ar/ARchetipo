# Running ARchetipo on an ARcipelago fleet

A runner is a general-purpose ARcipelago machine. Nothing about ARchetipo is
baked into its image, and that is on purpose: the image is released and rebuilt
on ARcipelago's clock, while ARchetipo changes several times a day. What a
runner needs from us arrives afterwards, the way a package does.

Three things have to be true on a machine before it can plan or implement a
spec, and they have three different owners:

| What | Who provides it | Changes when |
|---|---|---|
| the tooling — the `archetipo` CLI and the skills | `npm run install:runners`, from this checkout | ARchetipo is rebuilt |
| the working directory — a checkout of the project | the compose overlay, `npm run bundle:runner -- --compose` | the project being worked on changes |
| the artifacts — backlog, specs, plans | the connector configured in the project | every spec transition |

Each is declared to the hub as a capability, so a machine that lacks one is
refused at once, naming what is missing, instead of being handed work it cannot
do and leaving it queued until the caller's own timeout expires.

## Once: point the fleet at a checkout

From this repository:

```bash
npm run bundle:runner -- --project /abs/path/to/your-project --compose
docker compose -f provisioning/docker-compose.dev.yml -f <printed overlay> up -d
```

The overlay carries one thing: the bind mount of the project, with the path
already resolved on this machine — Docker silently *creates* a missing bind
source as a root-owned empty directory, so resolving here means a wrong path
fails now rather than inside a container minutes later. It declares that mount
as `project:<slug>`.

## Every time: install the current build

```bash
npm run install:runners
```

It finds the running runners of the compose project, asks each one which
architecture it is, cross-compiles the CLI for it, and installs into every
container:

- `/opt/archetipo/` — the binary, the skills, the runtime assets
- `/usr/local/bin/archetipo` — a wrapper that sets `ARCHETIPO_DATA_DIR`, so the
  CLI is on the PATH of every session without the fleet declaring a variable
- `$HOME/.pi/agent/skills` — a symlink to the bundle's skills, so the engine
  finds them for every session and no project has to install them for one
  particular tool
- `$HOME/.arcipelago/runner.json` — the capabilities `archetipo` and
  `archetipo:<version>`

Then it restarts the containers, because a runner reads its capabilities once,
at startup, and verifies the result by asking each machine for its
`archetipo version` rather than by reporting what it asked Docker to do.

Two tags and not one, because they answer different questions. `archetipo` is
what a workspace requires, and it has to stay the same string across every
rebuild or the requirement would need rewriting on every commit.
`archetipo:<version>` is what makes a fleet running yesterday's build visible as
such, instead of failing in a way that looks like a bug in a skill.

Everything the installer writes lives in the container filesystem, not in a
volume, and that is the deliberate trade: the tooling and the claim about the
tooling disappear together when a container is recreated, so the two can never
disagree. After a `docker compose up` that recreates a runner, run it again —
it takes seconds.

## Then: state what the work needs

```bash
arcipelago workspaces update <workspace> \
  --cwd /workspace \
  --requires archetipo,project:<slug> \
  --model <provider/modelId>
```

`--model` matters for the same kind of reason the capabilities do. A task that
names no model leaves the engine to take the first provider it finds credentials
for, and "first" is not "working": a provider whose OAuth token can no longer be
refreshed is still the first one found, and every run dies in half a second.
Naming it on the workspace makes which model does the work a decision somebody
made.

And in the project:

```bash
export ARCIPELAGO_TOKEN='arc_app_…'
archetipo execution setup --url <hub>
archetipo doctor
```

`doctor` reads `GET /api/external/workspaces` and reports, before anything is
dispatched, whether any known machine could take the work and which capability
nothing advertises.

## What this does not do yet

The runner still works in a directory somebody mounted, which is why
`project:<slug>` exists at all. The coherent end state is a workspace that
declares `repo` and `ref` and a runner that materialises its own checkout per
task — ARchetipo already has the seam for it, in the per-spec branch and
worktree. That step needs git credentials distributed to the runners, which is
the genuinely hard part, and it is tracked separately.

Until then: a `file` connector with remote execution means the runner and the
person share one working copy on one machine. For anything else, use `github`
or `jira`, where the source of truth is on the network and the shared-disk
problem does not exist. See
[`docs/wiki/decisions/remote-plan-ownership.md`](wiki/decisions/remote-plan-ownership.md).
