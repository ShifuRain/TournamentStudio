#!/usr/bin/env bash
# Builds the TournamentStudio frontend and embeds it into the Go binary.
# See README.md "Building from source" for the manual two-step process.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT="${1:-tournamentstudio}"

command -v npm >/dev/null 2>&1 || { echo "error: npm not found in PATH" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "error: go not found in PATH" >&2; exit 1; }

echo "==> Building frontend"
(
  cd "$ROOT_DIR/frontend"
  npm install
  npm run build
)

echo "==> Building Go binary -> $OUTPUT"
(
  cd "$ROOT_DIR"
  go build -o "$OUTPUT" ./cmd/tournamentstudio
)

case "$OUTPUT" in
  /*) OUTPUT_PATH="$OUTPUT" ;;
  *) OUTPUT_PATH="$ROOT_DIR/$OUTPUT" ;;
esac
echo "==> Done: $OUTPUT_PATH"
