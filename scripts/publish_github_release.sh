#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
RELEASES_DIR="$ROOT_DIR/releases"

usage() {
  cat <<'EOF'
Usage:
  scripts/publish_github_release.sh <tag> [--target <branch-or-sha>] [--title <title>] [--notes <notes>] [--notes-file <file>] [--draft] [--prerelease] [--dry-run]

Examples:
  scripts/publish_github_release.sh v0.1.0 --target main --draft
  scripts/publish_github_release.sh v0.1.0 --title "Content List Toolkit" --notes-file release-notes.md

Uploads release artifacts under releases/, excluding .gitkeep.
If the GitHub release already exists, matching assets are overwritten.
EOF
}

if [[ $# -eq 1 && ( "$1" == "-h" || "$1" == "--help" ) ]]; then
  usage
  exit 0
fi

if [[ $# -lt 1 ]]; then
  usage
  exit 2
fi

TAG="$1"
shift

TARGET="main"
TITLE="Content List Toolkit"
NOTES=""
NOTES_FILE=""
DRAFT=0
PRERELEASE=0
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      TARGET="${2:-}"
      shift 2
      ;;
    --title)
      TITLE="${2:-}"
      shift 2
      ;;
    --notes)
      NOTES="${2:-}"
      shift 2
      ;;
    --notes-file)
      NOTES_FILE="${2:-}"
      shift 2
      ;;
    --draft)
      DRAFT=1
      shift
      ;;
    --prerelease)
      PRERELEASE=1
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
done

# Read with a loop rather than mapfile: macOS ships bash 3.2, which has no
# mapfile, so this script could not run on the machine that builds the macOS
# release. NUL separators throughout, because asset names contain spaces.
ASSETS=()
while IFS= read -r -d '' asset; do
  ASSETS+=("$asset")
done < <(
  # Dotfiles are never assets. .DS_Store reached a published release once,
  # uploaded as "default.DS_Store" because GitHub will not take a leading dot.
  find "$RELEASES_DIR" -mindepth 2 -maxdepth 2 -type f ! -name ".*" -print0 |
    sort -z
)

if [[ ${#ASSETS[@]} -eq 0 ]]; then
  echo "No release artifacts found under $RELEASES_DIR" >&2
  exit 1
fi

echo "Release assets:"
for asset in "${ASSETS[@]}"; do
  printf '  %s\n' "${asset#$ROOT_DIR/}"
done

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "Dry run only; no GitHub release was created or updated."
  exit 0
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "GitHub CLI is required. Install gh and run 'gh auth login' first." >&2
  exit 1
fi

if gh release view "$TAG" >/dev/null 2>&1; then
  echo "GitHub release $TAG already exists; uploading assets with --clobber."
  gh release edit "$TAG" --title "$TITLE"
  gh release upload "$TAG" "${ASSETS[@]}" --clobber
else
  args=(release create "$TAG" "${ASSETS[@]}" --target "$TARGET" --title "$TITLE")
  if [[ -n "$NOTES_FILE" ]]; then
    args+=(--notes-file "$NOTES_FILE")
  elif [[ -n "$NOTES" ]]; then
    args+=(--notes "$NOTES")
  else
    args+=(--generate-notes)
  fi
  if [[ "$DRAFT" -eq 1 ]]; then
    args+=(--draft)
  fi
  if [[ "$PRERELEASE" -eq 1 ]]; then
    args+=(--prerelease)
  fi
  gh "${args[@]}"
fi

echo "Published GitHub release assets for $TAG."
