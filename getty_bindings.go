package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Bindings for the Getty Tag screen.
//
// The check runs to completion and returns one result rather than streaming
// progress, because a collection sheet is thousands of rows rather than the
// hundreds of thousands of files a content scan walks, and the vocabulary cache
// collapses the repeated terms. If a sheet ever appears that takes long enough
// to need a progress bar, the work already returns row by row and can be
// emitted as events without changing the checking code.

// gettyVocabularySource names where the screen wants terms verified.
type gettyVocabularySource string

const (
	// gettySourceNone checks structure and invisible characters only, which
	// needs neither the network nor a term list.
	gettySourceNone gettyVocabularySource = "none"
	// gettySourceLive asks vocab.getty.edu, which is authoritative.
	gettySourceLive gettyVocabularySource = "live"
	// gettySourceFile uses an exported term list, which works offline.
	gettySourceFile gettyVocabularySource = "file"
)

// GettyCheckOptions is what the screen sends when the user presses Check.
type GettyCheckOptions struct {
	SheetPath      string                `json:"sheetPath"`
	Source         gettyVocabularySource `json:"source"`
	VocabularyPath string                `json:"vocabularyPath"`
	WriteCleaned   bool                  `json:"writeCleaned"`
	WriteReport    bool                  `json:"writeReport"`
}

// GettyCheckResult is what comes back, including where the outputs landed.
type GettyCheckResult struct {
	Report      TagSheetReport `json:"report"`
	Summary     string         `json:"summary"`
	CleanedPath string         `json:"cleanedPath,omitempty"`
	ReportPath  string         `json:"reportPath,omitempty"`
	// Elapsed is rendered by the screen rather than computed there, so the
	// units cannot drift between runtimes.
	Elapsed string `json:"elapsed"`
}

// PickSheet opens a file dialog filtered to the exports this check accepts.
func (a *App) PickSheet(title string) string {
	path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: a.startDir,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Access exports (*.xlsx, *.csv, *.txt)", Pattern: "*.xlsx;*.xlsm;*.csv;*.txt;*.tsv;*.tab"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return ""
	}
	return path
}

// CheckGettyReachability tells the screen whether the live source is usable
// from this machine, so the choice is visible before a run rather than
// discovered through a sheet full of unchecked terms.
func (a *App) CheckGettyReachability() GettyReachability {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return probeGettyReachability(ctx, nil, "")
}

// SaveGettyVocabulary remembers the chosen source for next time.
func (a *App) SaveGettyVocabulary(source gettyVocabularySource, path string) error {
	settings, err := a.loadSettings()
	if err != nil {
		settings = defaultAppSettings()
	}
	settings.GettySource = string(source)
	settings.GettyVocabularyPath = path
	return a.writeSettings(settings)
}

// GetGettyDefaults gives the screen its remembered state on open.
func (a *App) GetGettyDefaults() GettyCheckOptions {
	options := GettyCheckOptions{
		Source:       gettySourceLive,
		WriteCleaned: true,
		WriteReport:  true,
	}
	settings, err := a.loadSettings()
	if err != nil {
		return options
	}
	if settings.GettySource != "" {
		options.Source = gettyVocabularySource(settings.GettySource)
	}
	options.VocabularyPath = settings.GettyVocabularyPath
	return options
}

// CheckGettyTags runs the whole check and writes whichever outputs were asked
// for.
//
// Writing is deliberately not all-or-nothing with the check: if the cleaned
// copy cannot be written because the file is open in Excel, the findings are
// still worth returning, so the failure is reported as an error only after the
// report has been produced.
func (a *App) CheckGettyTags(options GettyCheckOptions) (GettyCheckResult, error) {
	started := time.Now()

	if options.SheetPath == "" {
		return GettyCheckResult{}, fmt.Errorf("choose an exported sheet to check")
	}
	if _, err := os.Stat(options.SheetPath); err != nil {
		return GettyCheckResult{}, fmt.Errorf("%s cannot be opened", filepath.Base(options.SheetPath))
	}

	vocabulary, err := a.buildVocabulary(options)
	if err != nil {
		return GettyCheckResult{}, err
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	report, err := checkTagSheetWithVocabulary(ctx, options.SheetPath, vocabulary)
	if err != nil {
		return GettyCheckResult{}, err
	}

	result := GettyCheckResult{
		Report:  report,
		Summary: report.Summary(),
		Elapsed: time.Since(started).Round(time.Millisecond).String(),
	}

	// A sheet with nothing to correct gets no cleaned copy: writing an
	// identical file would only invite confusion about which one to upload.
	if options.WriteCleaned && report.RepairedRows() > 0 {
		cleanedPath := gettyCleanedPath(options.SheetPath)
		if err := writeCleanedSheet(report, nil, cleanedPath); err != nil {
			return result, fmt.Errorf("checked the sheet, but could not write the cleaned copy: %w", err)
		}
		result.CleanedPath = cleanedPath
	}
	if options.WriteReport {
		reportPath := gettyReportPath(options.SheetPath)
		if err := writeGettyTagReport(reportPath, report); err != nil {
			return result, fmt.Errorf("checked the sheet, but could not write the report: %w", err)
		}
		result.ReportPath = reportPath
	}

	return result, nil
}

// buildVocabulary turns the chosen source into something to look terms up in.
// Every source is wrapped in the cache, since a sheet repeats the same handful
// of terms across every row.
func (a *App) buildVocabulary(options GettyCheckOptions) (gettyVocabulary, error) {
	switch options.Source {
	case gettySourceNone, "":
		return nil, nil
	case gettySourceLive:
		return newCachedVocabulary(newSPARQLVocabulary(nil, "")), nil
	case gettySourceFile:
		if options.VocabularyPath == "" {
			return nil, fmt.Errorf("choose a term list, or switch to checking against Getty directly")
		}
		list, err := loadVocabularyFile(options.VocabularyPath)
		if err != nil {
			return nil, err
		}
		return newCachedVocabulary(list), nil
	default:
		return nil, fmt.Errorf("unknown vocabulary source %q", options.Source)
	}
}

// GettyTagEdit is one row corrected by hand on the screen.
type GettyTagEdit struct {
	Row  int    `json:"row"`
	Tags string `json:"tags"`
}

// GettySaveResult reports where the corrected sheet was written and what it
// still contains, so the screen can show the result of saving rather than
// claiming success blindly.
type GettySaveResult struct {
	CleanedPath string         `json:"cleanedPath"`
	Report      TagSheetReport `json:"report"`
	Summary     string         `json:"summary"`
}

// SaveGettyTagEdits writes the cleaned copy with hand corrections applied.
//
// It writes to the cleaned copy, never to the source. The export is the record
// of what Access held, and a tool that edits it in place destroys the only
// thing a correction can be checked against.
//
// The sheet is re-checked afterwards and the fresh report returned, so the
// screen shows what the saved file actually contains rather than what the
// edits were expected to achieve.
func (a *App) SaveGettyTagEdits(options GettyCheckOptions, edits []GettyTagEdit) (GettySaveResult, error) {
	if options.SheetPath == "" {
		return GettySaveResult{}, fmt.Errorf("choose an exported sheet to check")
	}

	vocabulary, err := a.buildVocabulary(options)
	if err != nil {
		return GettySaveResult{}, err
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	report, err := checkTagSheetWithVocabulary(ctx, options.SheetPath, vocabulary)
	if err != nil {
		return GettySaveResult{}, err
	}

	byRow := make(map[int]string, len(edits))
	for _, edit := range edits {
		if strings.TrimSpace(edit.Tags) == "" {
			continue
		}
		byRow[edit.Row] = edit.Tags
	}

	cleanedPath := gettyCleanedPath(options.SheetPath)
	if err := writeCleanedSheet(report, byRow, cleanedPath); err != nil {
		return GettySaveResult{}, fmt.Errorf("could not write the cleaned copy: %w", err)
	}

	// Re-check what was actually written. An edit can introduce a new problem
	// just as easily as it fixes one, and saying so immediately is better than
	// letting it reach CONTENTdm.
	saved, err := checkTagSheetWithVocabulary(ctx, cleanedPath, vocabulary)
	if err != nil {
		return GettySaveResult{CleanedPath: cleanedPath}, fmt.Errorf("wrote %s but could not re-check it: %w",
			filepath.Base(cleanedPath), err)
	}

	// The re-check finds a corrected sheet clean, which is the point — but a
	// report saying only "no problems found" is not a record of anything, and
	// it overwrites the one that described the problems. Carry the corrections
	// across so the report accounts for them.
	saved.SourcePath = options.SheetPath
	saved.Changes = collectTagChanges(report, byRow)

	if options.WriteReport {
		if err := writeGettyTagReport(gettyReportPath(options.SheetPath), saved); err != nil {
			return GettySaveResult{CleanedPath: cleanedPath, Report: saved}, fmt.Errorf("wrote the sheet but could not write the report: %w", err)
		}
	}

	return GettySaveResult{
		CleanedPath: cleanedPath,
		Report:      saved,
		Summary:     saved.Summary(),
	}, nil
}

// collectTagChanges pairs every altered cell with what it held before.
//
// The "before" is read from the original check of the source, so it is the
// text as Access exported it rather than as some intermediate pass left it.
func collectTagChanges(source TagSheetReport, edits map[int]string) []TagChange {
	before := make(map[int]string, len(source.Rows))
	for _, row := range source.Rows {
		before[row.Number] = row.Result.Original
	}

	var changes []TagChange
	for _, row := range source.Rows {
		edited, byHand := edits[row.Number]
		after := row.Result.Cleaned
		if byHand {
			after = checkTagCell(edited).Cleaned
		}
		if after == row.Result.Original {
			continue
		}
		changes = append(changes, TagChange{
			Row:    row.Number,
			Before: row.Result.Original,
			After:  after,
			ByHand: byHand,
		})
	}

	// An edit can land on a row the check found nothing wrong with.
	for number, edited := range edits {
		if _, alreadyListed := before[number]; alreadyListed {
			continue
		}
		changes = append(changes, TagChange{
			Row:    number,
			Before: "(no findings on this row)",
			After:  checkTagCell(edited).Cleaned,
			ByHand: true,
		})
	}
	return changes
}
