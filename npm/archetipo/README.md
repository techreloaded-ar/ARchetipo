# ARchetipo

A product workspace for chat, working app previews and enterprise traceability.
Business users explore problems and test working solutions; developers receive a
verified candidate through a pull request and decide how to integrate and deploy it.

## Try the isolated environment preview

Start Docker Desktop on macOS or Docker Engine on Linux. Then run these commands
from any directory.

```sh
npm install -g @techreloaded/archetipo@preview
archetipo view --isolated
```

ARchetipo downloads the public Docker image for the installed version from GitHub
Container Registry if it is missing, starts the environment and opens the viewer
in your browser. The first download may take a few minutes. Add a Git repository
by URL from the viewer or create a workspace there. Run `archetipo view stop
--isolated` to stop the environment while keeping its work in the Docker volume.

## Start in a local project

```sh
npm install -g @techreloaded/archetipo
cd my-project
archetipo init --tool codex --connector file
archetipo view
```

Configure Codex or Claude native sessions in the viewer. Optional backlog, product
documents, story planning and Wiki tools preserve project history. The CLI manages
artifacts, Git worktrees, local services and evidence; skills supply judgement.

In local mode, Git worktrees provide separate working directories and share the
host's security boundary. Preview may pause while the development turn uses the
environment.

See the repository README for the current branch, development builds, compatibility
changes and the complete product-work contract.
