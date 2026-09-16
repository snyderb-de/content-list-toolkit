#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

cd "$ROOT_DIR"

go vet ./...
go test ./...

# The in-app manual and its HTML mirror are generated from one source. This
# fails when the committed mirror no longer matches it.
npm --prefix frontend run --silent manual:check

echo "All checks passed."
