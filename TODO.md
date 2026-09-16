# TODO

## Retiring the Python runtime

Decided 2026-09-16. The Python app existed to give Windows a GUI before Wails
could build one. The Go app is now a strict superset — Python had content list,
email copy, and about; Go has those plus Clone Compare, the User Manual, and the
Getty Tag check — so the second implementation, the pinned `customtkinter` and
`blake3` versions, the duplicated `deps_check.py`, and the cross-language parity
fixtures all cost maintenance for nothing.

Documented in `README.md` and `docs/windows-build-checklist.md`. The code is
still in the tree so existing deployments have somewhere to migrate from.

Removal checklist, not yet started:

- [ ] Confirm no admin-deployed machine is still running the `.bat` launcher
- [ ] Stop publishing `content-list-generator-windows-portable.zip` and
      `content-list-generator-windows-python.zip`; drop the `windows-portable`
      and `windows-python` jobs from `.github/workflows/release.yml`
- [ ] Remove the download cards for both from `project-dashboard/index.html`
- [ ] Delete `python/`, `deploy/windows/`, `requirements.txt`,
      `requirements-build.txt`, `run-python-gui.{sh,bat}`,
      `scripts/package_windows_python_bundle.sh`,
      `scripts/package_windows_portable.ps1`
- [ ] Drop the Python steps from `scripts/dev_check.sh` and remove
      `scripts/parity_check.sh` along with the shared fixtures it drives
- [ ] Remove the `pip` ecosystem from `.github/dependabot.yml`
- [ ] Drop `PYTHON_VERSION` from `.github/workflows/release.yml`
- [ ] Update `testing/README.md` and the `testing/*/` runners that invoke Python

## Recently Shipped

### v0.2.7 (2026-06-12)
- ✅ Agency-template constant fields now start fresh for each new scan so old sheet-wide values cannot be reused by mistake
- ✅ `RC_Series` remains required in agency-template mode; Start Scan stays disabled until it is filled
- ✅ In-app User Manual added to the main menu with staff-facing workflow guidance and light/dark theme matching
- ✅ Settings regression tests added for fresh agency fields and legacy saved settings
- ✅ GitHub Release published at `https://github.com/snyderb-de/content-list-generator/releases/tag/v0.2.7`

### v0.2.6 (2026-06-12)
- ✅ GUI light/dark theme pass: stronger neutral palettes, clearer text contrast, and less pastel/muddy chrome
- ✅ Full-width form inputs for readable filenames and paths in the Wails UI
- ✅ Dev-only Wails bridge stub added so Vite browser UI review can run without crashing outside the desktop shell

### v0.2.5 (2026-06-12)
- ✅ Corrective release for the Windows Python source bundle: `deps_check.py` is included so launchers can import the dependency drift checker
- ✅ Release publisher now uploads only `content-list-generator-windows-python.zip` for the Windows Python source bundle, not loose staging files
- ✅ Release workflow builds with Node 24

### v0.2.4 (2026-06-12)
- ✅ Agency content-list template output mode added using `Content List Agencies.xlsx` headers
- ✅ Staff can fill constant agency fields once per sheet; `RC_Series` is required in agency mode
- ✅ Standard scan output remains available for audit/hash details
- ✅ Local Go/Python tests and shared fixtures are now tracked so CI runs real parity coverage
- ✅ GitHub Release published at `https://github.com/snyderb-de/content-list-generator/releases/tag/v0.2.4`
- ✅ GitHub Actions JavaScript actions updated to Node 24-compatible major versions

### v0.2.3 (2026-05-20)
- ✅ GitHub Pages dashboard deploy workflow shipped and Pages is live at `https://snyderb-de.github.io/content-list-toolkit/`
- ✅ User manual redesigned in the dashboard style and linked from `README.md`
- ✅ Scan progress overlay now persists while navigating between GUI screens
- ✅ Sponsor button support merged via `.github/FUNDING.yml` after v0.2.3

### v0.2.2 (2026-05-20)
- ✅ **Windows GUI silent launch failure FIXED** — root cause was `isGUIContext()` had no Windows production detection; binary fell through to Bubble Tea TUI which tried to open console input with no console attached → exit code 1, `open CONIN$: The handle is invalid.`
- ✅ Fix detects PE Subsystem field via `debug/pe`: Wails `-H windowsgui` → Subsystem=2 (GUI) routes to Wails; default `go build` → Subsystem=3 (Console) routes to TUI. Platform-agnostic, no env-var sniffing.
- ✅ Project dashboard rebuilt with dates-formatter style (IBM Plex, navy-orange, light/dark, live GitHub stats, downloads grid, roadmap)
- ✅ User manual stub at `project-dashboard/user-manual.html`
- ✅ Counting display improvements in scan progress (ContentList.tsx)

### v0.2.0 – v0.2.1 (2026-05-18 – 2026-05-19)
- ✅ Wails Windows GUI build via CI (no Windows host required)
- ✅ App icon set (svg + icns + multi-resolution ico)
- ✅ CI matrix: macOS, Linux, Windows GUI amd64/arm64, Windows portable, Windows python source
- ✅ Tag-driven publish pipeline (push `v*` tag → GitHub Release auto-attached)
- ✅ Python deploy strategy: exact-pin requirements.txt + runtime drift banner
- ✅ Dependabot tuned (no major-bump noise; security alerts still flow)
- ✅ Docs: `docs/github-setup-guide.md`, `docs/windows-build-checklist.md`, `docs/debug/windows-launch-failure.md`

## Active (priority order)

### 📌 P0 — sign and verify v0.2.7 user release
- Owner handoff: Bryan will sign the Windows and macOS release artifacts before broad staff distribution
- Decide signed-asset release path:
  - Replace the existing `v0.2.7` release assets if only the signatures change and the source commit stays `c425abd`
  - Cut `v0.2.8` only if signing changes packaging scripts, release metadata, or any shipped source/content
- macOS `.app` from `content-list-generator-gui-darwin-universal.zip` — verify signed/notarized app opens on a normal user machine and runs a small content-list scan
- Windows Wails `.exe` from `content-list-generator.exe` — verify signed app opens without unexpected SmartScreen friction and writes a small agency-template XLSX/CSV correctly
- Windows portable `.zip` — verify current v0.2.7 bundle launches and writes CSV correctly on a managed Windows host
- Python source bundle — verify `.bat` launchers, dependency drift banner behavior, and agency-template CSV output on Windows
- Linux binary smoke test on a Linux host
- If Windows `.exe` is green on the target host: mark `docs/debug/windows-launch-failure.md` fully verified

### ~~P0 — write real user manual content~~ ✅ (2026-05-26)
- ✅ `project-dashboard/user-manual.html` fact-checked against code; CSV columns, verdict names (Exact / Content / Metadata / Not a Clone), full 14-extension email list, soft-compare PDF-only behavior, report file naming all corrected
- ✅ FAQ section added (10 entries)
- ✅ README cross-link to user manual added
- Followup (P2): replace screenshot placeholders once GUI captures available

### P0 — capture GUI screenshots
- macOS `.app` (Wails GUI)
- Wails Windows GUI (now unblocked by v0.2.2 fix)
- Python customtkinter GUI (managed Windows path)
- Bubble Tea TUI (Linux/Mac terminal)
- Add to README hero + user manual + dashboard hero
- Suggested resolution: 1600×1000 PNG, light-mode default

### ~~P0 — enable GitHub Pages for the dashboard~~ ✅ (2026-05-20)
- ✅ Dashboard URL: `https://snyderb-de.github.io/content-list-toolkit/`
- Followup (P2): add the URL to README hero + repo About sidebar if desired

### ~~P1 — finish dependabot / Actions runtime sweep~~ ✅ (2026-06-12)
- ✅ `checkout` 4→6, `setup-go` 5→6, `setup-node` 4→6, `upload-artifact` 4→7, `download-artifact` 4→8
- ✅ Pages actions bumped: `configure-pages` 5→6, `upload-pages-artifact` 3→5, `deploy-pages` 4→5
- ✅ Release workflow build Node pin moved from 20 → 24
- ✅ PR and post-merge workflows passed

### P2 — release hygiene followups
- Local helper cleanup: old `go build -tags gui` path replaced with Wails-aware scripts
- Re-enable PR approval rule (or accept solo-dev posture) — branch protection requires PRs but currently allows merge without review

### P3 — feature/test work
- Test large scan (>300k rows) — verify CSV chunking visible in GUI progress
- Test Phase 7 (soft compare) — Newark drives CON-P74THY / CON-M4EM1V with soft compare on; verdict should be `Metadata Clone`, 1,831 metadata-only diffs

## Backlog (no order)
- Auto-update mechanism for Wails app
- Decide the final public GitHub repo URL and replace placeholder links in `python/content_list_generator.py`
- Transfer repo ownership or publishing control to `dpa-snyder`
- Decide the final project license (evaluate GPL vs MIT vs Apache)
- Decide the final attribution requirement for reuse or redistribution
- Smooth ETA behavior for very large scans with long-tail large files
- Investigate MacBook touchpad scrolling in the Python GUI; add proper macOS trackpad scroll handling
- Decide whether release bundles stay portable-only or move toward installer-style distribution
- Package the Linux release from a Linux build host
- Live diff table virtual scrolling (cap at 5000 rows currently — DOM choke risk at 100k+)
- Phase 7 soft compare: extend to same-path hash mismatches (not just path-renamed PDFs)
- CI parity check: fail if `python/*.py` and `deploy/windows/scripts/content-list-gen/*.py` drift apart

## Notes
- Branch protection: review-required rule currently OFF (turned off to allow solo-dev merging). Re-enable when adding collaborators.
- Dependabot config: ignores semver-major for gomod + npm. Security advisories still surface separately via Security tab.
- Workflow file path: `.github/workflows/release.yml`. Triggers: tag push, PR to main, manual dispatch.
- Latest release: **v0.2.7** (2026-06-12) — fresh agency-template fields plus in-app staff user manual.
