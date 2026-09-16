#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
MODE="${1:-go}"
BUILD_DIR="$ROOT_DIR/build"

cd "$ROOT_DIR"

case "$MODE" in
  go)
    mkdir -p "$BUILD_DIR"
    go build -o "$BUILD_DIR/content-list-generator" .
    exec "$BUILD_DIR/content-list-generator"
    ;;
  go-gui)
    exec ./run-go-gui.sh
    ;;
  *)
    echo "Usage: $0 [go|go-gui]" >&2
    exit 1
    ;;
esac
