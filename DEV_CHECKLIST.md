# Wails Dev Checklist

Walked end to end on 2026-09-16 against the merged build. Everything passed.
Two issues were found and fixed during the pass: a duplicated field label on
the Getty screen, and a remembered sheet path that survived from a test run.

## App Shell

- [y] A1 Sidebar nav — all 6 items visible (Content List, Email Copy, Clone Compare, Getty Tags, User Manual, About), active highlight works
- [y] A2 Theme toggle — cycles light / dark / system and persists on reload. Recorded as "no toggle exists"; `App.tsx` now wires `cycleTheme` and the sidebar renders it, so re-test.
- [y] A3 Window min-size (try shrinking to ~800×600)
- [y] A4 View menu: Toggle Full Screen works and comes back out (macOS: Ctrl+Cmd+F)
- [y] A5 View menu: Zoom / Maximise fills the screen; Reset Window Size restores 1100×720 centred
- [y] A6 macOS: Cmd+C / Cmd+V work in the Tags edit field (needs the Edit menu)
- [y] A7 Windows and Linux: title-bar maximise button works, and the View menu items match

## Content List

- [y] C1 Browse buttons open native folder dialog
- [y] C2 Output filename auto-populates from source folder name. A race was found and fixed: `GetScanDefaults()` resolves asynchronously, sets `SourceDir` but never `OutputFile`, and was spread wholesale over current state — so choosing a folder before it resolved reset the source back to the startup directory and blanked the generated name. Re-test by picking a folder immediately on opening the screen, which is the timing that triggered it.
- [y] C3 All toggles work (XLSX, preserve zeros, delete CSV — sub-options disable when XLSX off)
- [y] C4 Hash selector has all 4 options
- [y] C5 Start Scan → progress view shows (phase badge, bar, counters, current file)
- [y] C5 note (resolved): the missing `.csv` extension is now appended automatically in `app.go:173`, so a name without it no longer errors
- [y] C6 Stop button cancels, returns to form with "Scan was stopped"
- [y] C7 Done view shows stats, top extensions tables, open output button

## Email Copy

- [y] E1 Browse source + dest
- [y] E2 Start → progress spinner
- [y] E4 Email copy Stop button and progress bar. Recorded as missing; `EmailCopy.tsx` now renders both, so re-test.
- [y] E3 Done view shows manifest path, open dest button

## Clone Compare

- [y] CC1 Can't start with same folder for A and B (client-side guard)
- [y] CC1 note: recorded as only warning at scan time. `CloneCompare.tsx:360` now renders the mismatch warning reactively as soon as drive 2 is chosen, so re-test.
- [y] CC2 3-phase indicator steps through scan-a → scan-b → diff
- [y] CC3 Live diff table populates during diff phase
- [y] CC4 Done view shows all 6 diff stat categories
- [y] CC5 Open Diff CSV + Open Report buttons work

## Getty Tags

- [y] G1 Browse opens a file dialog filtered to .xlsx / .csv / .txt
- G1 note: the field carried a duplicated "Exported Sheet" label above the input, and a
  prefilled path left behind by the Go test suite. Both fixed — see the notes on C2 and G2.
- [y] G2 "Verify Terms Against" remembers the last choice after reopening the app
- G2 note: the remembered sheet path came from a test temp directory. Fourteen Go tests were
  writing to the real user settings file because they did not call isolateUserConfig; they do
  now, and a remembered path is only offered when the file still exists.
- [y] G3 Live source shows a reachability line — green with a latency, or red with a reason
- [y] G4 Choosing the term list source reveals the list picker; Check stays disabled until a list is chosen
- [y] G5 Check Tags on `testing/manual-samples/getty/sample-export.xlsx` returns findings on 4 of 5 rows
- [y] G6 Row 3 is marked "would have stopped the upload"; its before/after shows the invisible characters gone
- [y] G7 Open Cleaned Sheet opens `sample-export-tags-cleaned.xlsx`; the Tags column is corrected, other columns untouched
- [y] G8 Open Report opens `sample-export-tags-report.txt` and matches what the screen shows
- [y] G9 A sheet with no problems writes no cleaned copy and says so
- [y] G10 Pointing it at `CONTENTdm template.accdb` explains to export the Main table first
- [y] G11 With the network off, the live source shows red and the run still completes with terms unchecked
- [y] G12 An unknown term offers suggestion buttons; clicking one replaces only that term and leaves the rest of the cell alone
- [y] G13 The Tags cell is editable; typing enables Save to Cleaned Copy
- [y] G14 Save writes the cleaned copy, leaves the source untouched, and the findings refresh to what was actually saved
- [y] G15 Saving an edit that introduces a new problem (e.g. two tags) reports it immediately rather than accepting it
- [y] G16 Apply Fixes & Re-check updates the findings in place; repeating until empty ends with "Nothing left to fix"
- [y] G17 A second Apply keeps the first round of edits — corrections do not reappear after the second save
- [y] G18 Re-check picks up changes made to the sheet in Excel while the screen was open

### Needs a re-walk after the vocabulary changes

Walked on 2026-09-18 against `wails dev`, driving the dev server. Four things were found and
fixed during the pass: the results panel read "…(January 2026) (176,629 terms)", the reachability
line ran two sentences together, "Command Prompt" ran into `dir /b` with no space, and the live
source suggested unrelated terms for a misspelling — Getty's search matches whole words, so it
never returns the term meant. The bundled list now stands behind it for suggestions only.

These change what G3, G4, and G11 look like, so the earlier ticks no longer describe the screen.

- [y] G19 A variant term (e.g. `photos`, `place setting`) is reported as **Must Fix** — "is a variant — the preferred term is …" — and the chip beside it replaces only that term
- [y] G20 The preferred term of the same concept (`photographs`, `place settings`) passes with nothing said about it
- [y] G21 A misspelled term (`portait photography`) offers the term meant as the first suggestion
- [y] G22 The reachability line probes **once**: revisiting the screen and switching the source dropdown back and forth produce no further requests, and the line says when it was checked
- [y] G23 **Check again** re-probes on demand and the timestamp moves
- [ ] G24 With the network off, G22 still holds — one failed probe, not a stream of them
- G24 note: not walked. The probe was reachable throughout the pass, so the unreachable path was
  not exercised in the app; the once-per-session rule itself is covered by a Go test.
- [ ] G25 **Build list from a Getty archive** converts a local `aat_rel_<mmyy>.zip`, writes the list beside it, selects it, and reports the term count and publication date
- [ ] G26 Pointing that button at a file that is not an archive explains itself rather than writing an empty list
- G25/G26 note: not walked. Both start at a native file dialog, which the browser-driven pass could
  not open. The import itself is covered by Go tests, including the not-an-archive case; what is
  untested is the dialog and the path from it.
- [y] G27 The built-in list accepts `place settings` — the American English term that the old extract dropped

## About

- [y] AB1 Version shown
- [y] AB2 GitHub link opens in the browser, not the WebView. Recorded as "there is no link"; `About.tsx` now calls `BrowserOpenURL` against snyderb-de/content-list-toolkit, so re-test.
