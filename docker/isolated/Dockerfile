# Image of the isolated increment runtime (ADR 0004). One image serves both
# containers of an environment: the gateway, which owns the network namespace
# and its firewall, and the workspace, which runs the viewer, the agents, the
# recipes and the previews without capabilities.
#
# Build from the repository root:
#   docker build -f docker/isolated/Dockerfile -t archetipo-isolated .
#
# The host starts it with `archetipo view --isolated`; the gateway container
# runs `archetipo view _gateway`, the workspace container the default command.

FROM golang:1.26-bookworm AS cli
WORKDIR /src
COPY cli/go.mod cli/go.sum ./
RUN go mod download
COPY cli/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/archetipo ./cmd/archetipo

FROM node:22-bookworm-slim
ARG CLAUDE_CODE_VERSION=2.1.325
ARG CODEX_VERSION=0.162.1
ARG GH_VERSION=2.63.2
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates curl iptables iproute2 procps chromium \
 && rm -rf /var/lib/apt/lists/*
# The distribution's GitHub CLI is too old for the browser sign-in the viewer
# relays; the official release is pinned like the agents.
RUN arch="$(dpkg --print-architecture)" \
 && curl -fsSL -o /tmp/gh.deb "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${arch}.deb" \
 && dpkg -i /tmp/gh.deb && rm /tmp/gh.deb
# Declared services run here as processes, since the environment has no
# container engine (ADR 0005). PostgreSQL comes from the project's official
# repository, which carries every supported major version; no default cluster
# is created, each checkout initializes its own.
ARG POSTGRES_MAJOR=17
RUN install -d /usr/share/postgresql-common/pgdg /etc/postgresql-common \
 && curl -fsSL -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc https://www.postgresql.org/media/keys/ACCC4CF8.asc \
 && echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
 && echo "create_main_cluster = false" > /etc/postgresql-common/createcluster.conf \
 && apt-get update \
 && apt-get install -y --no-install-recommends postgresql-${POSTGRES_MAJOR} \
 && rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION} @openai/codex@${CODEX_VERSION} \
 && npm cache clean --force

# Same layout as the npm package: the CLI finds skills/ and runtime/ beside bin/.
COPY --from=cli /out/archetipo /opt/archetipo/bin/archetipo
COPY skills/ /opt/archetipo/skills/
COPY .archetipo/config.template.yaml /opt/archetipo/runtime/config.yaml
COPY .archetipo/shared-runtime.md /opt/archetipo/runtime/shared-runtime.md
RUN ln -s /opt/archetipo/bin/archetipo /usr/local/bin/archetipo

RUN useradd -m -u 1001 -s /bin/bash archetipo
ENV ARCHETIPO_DATA_DIR=/opt/archetipo \
    ARCHETIPO_PREVIEW_BIND=0.0.0.0 \
    ARCHETIPO_PREVIEW_PORTS=47810-47829 \
    ARCHETIPO_CHROME=/usr/bin/chromium
USER archetipo
WORKDIR /home/archetipo
EXPOSE 47800 47810-47829
CMD ["archetipo", "view", "--host", "0.0.0.0", "--port", "47800", "--no-open"]
