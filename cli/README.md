# archetipo CLI

Deterministic Go implementation of the ARchetipo workflow operations. Replaces markdown-described connector behavior with one binary that performs the public backlog, story, task, PRD, and board operations directly.

## Build

```bash
cd cli
go build ./cmd/archetipo
```

The output binary `archetipo` reads `.archetipo/config.yaml` from the project root (or any ancestor) to choose the connector (`file` or `github`) and execute the requested sub-command.

To build all release binaries locally from the repository root:

```bash
npm run build:cli
```

## Layout

```
cmd/archetipo/        # entry point
internal/
  cli/                # cobra sub-commands (one file per operation)
  connector/          # interface, registry, two implementations + inmemory ref
    filefs/           # markdown + HTML-comment markers
    github/           # gh CLI shell-out + GraphQL aliased mutations
    inmemory/         # reference impl used by the conformance suite
    conformance/      # behavioural test suite shared by every implementation
  config/             # .archetipo/config.yaml loader
  domain/             # canonical data types
  iox/                # JSON envelope on stdin/stdout/stderr + typed errors
  version/            # injected via -ldflags at release time
```

## Tests

```bash
go test ./...
```

The conformance suite runs against `filefs` and `inmemory`. The `github` connector is exercised with a mock `gh` runner; live smoke tests are gated behind `ARCHETIPO_E2E_GH=1` and need a sandbox repo with `gh` authenticated.

## Distribution

Tags `vX.Y.Z` produce a single bare binary per platform via GoReleaser. Release assets are named `archetipo-<os>-<arch>` for macOS/Linux and `archetipo-windows-<arch>.exe` for Windows, then downloaded by `install.sh` / `install.ps1` into `.archetipo/bin/` of the target project. GoReleaser appends the Windows executable extension automatically for `binary` archives.

The release asset names are platform-specific, but the installed command path is stable for skills:

- macOS/Linux: `.archetipo/bin/archetipo`
- Windows: `.archetipo/bin/archetipo.exe` plus `.archetipo/bin/archetipo.cmd` shim
