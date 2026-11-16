#!/bin/sh
# End-to-end check of the isolated runtime prototype (ADR 0004), without
# credentials and without an agent. It starts a throwaway environment, creates
# a project with two increments inside it, runs both previews through the real
# CLI and checks from the host what the person's browser would see, and from
# inside what the previewed application can reach.
#
# The environment is started by the real host launcher, `archetipo view
# --isolated`, built from this checkout, with a throwaway state directory.
#
#   docker/isolated/verify.sh            run and destroy the environment
#   KEEP=1 docker/isolated/verify.sh     leave it running for inspection
set -u
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
ID=verify
P="archetipo-$ID"
WS="$P-workspace"
WORK=$(mktemp -d)
export ARCHETIPO_STATE_DIR="$WORK/state"
(cd "$ROOT/cli" && go build -o "$WORK/archetipo" ./cmd/archetipo) || exit 1
CLI="$WORK/archetipo"
docker image inspect archetipo-isolated:latest >/dev/null 2>&1 \
  || docker build -f "$ROOT/docker/isolated/Dockerfile" -t archetipo-isolated:latest "$ROOT" || exit 1
destroy() {
  docker rm -f "$P-workspace" "$P-gateway" >/dev/null 2>&1
  docker network rm "$P-net" >/dev/null 2>&1
  docker volume rm "$P-home" >/dev/null 2>&1
}
FAIL=0
pass() { echo "  ✓ $*"; }
fail() { echo "  ✗ $*"; FAIL=1; }
in_ws() { docker exec "$WS" sh -c "$1"; }

HOSTIP=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || hostname -I 2>/dev/null | cut -d' ' -f1)
SENTINEL_DIR=$(mktemp -d)
echo "host-sentinel" > "$SENTINEL_DIR/index.html"
(cd "$SENTINEL_DIR" && exec python3 -m http.server 47901 --bind 0.0.0.0) >/dev/null 2>&1 &
SENTINEL_PID=$!
cleanup() {
  kill "$SENTINEL_PID" 2>/dev/null
  rm -rf "$SENTINEL_DIR"
  [ "${KEEP:-}" = 1 ] || destroy
  rm -rf "$WORK"
}
trap cleanup EXIT

echo "== environment"
destroy
"$CLI" view --isolated --environment "$ID" --no-open || exit 1
[ "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:47800/)" = 200 ] && pass "archetipo view --isolated returned with the viewer answering" || fail "viewer not answering after launch"
curl -s -m 3 http://127.0.0.1:47901/ | grep -q host-sentinel && pass "control: the host sentinel answers on the host" || fail "host sentinel not running"

echo "== viewer reached from the host browser address only"
status=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:47800/api/workspaces)
[ "$status" = 200 ] && pass "GET /api/workspaces on 127.0.0.1:47800 -> 200" || fail "viewer API answered $status"
curl -s -m 3 -o /dev/null "http://$HOSTIP:47800/" && fail "viewer reachable on the LAN address $HOSTIP" || pass "viewer not reachable on the LAN address $HOSTIP"

echo "== project added from a repository URL, cloned inside the environment"
curl -s http://127.0.0.1:47800/api/workspaces | grep -q '"cloneAvailable":true' && pass "the viewer offers cloning" || fail "cloning not offered"
clone=$(curl -s -X POST -H 'Content-Type: application/json' -H 'Origin: http://127.0.0.1:47800' \
  -d '{"url":"https://github.com/octocat/Hello-World"}' http://127.0.0.1:47800/api/workspaces/clone)
echo "$clone" | grep -q '"dir":"/home/archetipo/workspaces/Hello-World"' && echo "$clone" | grep -q '"initialized":false' \
  && pass "public repository cloned into the volume, reported as not yet a workspace" || fail "clone answered: $clone"
in_ws 'git -C ~/workspaces/Hello-World log --oneline -1' >/dev/null && pass "the clone is a Git checkout inside the environment" || fail "clone missing inside"
clone=$(curl -s -X POST -H 'Content-Type: application/json' -H 'Origin: http://127.0.0.1:47800' \
  -d '{"url":"https://user:secret@github.com/octocat/Hello-World"}' http://127.0.0.1:47800/api/workspaces/clone)
echo "$clone" | grep -q WORKSPACE_CLONE_URL_INVALID && pass "a URL carrying credentials is refused" || fail "credential URL answered: $clone"

echo "== project with two increments, created inside the environment"
in_ws '
set -e
mkdir -p ~/proj && cd ~/proj
git init -q -b main && git config user.name Verify && git config user.email verify@example.test
archetipo init --tool claude --connector file --yes >/dev/null
cat > server.mjs <<JS
import http from "node:http";
const port = Number(process.argv[2]);
async function probe(url) {
  try { const r = await fetch(url, {signal: AbortSignal.timeout(3000)}); return "REACHED " + r.status; }
  catch (e) { return "blocked"; }
}
http.createServer(async (req, res) => {
  res.setHeader("X-Checkout", process.cwd());
  if (req.url.startsWith("/probe")) {
    const target = new URL(req.url, "http://x").searchParams.get("url");
    res.end(await probe(target));
    return;
  }
  res.setHeader("Content-Type", "text/html");
  res.end("<!doctype html><title>Isolated preview</title><h1>Preview from " + process.cwd() + "</h1>");
}).listen(port, "127.0.0.1");
JS
cat > package.json <<JSON
{"name":"verify","private":true,"scripts":{"dev":"node server.mjs"}}
JSON
printf "%s" "{\"version\":1,\"directory\":\".\",\"argv\":[\"npm\",\"run\",\"dev\",\"--\",\"{port}\"],\"url\":\"http://127.0.0.1:{port}/\"}" > .archetipo/preview.json
git add -A . && git commit -q -m baseline
' && pass "project initialized and committed in the volume" || { fail "project setup"; exit 1; }

for n in 1 2; do
  in_ws "cd ~/proj && echo '{\"title\":\"Increment $n\",\"request\":\"verify\"}' > /tmp/inc$n.json && archetipo increment create --file /tmp/inc$n.json > /tmp/inc$n.out 2>&1" \
    || { fail "increment $n create: $(in_ws "cat /tmp/inc$n.out")"; exit 1; }
  WD=$(in_ws "node -e 'console.log(JSON.parse(require(\"fs\").readFileSync(\"/tmp/inc$n.out\",\"utf8\")).data.workdir)'")
  eval "WD$n=\$WD"
  docker exec -d -w "$WD" "$WS" sh -c "archetipo execution preview run > /tmp/preview$n.out 2>/tmp/preview$n.err"
done
pass "two increments: $WD1 and $WD2"

wait_url() {
  i=0
  while [ $i -lt 60 ]; do
    url=$(in_ws "node -e 'try{const d=JSON.parse(require(\"fs\").readFileSync(\"/tmp/preview$1.out\",\"utf8\")).data;if(d.state===\"ready\")console.log(d.url)}catch{}'" 2>/dev/null)
    [ -n "$url" ] && { echo "$url"; return 0; }
    i=$((i+1)); sleep 1
  done
  return 1
}

echo "== previews opened from the host, through the gateway"
URL1=$(wait_url 1) || { fail "preview 1 not ready: $(in_ws 'cat /tmp/preview1.out /tmp/preview1.err' | tail -5)"; exit 1; }
URL2=$(wait_url 2) || { fail "preview 2 not ready: $(in_ws 'cat /tmp/preview2.out /tmp/preview2.err' | tail -5)"; exit 1; }
echo "  preview 1: $URL1"
echo "  preview 2: $URL2"
for n in 1 2; do
  eval "url=\$URL$n; wd=\$WD$n"
  port=$(echo "$url" | sed -E 's#http://[^:]+:([0-9]+)/.*#\1#')
  [ "$port" -ge 47810 ] && [ "$port" -le 47829 ] && pass "preview $n on port $port, inside the published range" || fail "preview $n port $port outside 47810-47829"
  case "$url" in http://127.0.0.1:*) pass "preview $n URL names host loopback" ;; *) fail "preview $n URL $url" ;; esac
  checkout=$(curl -s -m 5 -D - -o /dev/null "$url" | tr -d '\r' | awk -F': ' 'tolower($1)=="x-checkout"{print $2}')
  [ "$checkout" = "$wd" ] && pass "host browser address reaches the app of increment $n ($checkout)" || fail "preview $n from host returned checkout '$checkout'"
done
[ "$URL1" != "$URL2" ] && pass "the two increments have distinct browser origins" || fail "increments share an origin"

echo "== what the previewed application can reach (its own process, inside)"
probe() {
  in_ws "cd $WD1 && node -e '
    fetch(process.argv[1],{signal:AbortSignal.timeout(4000)}).then(r=>console.log(\"REACHED \"+r.status)).catch(()=>console.log(\"blocked\"))' '$1'"
}
[ "$(probe https://example.com/)" = "REACHED 200" ] && pass "internet reachable without proxy configuration" || fail "internet not reachable"
for target in http://host.docker.internal:47901/ "http://$HOSTIP:47901/" http://192.168.65.254:47901/; do
  r=$(probe "$target"); [ "$r" = blocked ] && pass "blocked: $target" || fail "$target -> $r"
done
in_ws 'ls /Users /Volumes /mnt/c 2>/dev/null | head -1' | grep -q . && fail "a host directory is visible" || pass "no host directory is visible (/Users, /Volumes, /mnt/c)"
in_ws 'iptables -A OUTPUT -j ACCEPT 2>&1; ip route add 10.0.0.0/8 dev eth0 2>&1' | grep -q -i 'permission denied\|not permitted' && pass "the workspace cannot change rules or routes" || fail "the workspace changed the boundary"

echo "== recreation keeps work and browser origins"
"$CLI" view stop --isolated --environment "$ID" >/dev/null || fail "stop"
docker ps -a --format '{{.Names}}' | grep -q "^$WS$" && fail "workspace container left after stop" || pass "stop removed the containers"
docker volume inspect "$P-home" >/dev/null 2>&1 && pass "stop kept the volume" || fail "stop removed the volume"
"$CLI" view --isolated --environment "$ID" --no-open >/dev/null || fail "restart"
for n in 1 2; do
  eval "wd=\$WD$n; old=\$URL$n"
  docker exec -d -w "$wd" "$WS" sh -c "archetipo execution preview run > /tmp/preview$n.out 2>/tmp/preview$n.err"
  new=$(wait_url $n) || { fail "preview $n not ready after recreation"; continue; }
  [ "$new" = "$old" ] && pass "increment $n kept its origin $new after the container was recreated" || fail "increment $n moved from $old to $new"
done

echo
[ $FAIL = 0 ] && echo "ALL CHECKS PASSED" || echo "SOME CHECKS FAILED"
exit $FAIL
