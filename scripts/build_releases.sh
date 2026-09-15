#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
RELEASES_DIR="$ROOT_DIR/releases"
MACOS_DIR="$RELEASES_DIR/macos"
LINUX_DIR="$RELEASES_DIR/linux"
WINDOWS_GO_DIR="$RELEASES_DIR/windows-go"
WINDOWS_PY_DIR="$RELEASES_DIR/windows-python"

mkdir -p "$MACOS_DIR" "$LINUX_DIR" "$WINDOWS_GO_DIR" "$WINDOWS_PY_DIR"

cd "$ROOT_DIR"

export PATH="$PATH:$(go env GOPATH)/bin"

echo "── Frontend build (required by go:embed in gui_wails.go) ────"
(cd frontend && npm ci && npm run build)

echo "── Tests ────────────────────────────────────────────────────"
go test ./...

echo "── CLI / TUI binaries (go build) ───────────────────────────"
# Windows TUI .exe intentionally not shipped. Windows users get the Wails GUI
# (built on a Windows host) or the Python bundle. TUI source still compiles
# fine on Windows via `go build` for developers who want it locally.
GOOS=darwin  GOARCH=arm64 go build -o "$MACOS_DIR/content-list-generator-darwin-arm64"                  .
GOOS=darwin  GOARCH=amd64 go build -o "$MACOS_DIR/content-list-generator-darwin-amd64"                  .
GOOS=linux   GOARCH=amd64 go build -o "$LINUX_DIR/content-list-generator-linux-amd64"                   .

echo "── GUI app — macOS universal (wails build) ──────────────────"
wails build -platform darwin/universal -clean -o "content-list-generator"
# Output: build/bin/content-list-generator.app  (wails wraps in .app)
APP_SRC="$ROOT_DIR/build/bin/content-list-generator.app"
if [ -d "$APP_SRC" ]; then
    rm -rf "$MACOS_DIR/content-list-generator.app"
    cp -r "$APP_SRC" "$MACOS_DIR/content-list-generator.app"
    cd "$MACOS_DIR" && zip -qr "content-list-generator-gui-darwin-universal.zip" "content-list-generator.app"
    echo "  → $MACOS_DIR/content-list-generator-gui-darwin-universal.zip"
else
    # wails may use the name from wails.json ("Content List Toolkit")
    APP_SRC2="$ROOT_DIR/build/bin/Content List Toolkit.app"
    if [ -d "$APP_SRC2" ]; then
        rm -rf "$MACOS_DIR/Content List Toolkit.app"
        cp -r "$APP_SRC2" "$MACOS_DIR/Content List Toolkit.app"
        cd "$MACOS_DIR" && zip -qr "content-list-generator-gui-darwin-universal.zip" "Content List Toolkit.app"
        echo "  → $MACOS_DIR/content-list-generator-gui-darwin-universal.zip"
    fi
fi
cd "$ROOT_DIR"

# Windows GUI requires running on Windows — wails cannot cross-compile the WebView2 wrapper.
# Run on a Windows host: wails build -platform windows/amd64 -o "content-list-generator.exe"
# Then copy the .exe to releases/windows-go/.

echo "── Done ─────────────────────────────────────────────────────"
ls -lh "$MACOS_DIR" "$WINDOWS_GO_DIR" "$LINUX_DIR" 2>/dev/null | grep -v "^total\|^$" || true
