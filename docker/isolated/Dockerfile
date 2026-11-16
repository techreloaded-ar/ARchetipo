# Image of the isolated increment runtime (ADR 0004). One image serves both
# containers of an environment: the gateway, which owns the network namespace
# and its firewall, and the workspace, which runs the viewer, the agents, the
# recipes and the previews without capabilities.
#
# Build from the repository root:
#   docker build -f docker/isolated/Dockerfile -t archetipo-isolated .

FROM golang:1.26-bookworm AS cli
WORKDIR /src
COPY cli/go.mod cli/go.sum ./
RUN go mod download
COPY cli/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/archetipo ./cmd/archetipo

FROM node:22-bookworm-slim
ARG CLAUDE_CODE_VERSION=2.1.325
ARG CODEX_VERSION=0.162.1
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates curl gh iptables iproute2 procps chromium \
 && rm -rf /var/lib/apt/lists/*
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION} @openai/codex@${CODEX_VERSION} \
 && npm cache clean --force

# Same layout as the npm package: the CLI finds skills/ and runtime/ beside bin/.
COPY --from=cli /out/archetipo /opt/archetipo/bin/archetipo
COPY skills/ /opt/archetipo/skills/
COPY .archetipo/config.template.yaml /opt/archetipo/runtime/config.yaml
COPY .archetipo/shared-runtime.md /opt/archetipo/runtime/shared-runtime.md
COPY docker/isolated/gateway.sh /opt/archetipo/gateway.sh
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
