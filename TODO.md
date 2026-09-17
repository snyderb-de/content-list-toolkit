# TODO

## E-01 — YouTube Upload (in progress)

New left-hand screen. The operator fills a form or loads a row from a
spreadsheet, supplies a video file and an ArchivERA URL, and the app uploads to
YouTube with the record's title, description, Resource ID, and AE link in the
description.

Decided 2026-09-16: credentials are supplied by the operator, not shipped;
first version handles one video at a time from either the form or the sheet;
the description layout follows the client's spreadsheet and the existing
Wilmington City Council video rather than a format we invent.

Landed: OAuth against an operator-supplied client_secret.json, token storage,
and the resumable upload protocol with progress. 182 tests.

### P0-03 — Start the Google API audit

**Blocking, and outside our control.** Google restricts every video uploaded
through `videos.insert` from an unverified project created after 28 July 2020
to private, whatever privacy status the request asks for. Removing that needs
an audit of the Cloud project against Google's Terms of Service.

Until it passes, each uploaded video has to be made public by hand in YouTube
Studio, which removes much of the point. The audit is a review with a
turnaround nobody here sets, so it wants starting now and running alongside the
build rather than after it.

Owner: whoever owns the Google account.

### P0-04 — Supply a client_secret.json

A Google Cloud project with the YouTube Data API enabled and an OAuth client of
type **Desktop app**. Web application clients will not work for a desktop
upload and the app says so if one is chosen.

Not urgent for development: the screen and the whole upload path are tested
against a local fake. It is needed before a real video moves.

The project's channel, its daily allowance, and its audit all belong to the
organisation. That is why the app reads a credentials file rather than shipping
one — and why nothing secret is committed to this public repository.

### P0-05 — Provide the client's spreadsheet

Save it to `testing/manual-samples/youtube/`, which is gitignored.

It decides the description layout: the client's sheet carries record title,
description, and item number labelled "Resource ID", and asked for the
ArchivERA URL to appear in the YouTube description. Reading the real columns
beats guessing at them — the Access template already proved that, where the
export's headers turned out to differ from the database's.

### P0-06 — Build the screen

Waiting on P0-05 for the description layout. Covers the sidebar entry, the
form, loading a row from a spreadsheet, the blank template the operator can
save from the app, a confirmation of exactly what will be sent, upload progress,
and the resulting video link.

### P0-07 — Decide the privacy status to request

Private, unlisted, or public. Worth deciding deliberately rather than
defaulting: while P0-03 is outstanding every upload is private regardless, so
the choice only takes effect once the audit passes — which is exactly when a
wrong default would publish something before anyone intended.

## Known limits, for reference

- `videos.insert` has its own daily allowance, documented as 100 uploads a day
  for a new project, separate from the 10,000-unit pool the other endpoints
  share. Confirm the real figure in the Cloud console before a large batch.
- Only the `youtube.upload` scope is requested. It can insert a video and
  nothing else: not read the channel, not edit or delete what is already there.

## Open questions from the Getty module

Raised 2026-09-16 while building the tag check. Both are cheap because the
machinery already exists — the vocabulary seam, the cache, the reachability
probe, the findings UI, and the report all take another field without changing
shape.

### Check the other two controlled fields?

The CONTENTdm template has two more fields drawn from fixed vocabularies, and
neither is checked today.

- `Location(TGN)` draws on the Getty Thesaurus of Geographic Names, a different
  Getty vocabulary on the same SPARQL endpoint. The template ships 64 Delaware
  places in TGN's hierarchical form, `United States -- Delaware -- Kent County
  -- Dover`. Checking it is the AAT query with a different scheme.
- `Type` draws on the DCMI Type Vocabulary — 12 fixed values, from Collection
  to Text. A closed list of twelve needs no network and cannot go stale.

Open question: is either field actually going wrong in practice? The tag check
was built because tags were breaking uploads by hand. Nobody has said these two
are a problem, and a check nobody needs is a check that gets ignored.

### Clean every free-text field, not just Tags?

The invisible characters the tag check removes arrive by copy and paste, and
`Title`, `Description of Item(s)`, and every other free-text column are pasted
into the same way. A tab or a newline in any of them breaks a tab-delimited
upload exactly as it does in Tags.

Widening the cleaning pass is small. The work is in the reporting: findings are
currently organised per tag within one column, and a whole-row view of eight
columns is a different screen, not a wider table.

Open question: has an upload ever failed on a field other than Tags? If it has,
this is the more valuable of the two. If it has not, the tag check may already
cover the only column people paste Getty terms into.

## Retiring the Python runtime

Decided 2026-09-16. The Python app existed to give Windows a GUI before Wails
could build one. The Go app is now a strict superset — Python had content list,
email copy, and about; Go has those plus Clone Compare, the User Manual, and the
Getty Tag check — so the second implementation, the pinned `customtkinter` and
`blake3` versions, the duplicated `deps_check.py`, and the cross-language parity
fixtures all cost maintenance for nothing.

Documented in `README.md` and `docs/windows-build-checklist.md`. The code is
still in the tree so existing deployments have somewhere to migrate from.

Removed on 2026-09-16, after confirming no machine still runs the `.bat`
launcher. Deleted `python/`, `deploy/windows/`, the requirements files, the
Python launchers, the two Python packaging scripts, `parity_check.sh`, and
`copy_email_files.py`; dropped the `windows-portable` and `windows-python` CI
jobs, `PYTHON_VERSION`, and the `pip` Dependabot ecosystem; removed both
download cards from the dashboard and rewrote the user manual's runtime
guidance.

Kept on purpose: the `testing/` golden fixtures, which Go tests assert against,
and `generate_fixture.py`, which regenerates them and never imported the
retired runtime.

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
- Decide the final public GitHub repo URL (the placeholder link is gone; the Go About screen points at snyderb-de/content-list-toolkit)
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
