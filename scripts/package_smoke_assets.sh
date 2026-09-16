#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

cd "$ROOT_DIR"

case "$(uname -s)" in
  Darwin)
    ./scripts/package_macos_local.sh
    ;;
  Linux)
    ./scripts/package_linux_local.sh
    ;;
  *)
    echo "Skipping local Go GUI package on this OS."
    ;;
esac


echo "Release package build complete."
