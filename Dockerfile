# Dockerfile for archetipo-analytics — the standalone analytics ingest server.
#
# Multi-stage build: compile in golang, run on a tiny static image. The
# resulting image contains only the analytics-server binary + CA certs, so
# the attack surface and image size are minimal.
#
# Build:  docker build -t archetipo-analytics .
# Run:    docker run -p 8080:8080 -v $(pwd)/data:/data archetipo-analytics
#
# On Fly.io this image is built remotely by `fly deploy` and the /data
# volume is mounted from a persistent Fly volume (see fly.toml).

# ─── Build stage ───────────────────────────────────────────────────────
FROM golang:1.26 AS builder

WORKDIR /src

# Cache deps: copy go.mod/go.sum first and download.
COPY cli/go.mod cli/go.sum ./cli/
RUN cd cli && go mod download

# Copy the rest of the CLI module source.
COPY cli/ ./cli/

# Build the analytics-server binary as a static binary (no CGO). The pure-Go
# modernc.org/sqlite driver keeps CGO_ENABLED=0 viable.
ENV CGO_ENABLED=0 GOOS=linux
RUN cd cli && go build \
    -trimpath -ldflags="-s -w" \
    -o /out/archetipo-analytics \
    ./cmd/analytics-server

# Create the /data mount point owned by nonroot so the db file is writable
# even without a volume mounted (e.g. local docker run without -v).
RUN mkdir -p /out/data && chown 65532:65532 /out/data

# ─── Runtime stage ─────────────────────────────────────────────────────
# distroless/static: no shell, no package manager, ~2MB base. nonroot runs
# the binary as non-root user (uid 65532) for defense in depth.
FROM gcr.io/distroless/static-debian12:nonroot

# /data is the mount point for the persistent SQLite volume on Fly.
# Create it and mark it owned by nonroot so the db file is writable.
COPY --from=builder --chown=nonroot:nonroot /out/archetipo-analytics /archetipo-analytics
COPY --from=builder --chown=nonroot:nonroot /out/data /data

EXPOSE 8080

# Default args match the Fly deployment: listen on all interfaces, persist
# to /data/analytics.db. Overridable via `fly.toml` or `docker run` args.
ENTRYPOINT ["/archetipo-analytics"]
CMD ["--addr", "0.0.0.0:8080", "--db-path", "/data/analytics.db"]
