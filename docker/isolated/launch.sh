#!/bin/sh
# Prototype launcher of the isolated runtime (ADR 0004). It will become
# `archetipo view --isolated`; until then it shows the exact Docker objects
# an environment is made of.
#
#   docker/isolated/launch.sh [up|down|destroy] [environment-id]
#
# up       builds the image if missing, starts gateway and workspace, keeps the volume
# down     stops and removes the containers, keeps the volume (work is kept)
# destroy  removes containers, network and volume (unpushed work is lost)
set -eu
ACTION=${1:-up}
ID=${2:-default}
IMAGE=${ARCHETIPO_ISOLATED_IMAGE:-archetipo-isolated}
P="archetipo-$ID"
VIEW_PORT=47800
PREVIEW_PORTS=47810-47829
ROOT=$(cd "$(dirname "$0")/../.." && pwd)

down() { docker rm -f "$P-workspace" "$P-gateway" >/dev/null 2>&1 || true; }

case "$ACTION" in
  down) down; echo "stopped $P (volume $P-home kept)"; exit 0 ;;
  destroy) down; docker network rm "$P-net" >/dev/null 2>&1 || true; docker volume rm "$P-home" >/dev/null 2>&1 || true; echo "destroyed $P"; exit 0 ;;
  up) ;;
  *) echo "usage: $0 [up|down|destroy] [environment-id]" >&2; exit 2 ;;
esac

docker info >/dev/null 2>&1 || { echo "Docker non è avviato: apri Docker Desktop e riprova." >&2; exit 1; }
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker build -f "$ROOT/docker/isolated/Dockerfile" -t "$IMAGE" "$ROOT"
down
docker network inspect "$P-net" >/dev/null 2>&1 || docker network create "$P-net" >/dev/null
docker volume inspect "$P-home" >/dev/null 2>&1 || docker volume create "$P-home" >/dev/null

docker run -d --name "$P-gateway" --network "$P-net" --user root \
  --cap-drop ALL --cap-add NET_ADMIN --security-opt no-new-privileges \
  -p "127.0.0.1:$VIEW_PORT:$VIEW_PORT" -p "127.0.0.1:$PREVIEW_PORTS:$PREVIEW_PORTS" \
  "$IMAGE" /opt/archetipo/gateway.sh >/dev/null
i=0
until docker exec "$P-gateway" test -f /tmp/gateway-ready 2>/dev/null; do
  i=$((i+1)); [ $i -gt 30 ] && { docker logs "$P-gateway" >&2; echo "gateway did not start" >&2; exit 1; }
  sleep 1
done

docker run -d --name "$P-workspace" --network "container:$P-gateway" \
  --cap-drop ALL --security-opt no-new-privileges \
  -v "$P-home:/home/archetipo" "$IMAGE" >/dev/null

i=0
until curl -s -o /dev/null "http://127.0.0.1:$VIEW_PORT/"; do
  i=$((i+1)); [ $i -gt 30 ] && { docker logs "$P-workspace" >&2; echo "viewer did not start" >&2; exit 1; }
  sleep 1
done
echo "ARchetipo isolato: http://127.0.0.1:$VIEW_PORT/"
