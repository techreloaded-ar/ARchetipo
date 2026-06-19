#!/usr/bin/env bash
# ci-check.sh — ARchetipo CI validation suite
# Runs the full quality gate from the cli/ directory.
# Usage: ./ci-check.sh          (from repo root or cli/)
#        bash ci-check.sh       (from repo root or cli/)
#
# Exit code: 0 only when all five checks pass.

set -euo pipefail

# --- helpers -----------------------------------------------------------

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

pass()  { printf "${GREEN}PASS${NC}  %s\n" "$*"; }
fail()  { printf "${RED}FAIL${NC}  %s\n" "$*"; exit 1; }

# Resolve cli/ directory: if the script is inside cli/, use the script's
# own directory; otherwise assume we are at the repo root and enter cli/.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [[ "$(basename "$SCRIPT_DIR")" == "cli" ]]; then
	CLI_DIR="$SCRIPT_DIR"
else
	CLI_DIR="$SCRIPT_DIR/cli"
fi

cd "$CLI_DIR"
echo "==> Running CI checks from $CLI_DIR"

# ---- 1. gofmt ---------------------------------------------------------
echo ""
echo "--- gofmt ---"
FMT_OUT="$(gofmt -l . 2>&1)" || true
if [[ -z "$FMT_OUT" ]]; then
	pass "gofmt — no formatting issues"
else
	fail "gofmt — files with formatting issues:"
	echo "$FMT_OUT"
	exit 1
fi

# ---- 2. go vet --------------------------------------------------------
echo ""
echo "--- go vet ---"
go vet ./...
pass "go vet — no issues"

# ---- 3. go build ------------------------------------------------------
echo ""
echo "--- go build ---"
go build ./...
pass "go build — successful"

# ---- 4. go test -------------------------------------------------------
echo ""
echo "--- go test ---"
go test ./...
pass "go test — all tests passed"

# ---- 5. golangci-lint -----------------------------------------------
echo ""
echo "--- golangci-lint ---"
golangci-lint run --timeout 5m ./...
pass "golangci-lint — 0 issues"

echo ""
echo "==> All CI checks passed. ✅"
