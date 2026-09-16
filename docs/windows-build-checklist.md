# Windows Release Build Checklist

Step-by-step for building all Windows artifacts on a clean Windows machine. Assumes nothing installed, nothing cloned.

Estimated time: 45–60 min for first run (mostly downloads). 5–10 min on subsequent runs.

---

## Phase 1 — One-time prerequisite install

Open PowerShell as your normal user (NOT admin, unless noted).

### [ ] 1. Install Git for Windows
- Download: https://git-scm.com/download/win
- Run the installer with defaults
- Verify in a NEW PowerShell window:
  ```powershell
  git --version
  ```

### [ ] 2. Install Go 1.26
- Download `go1.26.0.windows-amd64.msi` from https://go.dev/dl/
- Run the installer (default path `C:\Program Files\Go`)
- Open a NEW PowerShell window and verify:
  ```powershell
  go version
  # expect: go version go1.26.0 windows/amd64
  ```

### [ ] 3. Install Node.js 24 LTS
- Download Node 24.x LTS installer from https://nodejs.org/
- Run the installer with defaults
- Open a NEW PowerShell window and verify:
  ```powershell
  node --version
  npm --version
  ```

### [ ] 5. Verify WebView2 runtime present
- Win11 has it by default. Win10 may need it.
- Run:
  ```powershell
  Get-AppxPackage -Name "*WebView2*"
  ```
- If empty, install the "Evergreen Standalone Installer" from:
  https://developer.microsoft.com/en-us/microsoft-edge/webview2/

### [ ] 6. Install Wails CLI
```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
```

### [ ] 7. Add Go bin directory to PATH
- Find your GOPATH:
  ```powershell
  go env GOPATH
  # typically: C:\Users\<you>\go
  ```
- Add `<GOPATH>\bin` to your PATH:
  - Settings → System → About → Advanced system settings → Environment Variables
  - Under "User variables", edit `Path`, add `C:\Users\<you>\go\bin`
- Open a NEW PowerShell window and verify:
  ```powershell
  wails version
  # expect: v2.16.0
  ```

### [ ] 8. Install GitHub CLI (optional, for publishing)
- Download from https://cli.github.com/
- Run installer
- Authenticate once:
  ```powershell
  gh auth login
  ```

### [ ] 9. Run Wails doctor
```powershell
wails doctor
```
Confirms Go, npm, WebView2 all detected. Fix any reds before proceeding.

---

## Phase 2 — Clone the repo

### [ ] 1. Choose a workspace
```powershell
cd $env:USERPROFILE
mkdir code -ErrorAction SilentlyContinue
cd code
```

### [ ] 2. Clone
```powershell
git clone https://github.com/snyderb-de/content-list-toolkit.git
cd content-list-generator
```

### [ ] 3. Confirm on main
```powershell
git status
# expect: On branch main, nothing to commit
```

---

## Phase 3 — Build the Wails GUI (.exe)

### [ ] 1. Build the Windows executable
```powershell
wails build -platform windows/amd64 -clean -o "content-list-generator.exe"
```
- Output: `build\bin\content-list-generator.exe`
- Takes ~2–3 min on first run (frontend npm install + Go compile)

### [ ] 2. Stage output for release
```powershell
New-Item -ItemType Directory -Force -Path releases\windows-go | Out-Null
Copy-Item "build\bin\content-list-generator.exe" "releases\windows-go\content-list-generator.exe"
```

### [ ] 3. Smoke test
- Double-click `releases\windows-go\content-list-generator.exe`
- Window opens, title reads "Content List Toolkit"
- Click "Generate" → browse a small folder → confirm CSV writes

### [ ] 4. Expected SmartScreen warning
- First launch will show "Windows protected your PC" because the .exe is unsigned
- Click "More info" → "Run anyway"
- Code-signing is a separate backlog item — see `TODO.md`

---

## Phase 5 — Publish to GitHub Release (optional)

You only need this if cutting a public release. Skip for local-only builds.

### [ ] 1. Tag the release locally (on main, AFTER all builds succeed)
```powershell
git tag v0.1.0
git push origin v0.1.0
```

### [ ] 2. Upload artifacts via gh CLI (Git Bash needed for the .sh script)
Open Git Bash:
```bash
./scripts/publish_github_release.sh v0.1.0 --target main
```
This uploads everything under `releases/` to the GitHub Release matching the tag.

### [ ] 3. Or upload manually
- Go to https://github.com/snyderb-de/content-list-toolkit/releases
- Click "Draft a new release"
- Choose tag `v0.1.0`
- Drag and drop the `.exe` files from `releases\windows-go\`
- Publish

---

## Phase 6 — When the GitHub Actions CI is working

Eventually, this whole checklist becomes unnecessary. CI does it automatically:
- Push a tag → workflow `.github/workflows/release.yml` runs on `windows-latest` runners
- Builds Wails GUI (amd64 + arm64)
- Publish job attaches them to the GitHub Release

Use this checklist when:
- CI is red and you need a release today
- You want to test a build before tagging
- You're debugging a CI failure by reproducing locally

---

## Troubleshooting

### `wails: command not found`
- Add `<GOPATH>\bin` to PATH (Phase 1 step 7)
- Open a NEW PowerShell window

### `WebView2 runtime not found`
- Install the Evergreen runtime from Microsoft (Phase 1 step 5)

### `npm error code ERESOLVE` during `wails build`
- Delete `frontend\node_modules` and `frontend\package-lock.json`, retry
- If still failing, check for a recent dependabot bump that conflicts

### `flag provided but not defined: -gui` during build
- Non-fatal warning from wails binding generation. Build should still complete. Ignore.

### Antivirus blocks the .exe
- Add the build/bin directory to your antivirus allowlist
- Or sign the binary (backlog item — `TODO.md`)

### `go: cannot find go.mod`
- You're not in the repo root. `cd content-list-generator` first.

### Slow first build
- Normal. First `wails build` downloads all Go modules + runs `npm install`. Subsequent builds are 10–20 sec.

---

## Quick reference card

```powershell
# Full Windows release in 2 commands (after Phase 1 prereqs):
git pull
wails build -platform windows/amd64 -clean -o "content-list-generator.exe"
```

Outputs:
- `build\bin\content-list-generator.exe`
