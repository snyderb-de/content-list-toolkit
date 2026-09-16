# Content List Toolkit

Written by Bryan Snyder

Content List Toolkit is a single Go desktop application: a Wails GUI on macOS,
Windows, and Linux, plus a Bubble Tea TUI on macOS and Linux.

> **The Python runtime was removed on 2026-09-16.** It existed to give Windows a
> GUI before the Wails build covered that platform. The Go app is a strict
> superset, so nothing was lost. See
> [Retired: the Python runtime](#retired-the-python-runtime) if you are
> migrating an existing Windows deployment.

End-user docs: [User Manual](project-dashboard/user-manual.html) · [Project Dashboard](https://snyderb-de.github.io/content-list-toolkit/)

The app supports:

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
- `getty_*.go` — Getty AAT tag checking for the CONTENTdm workflow
- `frontend/` — React + TypeScript UI (Vite, built into the Wails app bundle)

Deploy and distribution files that must stay aligned with the app:

- repo-root launchers such as `run-go-gui.sh` and `content-list-generator.bat`
- packaging helpers in `scripts/`

The `python/` runtime and the `deploy/windows/` `.bat` bundle were removed on
2026-09-16. They remain in git history at tag `v0.2.10` if ever needed.

Generated outputs belong in `build/` and `releases/` and are intentionally not tracked.

## Repo Layout

- `project-dashboard/` static project dashboard for repo status and docs
  - `user-manual.html` is the standalone web manual, written by hand
  - `app-user-manual.html` is generated from `frontend/src/manual.json` and mirrors the manual inside the app; edit the JSON, never this page
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
- cross-platform helper: `./scripts/run_local.sh [go|go-gui]`
- Windows desktop launcher: `content-list-generator.bat`

## Release Strategy

Two distinct release tracks with different update policies:

**Native bundles (Wails GUI .exe, mac .app, Linux binary)**
- Deps frozen into the bundle at build time
- Users redownload the bundle to update
- Windows has an in-app updater that checks a release folder (see `update.go`)
- Dependabot raises major bumps in their own grouped PR, separate from minor and patch. Majors were ignored until 2026-09-16, which is how the frontend reached five Vite majors and three TypeScript majors behind while every dependency PR looked green — a major needs reading before merging, which argues for a separate pile rather than silence

The Python-derived artifacts — the PyInstaller portable zip and the Windows
Python source bundle — are retired along with the runtime that produced them.

## Platform Notes

macOS and Linux:

- use the Go app (Wails GUI or Bubble Tea TUI)
- GUI: double-click `releases/macos/Content List Toolkit.app` or run `./run-go-gui.sh` in dev mode
- TUI: run the CLI binary directly (no `--gui` flag, no `.app` bundle)
- local binaries are built into `build/`
- local release packages are produced by `./scripts/build_releases.sh`

Windows Wails GUI path:

- must be built on a Windows host (Wails cannot cross-compile WebView2)
- run `wails build -platform windows/amd64 -o "content-list-generator.exe"`
- copy the resulting `.exe` into `releases/windows-go/` with the same filename
- the `.exe` needs no installer and runs from any folder, including a USB drive, which is what the retired PyInstaller portable zip existed to provide
- the Windows TUI binary is intentionally not shipped — Windows users get the Wails GUI

## Testing

Automated checks:

```bash
go test ./...
```

The golden fixtures under `testing/` began as cross-language parity checks
against the Python runtime. They outlived it: `scan_test.go` and
`email_copy_test.go` still assert against them as regression coverage.

Shared helper scripts:

- `./scripts/dev_check.sh` runs vet and the full test suite

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

## Retired: the Python runtime

The Python app was the Windows GUI before Wails could build one. Now that the
Go app ships a Windows `.exe` that needs no installer and runs from a USB
drive, Python covers nothing the Go app does not, and it cost real maintenance:
a second implementation of every feature, pinned `customtkinter` and `blake3`
versions to keep aligned on admin-deployed machines, a duplicated
`deps_check.py` that had already drifted out of sync, and cross-language parity
fixtures for every change.

**Nothing is lost.** The Python GUI had three screens — content list, email
copy, and about. The Go app has those plus Clone Compare, the in-app User
Manual, and the Getty Tag check.

### If you have a Python deployment today

| Was | Now |
| --- | --- |
| `.bat` launcher running `content_list_generator.py` from `%USERPROFILE%\scripts\content-list-gen\` | `content-list-generator.exe` from the Windows GUI release |
| `content-list-generator-windows-portable.zip` (PyInstaller) | the same `.exe` — no installer, runs from any folder |
| `content-list-generator-windows-python.zip` | no replacement needed |
| `pip install -r requirements.txt` on each machine | nothing to install |

Settings do not carry over automatically. The Python app stored them in
`~/scripts/settings/content-list-generator-settings.json`; the Go app uses
`%APPDATA%\content-list-generator\settings.json`. They are small and quick to
re-enter.

### What was removed

`python/`, `deploy/windows/`, `requirements.txt`, `requirements-build.txt`, the
Python launchers, `scripts/package_windows_python_bundle.sh`,
`scripts/package_windows_portable.ps1`, `scripts/parity_check.sh`, and
`scripts/copy_email_files.py`. The `windows-portable` and `windows-python` CI
jobs and the `pip` Dependabot ecosystem went with them.

Two things deliberately stayed. The golden fixtures under `testing/` are
asserted against by Go tests, so they are regression coverage rather than
parity leftovers. And `testing/*/generate_fixture.py` regenerates those
fixtures — it is standard-library dev tooling that never imported the retired
runtime.

Everything removed is in git history at tag `v0.2.10`.
