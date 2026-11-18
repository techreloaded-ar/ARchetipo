# ADR 0004: Isolated increment runtime in a local container

- Status: accepted and implemented for macOS; Linux is the automated CI verifier
- Date: 2026-11-16, accepted 2026-11-18

## Context

A business person may create increments on their own computer. Today an increment is a Git worktree: it separates working copies, not privileges. Three kinds of process run on the host with the person's full rights:

- the coding agent (`execution/localrun`, `execution/claude`, `execution/codex`);
- the project recipe, setup and checks (`verification`, `environmentrecipe`);
- the previewed application (`preview/process.go`).

Any of them can read the person's files and reach their local network. Agent-native sandboxes do not close this: they cover the agent's own tool calls, not the services a recipe starts, and their read restrictions are not the guarantee we need.

The requirement is that nothing started for an increment can read the person's files or reach their local network. Code that leaves the machine is not acceptable for every customer, so the first answer must be local.

## Decision

`archetipo view --isolated` runs the **whole** viewer — server, agent, recipe and preview — inside one Linux container, and leaves only the browser and a small launcher on the host. The flag is explicit in this version; without it the local mode is unchanged.

### Scope of this version

- **Hosts**: macOS with Docker Desktop. Linux is the automated CI verifier for the
  same Linux containers and network rules. Windows is not advertised as supported
  until the same boundary acceptance has run on Docker Desktop for Windows.
- **Applications**: web applications only. The preview is an HTTP origin in an iframe, as today.
- **Agents**: Claude and Codex, both in the image; the person chooses through the existing provider settings.
- **Code host**: GitHub, for clone, push and PR publication.
- **Backlog connectors**: `file`, `github` and `jira`. Jira Cloud works as is; a Jira on a private address is blocked by the network rule below and is out of scope.
- **Recipes**: a recipe that starts services itself through a container engine cannot prepare here, because no engine is available inside the environment, and reports that prerequisite. Services are declared and started by ARchetipo instead: here as processes from the runtimes installed in the image, today PostgreSQL 17 (ADR 0005).

### One boundary, not three

The viewer is not split. Routing the three spawn sites through a per-process runtime was considered and rejected: a site missed once breaks the guarantee silently, and native sessions would have to cross the boundary on every turn. With the viewer inside, every child process is inside by construction, and View keeps using the same standalone CLI contracts.

### Environment and workspaces

- An **environment** is one workspace container, one gateway, one network and one volume. In this version a person has **one shared environment** holding all their workspaces: the image is downloaded once and each credential is given once.
- A workspace remains **one repository**. An increment keeps one branch, one approved commit and one PR.
- A workspace is added from the viewer's home by giving the **repository URL**. The viewer offers this only where `ARCHETIPO_CLONE_DIR` is set, which the launcher sets to a directory of the volume. It clones over HTTPS, refuses URLs carrying credentials and never waits for a terminal prompt. A repository that is already a workspace is recorded at once; any other opens the ordinary creation form on the cloned directory, so connector and agents remain the person's choice. Adding an existing directory keeps working in local mode.
- A stopped or missing container engine is reported in one plain sentence.

To keep later extensions cheap:

- Every environment has an **identifier**. Container, network and volume names and the published port range derive from it; nothing is a fixed global name.
- The host launcher records environments. Each environment keeps its own known-workspaces registry inside its volume (`ARCHETIPO_STATE_DIR`), so a workspace belongs to exactly one environment by construction.
- Credentials are bound to an environment, never stored as a person-wide setting.
- The launcher talks to an **environment provider** interface (create, start, stop, publish ports, remove). Docker is its first implementation; Podman or a directly managed virtual machine can follow without touching what runs inside.

One environment per workspace then becomes a choice made when a project is added. Moving an existing workspace means publishing its pending commits and cloning it again.

Several repositories per workspace are out of scope. Their cost lies in the increment model (several branches and PRs under one approval), not in this boundary.

### First use

The person never configures the environment by hand. On the first run:

1. The launcher checks Docker, downloads the image showing progress, starts the environment and opens the browser.
2. The viewer opens on a guided first-use page, rendered from server metadata like every other view. It asks only for what the person's choices require:
   - the agent: Claude or Codex, then that agent's sign-in;
   - GitHub access;
   - Jira email and token, only when the workspace uses the Jira connector.
3. The person adds a project by repository URL; the viewer clones it and opens it.

In this version the page offers an **Accessi** dialog, from the home and from the topbar menu, only where the launcher declares `ARCHETIPO_RUNTIME=isolated`. The commands are `claude auth login --claudeai`, `codex login --device-auth` and `gh auth login --web` followed by `gh auth setup-git`, so Git uses the same GitHub access; their own status commands say whether each account is signed in. A link is shown only when its host belongs to that account. Cancelling stops the command's whole process tree, because a wrapper may leave a native child holding its output open. Choosing the agent is still done in the provider settings.

Each sign-in is shown as a card in the page: a link to open and, depending on the flow, a code to type there or a code to paste back. The viewer runs the agent's or GitHub's own sign-in command inside the environment and relays its prompts; it never asks for a password and never reads credentials from the host. Credentials land in the environment's volume. A credential that expires or is revoked brings back the same card.

### Files

- The launcher requests **no bind mount** from the host and never mounts the Docker socket.
- Each repository is **cloned inside** the environment's named volume, over HTTPS. Work leaves only through `git push` and the existing PR publication.
- The same volume holds the container user's home, so worktrees, `.archetipo/` records, the conversation journal and the harness's native session files stay durable and resumable across container restarts.
- The launcher never removes that volume implicitly. Removing it follows the same explicit keep/remove choice as increment deletion, because unpushed commits live there.
- `archetipo view stop --isolated` keeps the volume. The separate destructive
  command `archetipo view remove --isolated --yes` removes containers, network,
  volume and host record only after explicit confirmation.
- The container runs as a non-root user without `sudo`, with all capabilities dropped, `no-new-privileges` and no privileged mode.

### Network

Two containers share **one network namespace**:

- The **gateway container** starts first, on a dedicated Docker network, with `NET_ADMIN`. It installs the firewall and then only holds the namespace. It runs from the same image.
- The **workspace container** joins it with `--network container:<gateway>` and has no capabilities, so it can change neither rules nor routes.
- *Egress*: the internet is open, with no proxy to configure. New outbound connections to private, carrier-grade NAT and link-local ranges are rejected, which covers the LAN, the Docker host aliases and the engine's own bridge. Loopback and replies to established connections are allowed. IPv6 is disabled or given the equivalent rules.
- *Ingress*: the gateway publishes the viewer port and the preview port range on the host's `127.0.0.1` only. The workspace's listeners are reached directly, because the namespace is shared.
- The workspace is started only after the gateway reports its rules installed. If the gateway is recreated, the workspace is recreated with it.
- A **domain allowlist** is a later option for enterprise customers. It will belong to the launcher, on the host, never to the cloned repository, which the agent can edit.

An `internal` network with a proxying gateway was tried first and set aside. It gives a workspace with no route at all, which is stronger, but egress then depends on every tool honouring proxy variables, and routing through the gateway instead is dropped by the engine's own isolation rules.

### Viewer and preview ports

- The viewer already accepts `--host`; inside the container it binds a non-loopback address on a fixed port. It ships no authentication, so the gateway publishing on host loopback only is what keeps it single-user.
- The preview's browser origin (`preview/origin.go`) binds `127.0.0.1:0` today. It gains a configurable bind address and a configurable port range. The assigned port is still persisted in `browser-origin.json` and never silently replaced, so retained browser data keeps its origin.
- The gateway publishes each port of the range under the **same number**, so the URL the viewer already hands to the iframe is valid on the host. Ports are published when the gateway is created, hence a reserved range: 20 by default, configurable.
- The application's own upstream port (`resolvePort`) stays on container loopback.
- The size of the range bounds how many checkouts can preview at once; exhausting it is an explicit conflict.

### Credentials

Only named secrets enter, passed by the launcher as environment and never written to the host disk by it:

- the credential of the agent the person uses. The **subscription sign-in** is the primary way for both agents: it is done once inside the environment and kept in the volume, and it is how this version is tested. An **API key** stays possible as plain environment, and matters mainly for a later shared installation that several people reach;
- Git host access limited to the repositories the person granted;
- `JIRA_EMAIL` and `JIRA_API_TOKEN` when the workspace uses the Jira connector.

The launcher never copies `~/.claude`, `~/.codex`, SSH keys, the host's `gh` login or the keychain.

### Image

- One multi-architecture image (arm64, amd64) per release, on the same tag as the CLI and skills, published to GitHub Container Registry: Debian slim, CLI, skills, Git, `gh`, Node, both agent CLIs at pinned versions and a headless Chrome for browser checks.
- Agent CLIs are therefore updated with ARchetipo releases, not on their own.
- A project may name a derived image for its toolchain. That name is launcher-side configuration.

## Consequences

- Recipe, preview and agent code needs no isolation logic of its own. The product changes are the preview bind/range, adding a workspace from a URL, the launcher, the gateway and the image.
- The launcher is host-side orchestration only. It must not grow a second provisioner or approval policy: setup, checks, evidence and publication remain the CLI contracts running inside the container.
- `archetipo view list`, `stop` and `remove` on the host account for a viewer
  running in a container; `stop` retains its durable volume and `remove --yes`
  destroys it.
- The first start downloads the image and installs dependencies; later starts reuse the volume and the existing preparation fingerprints.
- Docker Desktop needs administrator rights, enabled virtualization (WSL2 or Hyper-V on Windows) and a paid licence for larger companies.
- `AGENTS.md` keeps stating that worktrees are not OS security isolation until this is delivered, then describes isolated mode with its limits.

### What this does not guarantee

- With the internet open, what is inside the environment — the cloned code and the credentials — **can be sent to any public address**. Only the person's files and local network are protected.
- Workspaces in the same environment are **not isolated from each other**: an agent working on one can read the code of the others and use the same Git host access.
- The network rule is a list of **address ranges**, not the absence of a route. A local network using public addresses is not covered, and names of internal hosts can still be resolved even though the hosts cannot be reached.
- The boundary towards the host is the engine's Linux VM and Apple's or Microsoft's hypervisor. ARchetipo adds no protection against an escape.
- Desktop and mobile applications have no preview. A virtual display for cross-platform toolkits, and Windows or macOS guest machines for native ones, are separate later work.
- It is not a remote runtime. A hosted sandbox remains the preferred later option for people who should install nothing.

## Verification

The credential-free `npm run test:view-isolated-smoke` builds the release image
from this checkout and proves from machine effects:

- the workspace receives no host-directory mount, cannot see the usual host roots
  and cannot change firewall rules or routes;
- a listener started on the host and private-range addresses are unreachable from
  the preview process, while a public host is reachable;
- a workspace added by repository URL is cloned inside the environment, and nothing is written on the host outside the launcher's state directory;
- the viewer and a preview iframe load from the host browser, and the browser origin survives Stop/Start;
- two increment previews keep their origins and work after Stop/Start;
- the destructive command removes containers, network, durable volume and host record.

The smoke runs automatically on Linux in CI and manually on each supported host
before a release. Agent adapters and sign-in parsing have their own credential-free
tests; completing subscription and GitHub sign-ins remains a manual release check
because CI must not hold personal credentials.

## Acceptance evidence

These decide whether the design stands as written. Windows remains unsupported
until the network checks are repeated there.

1. **Done on macOS (Docker 29.7.2, Apple Silicon), 2026-11-16.** With the shared namespace, a workspace without capabilities reached the internet with no proxy, could not reach a listener on the host through `host.docker.internal`, the host's LAN address or the engine's bridge, could not add a route or open a raw socket, and served a page to the host on `127.0.0.1` under the same port number while the host's LAN address refused it. A control container on a default network reached the same host listener.
2. **Done on macOS, same date.** On an `internal` network the proxy worked only for clients given the proxy variables, and routed egress through the gateway did not pass. This is why the shared namespace was chosen.
3. **Done on macOS, 2026-11-16**, Claude Code 2.1.325 and Codex 0.162.1 in a workspace container without capabilities, as a non-root user, behind the gateway rule:
   - Both subscription sign-ins run **without a terminal**, as a child process with piped input, so the viewer can drive them with no new dependency. Codex: `codex login --device-auth` (not listed in its help) prints a link and a code and completes by itself. Claude: `claude auth login --claudeai` prints a link and reads back on standard input the code the sign-in page shows.
   - Each agent answered with the subscription, then **resumed the same native session** after the container was destroyed and recreated on the same volume (`claude --resume <id>`, `codex exec resume <id>`), and recalled what it had been told. The network rule was still in force.
   - Resume went through each agent's CLI, not through ARchetipo's adapters (stream-json and app-server). They read the same session stores; this is confirmed when the viewer first runs inside the container.
   - A browser-side filter on the person's computer (here a VPN's threat protection) made the sign-in page time out in one browser while the same address answered from the terminal. The sign-in card must name the domain that has to open and suggest another browser.
4. **Done, 2026-11-16: the workspace root stays the repository root.** `ProjectRoot` is used in 44 non-test files, nine of which also run Git from it. `.archetipo/` (configuration, records, recipes, worktree directory) and the agents' skill directories live inside the repository, increment worktrees carry a committed copy of `.archetipo/`, and the agent finds its skills because its working directory is the checkout. Separating the two roots is a cross-cutting change, so this version clones each repository as the workspace itself, and several repositories per workspace will need a directory migration.

5. **Done on macOS, 2026-11-16: end to end with the prototype** in `docker/isolated/` (`verify.sh`). The real viewer and CLI run in the workspace container; two increments get previews on distinct ports of the published range, each opened from the host on `127.0.0.1` and served by its own checkout; a preview started through the viewer takes the next free port, because the stopped increments keep their reservations; the previewed application reaches the internet but not the host; both increments keep their browser origin after the containers are recreated on the same volume; the viewer page loads in the host's Chrome. The gateway is the hidden `archetipo view _gateway` and the environment is started and stopped by `archetipo view --isolated` and `archetipo view stop --isolated`, which keeps the volume. Adding a public repository by URL from the home, then initializing it through the creation form, was driven in the host's Chrome against the isolated viewer. In the same way the Accessi dialog started the three real sign-ins inside the container (Codex and GitHub showed their link and code, Claude its link and the paste field), kept a code being typed across polling, and cancelling left no sign-in process behind; completing a sign-in from the page needs a person and was not done. The agent adapters were not exercised inside the container.

## Open points

1. GitHub access with **limited repositories**. GitHub's CLI device flow is the simplest to drive but grants access to all of the person's repositories; a GitHub App limits it to the chosen ones but must be registered and maintained by ARchetipo.
2. How the launcher is installed by someone without Node on their computer.
3. Whether a workspace can require isolated mode so local mode is refused, and when isolated becomes the default.

## Delivery order

1. The verifications above.
2. Preview bind address and port range, with unit tests; local behaviour unchanged.
3. Image and gateway, started by hand, with the isolation smoke.
4. `archetipo view --isolated`: launcher, environment provider, environment identity, volume lifecycle, host registry.
5. Adding a workspace from a repository URL.
6. Credential onboarding for business people, then documentation and `AGENTS.md`.
