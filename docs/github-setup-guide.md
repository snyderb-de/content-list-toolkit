# GitHub Setup Guide

A reference for the GitHub infrastructure around this repo: branch protection, dependency automation, code owners, and a primer on GitHub Actions. Written for someone new to these tools.

---

## Branch protection

Branch protection is a GitHub setting on the repo (not in code — configured in the web UI). Once enabled on `main`, it enforces rules before anyone can merge or push to that branch.

### Recommended rules

- **Require pull request before merging** — no direct pushes to `main`, everything goes through a PR
- **Require status checks to pass** — the CI workflow must succeed before the merge button enables
- **Require branches to be up to date** — PR must include latest `main` commits before merging (catches conflicts early)
- **Include administrators** — applies the rules to you too, not just contributors

### How to turn it on

1. GitHub repo → Settings → Branches
2. Click "Add branch protection rule"
3. Branch name pattern: `main`
4. Check the boxes above
5. Under "Require status checks", search for the job names from `release.yml` (`unix-bundle`, `windows-gui`) — add the ones you want as required
6. Save

After the first CI run, the job names will appear in the search box. Before that first run, the dropdown is empty. So: push the workflow first, let it run once, then come back to add the required checks.

### Why this matters

Without branch protection, you (or a future collaborator) can accidentally merge a PR with red CI, or push broken code straight to `main` and break the release tag flow.

---

## Renovate vs. Dependabot

Both do the same core job: automatically open PRs to bump dependency versions. They're competitors.

### Dependabot (currently configured)

- Built into GitHub, no setup beyond the YAML file
- Free, integrated UI ("Dependabot alerts" tab)
- Limited config — grouping rules are basic, scheduling is simple
- Good enough for ~80% of repos

### Renovate (Mend's tool, free for open source)

- More powerful — supports more package managers, finer grouping, regex-based rules, version range pinning strategies, custom changelogs in the PR body
- Can group across ecosystems (e.g., bump React + React-DOM + @types/react together in one PR)
- Can auto-merge patches if CI passes (Dependabot can do this too but more clunky)
- Requires installing the Renovate GitHub App (one click) plus a `renovate.json` config file
- Steeper learning curve

### When to pick Renovate over Dependabot

- You have 10+ packages bumping per week and Dependabot PRs feel noisy
- You want auto-merge for safe patches without writing extra GitHub Actions
- You need ecosystems Dependabot doesn't support (e.g., Helm charts, Terraform modules)

### Current recommendation

Stick with Dependabot for now. It's already configured at `.github/dependabot.yml`. If after a month you find yourself drowning in PRs or fighting the grouping rules, switch to Renovate. The migration is straightforward — disable Dependabot in settings, install the Renovate app, drop in a `renovate.json`.

---

## CODEOWNERS

This is a file at `.github/CODEOWNERS` that maps file paths to GitHub usernames or teams. When someone opens a PR that touches a path matched in the file, the listed owners are **automatically requested as reviewers**.

### Example file

```
# Default — anything not matched below
*                           @yourusername

# Go backend
*.go                        @yourusername
core.go                     @yourusername @anotherusername

# Python files need both you and someone else

# Frontend
frontend/                   @frontend-team-member

# CI workflows need extra scrutiny
.github/                    @yourusername
```

### What it gets you

- Auto reviewer assignment — no one has to remember to add you
- Combined with branch protection ("Require review from Code Owners"), it enforces that the listed owner must approve before merge
- Documents who's responsible for what code

### When you need it

- Multiple collaborators on the repo
- You want to enforce "frontend changes need frontend-team review"

### When you don't

- Solo repo (just you) — there's no one else to assign, so it's pointless

### Current recommendation

Skip CODEOWNERS for now. It's only useful once you have collaborators. Revisit when you transfer ownership to `dpa-snyder` (on your TODO backlog) or add other contributors.

---

## GitHub Actions primer

The basics in plain English:

### Core concepts

- A **workflow** is the YAML file in `.github/workflows/`. The filename doesn't matter; the `name:` at the top is what shows in the UI.
- **Triggers** (`on:`) tell GitHub when to run the workflow. The repo's workflow runs on PR, on tag push, or when you click "Run workflow" in the Actions tab.
- A **job** is an independent task. Jobs run in parallel on separate fresh virtual machines unless you say otherwise with `needs:`.
- A **runner** is the VM (`macos-latest`, `windows-latest`, `ubuntu-latest`). Free public-repo minutes are unlimited on Linux, more limited on macOS/Windows but still generous.
- **Steps** inside a job run sequentially. Each `uses:` pulls in a reusable action from the marketplace; each `run:` is a shell command.
- **Artifacts** are files you upload from one job (`actions/upload-artifact`) and download in another (`actions/download-artifact`). They persist for 90 days by default.
- **Secrets** (like `GITHUB_TOKEN`) are injected via `env:` and never logged.

### What you'll actually do

1. Commit the workflow file.
2. Push to a branch and open a PR — the Actions tab will show jobs running in real time. Each job is clickable, showing live logs.
3. Failures will be loud (red X next to the PR). Click the job, find the failing step, read the log.
4. When all green and merged, tag `v0.2.0` (`git tag v0.2.0 && git push --tags`) and the publish job will create the GitHub Release with all artifacts attached.

The first run will probably surface 1–2 issues (Go version availability, Wails ARM64 quirks, path differences on Windows). That's normal — fix them one at a time. Once it's green, subsequent runs Just Work.

---

## Summary of files added to the repo

| File | Purpose |
|---|---|
| `.github/workflows/release.yml` | CI matrix — builds all platform artifacts on PR and tag |
| `.github/dependabot.yml` | Weekly dependency update PRs across gomod / npm / pip / actions |
| `docs/github-setup-guide.md` | This document |

## Things still to configure in the GitHub web UI

- [ ] Turn on branch protection for `main` (Settings → Branches)
- [ ] Add required status checks after first CI run
- [ ] (Later, when collaborators join) Add `.github/CODEOWNERS`
- [ ] (Later) Consider Renovate if Dependabot PRs feel noisy

## Things still in `TODO.md`

- [ ] Build first Windows Wails GUI .exe (top priority — pinned)
- [ ] Code-signing cert for Windows .exe (avoid SmartScreen warnings)
- [ ] Auto-update mechanism for Wails app
