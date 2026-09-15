# Content List Generator

Written by Bryan Snyder

Content List Generator is maintained as two live desktop runtimes that stay in feature parity as closely as practical:

- Go app for macOS and Linux, with optional Windows `.exe` build outputs
- Python app for Windows portable deployment

End-user docs: [User Manual](project-dashboard/user-manual.html) · [Project Dashboard](https://snyderb-de.github.io/content-list-generator/)

Both runtimes support:

- recursive content-list export to CSV
- automatic CSV chunking for large scans (default: 300,000 rows per file, named like `report-001.csv`, `report-002.csv`)
- optional XLSX generation
- agency content-list template export with constant metadata fields
- hash verification modes for migration workflows
- plain-text scan reports
- integrated email-file copy with manifest output

## Runtime And Deploy Paths

Core app/runtime files:

- `main.go`, `core.go`, `app.go`, `app_types.go`, `gui_wails.go`, `scan_*.go` — Go runtime
- `frontend/` — React + TypeScript UI (Vite, built into the Wails app bundle)
- `python/content_list_generator.py`, `python/content_list_core.py` — Python runtime

Deploy and distribution files that must stay aligned with the app:

- `deploy/windows/desktop/content-list-generator.bat`
- `deploy/windows/scripts/content-list-gen/content_list_generator.py`
- `deploy/windows/scripts/content-list-gen/content_list_core.py`
- repo-root launchers such as `run-go-gui.sh`, `run-python-gui.sh`, and `content-list-generator.bat`
- packaging helpers in `scripts/`

Generated outputs belong in `build/` and `releases/` and are intentionally not tracked.

## Repo Layout

- `python/` Python runtime code and Python automated tests
- `project-dashboard/` static project dashboard for repo status and docs
- `scripts/` build, parity, packaging, and local-run helpers
- `testing/` tool-oriented fixtures, generators, runners, and ignored local manual-test folders
- `deploy/` copy-ready deployment files that are part of the operational workflow

## Quick Start

```bash
git clone <repo-url>
cd content-list-generator
./scripts/dev_check.sh
```

Local launchers:

- macOS/Linux Go GUI: `./run-go-gui.sh`
- macOS/Linux Python GUI: `./run-python-gui.sh`
- cross-platform helper: `./scripts/run_local.sh [go|go-gui|python|python-cli]`
- Windows desktop launcher: `content-list-generator.bat`

## Release Strategy

Two distinct release tracks with different update policies:

**Native bundles (Wails GUI .exe, mac .app, Linux binary, PyInstaller portable zip)**
- Deps frozen into the bundle at build time
- Users redownload the bundle to update
- Auto-update mechanism planned (see `TODO.md`)

**Python `.bat`-launcher deploy path** (`deploy/windows/*`)
- Deps pinned to EXACT versions in `requirements.txt`
- Admin installs Python + deps on user machines via `pip install -r requirements.txt`
- App shows a non-blocking orange banner at the top of the window if installed deps drift from the pinned versions (see `python/deps_check.py`)
- Bumping a dep is a deliberate event: update `requirements.txt`, bump matching entry in BOTH `python/deps_check.py` and its deploy copy `deploy/windows/scripts/content-list-gen/deps_check.py`, re-deploy scripts via the deploy bundle, admin re-runs `pip install` on user machines
- Build-only deps (PyInstaller) are pinned separately in `requirements-build.txt` — installed by `scripts/package_windows_portable.ps1`, never on user machines
- Dependabot ignores major bumps for npm + gomod, but still surfaces security advisories

## Platform Notes

macOS and Linux:

- use the Go app (Wails GUI or Bubble Tea TUI)
- GUI: double-click `releases/macos/Content List Generator.app` or run `./run-go-gui.sh` in dev mode
- TUI: run the CLI binary directly (no `--gui` flag, no `.app` bundle)
- local binaries are built into `build/`
- local release packages are produced by `./scripts/build_releases.sh`

Windows portable Python path:

- install Python 3 with Tkinter
- install GUI dependencies with `pip install -r requirements.txt`
- Python defaults to `SHA-1`
- `BLAKE3` is optional on Python and requires the `blake3` package from `requirements.txt`
- copy the deploy bundle from `deploy/windows/` or generate a fresh bundle with `./scripts/package_windows_python_bundle.sh`
- supported launcher lookup paths remain `%USERPROFILE%\\scripts\\` and `%USERPROFILE%\\scripts\\content-list-gen\\`

Windows portable no-install ZIP:

- build on a Windows host with `powershell -ExecutionPolicy Bypass -File .\scripts\package_windows_portable.ps1`
- the script creates `releases/windows-portable/content-list-generator-windows-portable.zip`
- unzip the package to a USB drive or local folder, then run `Start Content List Generator.cmd`
- portable settings are stored beside the app in `data/content-list-generator-settings.json`

Windows Wails GUI path:

- must be built on a Windows host (Wails cannot cross-compile WebView2)
- run `wails build -platform windows/amd64 -o "content-list-generator.exe"`
- copy the resulting `.exe` into `releases/windows-go/` with the same filename
- the Windows TUI binary is intentionally not shipped — Windows users get the Wails GUI or the Python bundles

## Testing

Automated checks:

```bash
go test ./...
python3 -m unittest discover -s ./python/tests -p 'test_*.py'
python3 -m py_compile python/content_list_core.py python/content_list_generator.py scripts/copy_email_files.py
```

Shared helper scripts:

- `./scripts/dev_check.sh` runs the main smoke suite
- `./scripts/parity_check.sh` runs the cross-language fixture parity checks

Tool-oriented testing layout:

- `testing/content-scan/` contains content-list fixtures, regeneration helpers, and a feature runner
- `testing/email-copy/` contains email-copy fixtures, regeneration helpers, and a feature runner
- `testing/manual-samples/` and `testing/manual-output/` are reserved for ignored machine-local testing data

## Packaging And Releases

Release and local package helpers:

```bash
./scripts/build_releases.sh
./scripts/package_macos_local.sh
./scripts/package_linux_local.sh
./scripts/package_windows_python_bundle.sh
./scripts/package_windows_portable.ps1
./scripts/package_smoke_assets.sh
./scripts/package_local.sh
```

Publish all generated release artifacts to one GitHub Release:

```bash
git push origin main
scripts/publish_github_release.sh v0.1.0 --target main --draft
```

The publish helper uploads release artifacts under `releases/`, excluding `.gitkeep`;
for the Windows Python source bundle it uploads only the zip, not the loose
staging files used to build that zip.
Build platform-specific artifacts first; for example, run the portable Windows
packager on Windows before publishing if that ZIP should be included.

These scripts generate fresh artifacts in `build/` and `releases/`. The repo no longer treats generated binaries, zips, or tarballs as source files.

## Docs

Canonical docs now live in:

- `README.md` for setup, structure, runtime, and testing
- `TODO.md` for active follow-up work
- `project-dashboard/` for a lightweight static project overview

## Local GUI Reference

For Python GUI work that depends on `customtkinter`, we also have a local fork checked out at:

- `/Users/baghead/code/CustomTkinter`

Use that local repo as an offline reference for:

- source-level behavior review
- examples and usage patterns
- platform-specific GUI behavior checks
- future work such as matching the system light/dark mode on Windows, Linux, and macOS
- verifying that text is not clipped or truncated in widgets and layouts

When making Python GUI changes in this repo, prefer checking the local `CustomTkinter` fork before assuming behavior from memory or from internet docs.
