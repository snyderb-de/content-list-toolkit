#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="$ROOT_DIR/build"
LOCAL_DIR="$BUILD_DIR/local"

mkdir -p "$BUILD_DIR" "$LOCAL_DIR"

cd "$ROOT_DIR"

./scripts/dev_check.sh

go build -o "$BUILD_DIR/content-list-generator" .

echo "  Go GUI app: use ./scripts/build_releases.sh for Wails GUI packaging"

echo "Built local artifacts:"
echo "  Go binary: $BUILD_DIR/content-list-generator"
