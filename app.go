package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const appVersion = "0.4.3"

type App struct {
	ctx            context.Context
	startDir       string
	cloneCancel    func()
	emailCancel    func()
	cloneDriveBCh  chan string // non-nil only while awaiting drive B in single-drive mode
	updateMu       sync.Mutex
	preparedUpdate *preparedUpdate

	// reachMu guards the one reachability answer this session keeps. Probing
	// is a network call, so it happens once and is remembered; asking again is
	// something the user does deliberately.
	reachMu sync.Mutex
	reach   *GettyReachability
	// probeClient and probeURL are empty in the application and set by tests,
	// so the once-per-session rule can be tested without reaching Getty.
	probeClient *http.Client
	probeURL    string
}

func newApp(startDir string) *App {
	return &App{startDir: startDir}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// EmailExtensions lists the file types Email Copy recognises.
//
// The screen used to carry this list as a sentence of its own, and it drifted:
// .emlxpart was added to the code and the sentence kept saying otherwise. A
// list of file types is a fact about the program, so the program answers it.
func (a *App) EmailExtensions() []string {
	return sortedEmailExtensions()
}

func (a *App) GetAppVersion() string {
	return appVersion
}

func (a *App) PickFolder(title string) string {
	dir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:                title,
		DefaultDirectory:     a.startDir,
		CanCreateDirectories: true,
	})
	if err != nil {
		return ""
	}
	return dir
}

func (a *App) OpenPath(path string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", path).Start() //nolint:errcheck
	case "windows":
		exec.Command("explorer", path).Start() //nolint:errcheck
	default:
		exec.Command("xdg-open", path).Start() //nolint:errcheck
	}
}

// RevealPath opens the folder containing a file and selects the file in it,
// which is what someone wants after a tool writes output: the file next to its
// siblings, ready to be moved, renamed, or attached.
//
// macOS and Windows can both select the file. Linux has no portable way to ask
// a file manager to do that, so it falls back to opening the folder.
func (a *App) RevealPath(path string) {
	if path == "" {
		return
	}
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", "-R", path).Start() //nolint:errcheck
	case "windows":
		// explorer exits non-zero even when it succeeds, so the error is
		// ignored here as it is in OpenPath.
		exec.Command("explorer", "/select,"+filepath.Clean(path)).Start() //nolint:errcheck
	default:
		exec.Command("xdg-open", filepath.Dir(path)).Start() //nolint:errcheck
	}
}

func (a *App) GetScanDefaults() ScanOptions {
	defaults := ScanOptions{
		SourceDir:     a.startDir,
		OutputDir:     a.startDir,
		HashAlgorithm: string(defaultHashAlgorithm()),
		ExcludeHidden: true,
		ExcludeSystem: true,
		CreateXLSX:    true,
		PreserveZeros: true,
		DeleteCSV:     true,
		AgencyFields:  defaultAgencyTemplateFields(),
		ReleaseFolder: defaultReleaseFolder,
	}
	saved, err := a.loadSettings()
	if err != nil {
		return defaults
	}
	if saved.HashAlgorithm != "" {
		defaults.HashAlgorithm = saved.HashAlgorithm
	}
	defaults.ExcludeHidden = saved.ExcludeHidden
	defaults.ExcludeSystem = saved.ExcludeSystem
	defaults.CreateXLSX = saved.CreateXLSX
	defaults.PreserveZeros = saved.PreserveZeros
	defaults.DeleteCSV = saved.DeleteCSV
	defaults.ExcludedExts = saved.ExcludedExts
	defaults.FoldersOnly = saved.FoldersOnly
	defaults.FolderDepth = saved.FolderDepth
	defaults.AgencyTemplate = saved.AgencyTemplate
	if strings.TrimSpace(saved.ReleaseFolder) != "" {
		defaults.ReleaseFolder = saved.ReleaseFolder
	}
	return defaults
}

func defaultAgencyTemplateFields() AgencyTemplateFields {
	return AgencyTemplateFields{
		MaterialType: "Born Digital",
		RecordLevel:  "Item",
	}
}

func (a *App) SaveSettings(opts ScanOptions) {
	s := AppSettings{
		HashAlgorithm:  opts.HashAlgorithm,
		ExcludeHidden:  opts.ExcludeHidden,
		ExcludeSystem:  opts.ExcludeSystem,
		CreateXLSX:     opts.CreateXLSX,
		PreserveZeros:  opts.PreserveZeros,
		DeleteCSV:      opts.DeleteCSV,
		ExcludedExts:   opts.ExcludedExts,
		FoldersOnly:    opts.FoldersOnly,
		FolderDepth:    opts.FolderDepth,
		AgencyTemplate: opts.AgencyTemplate,
		AgencyFields:   defaultAgencyTemplateFields(),
		ReleaseFolder:  strings.TrimSpace(opts.ReleaseFolder),
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	path, err := a.settingsPath()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, data, 0o644)
}

func (a *App) loadSettings() (AppSettings, error) {
	path, err := a.settingsPath()
	if err != nil {
		return AppSettings{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AppSettings{}, err
	}
	var s AppSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return AppSettings{}, err
	}
	return s, nil
}

func (a *App) settingsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "content-list-generator", "settings.json"), nil
}

func (a *App) StartScan(opts ScanOptions) error {
	excludedMap, err := parseExcludedExtensions(opts.ExcludedExts)
	if err != nil {
		return err
	}

	outputFile := strings.TrimSpace(opts.OutputFile)
	if outputFile == "" {
		if opts.FoldersOnly {
			outputFile = defaultFolderListFilename(opts.SourceDir)
		} else {
			outputFile = defaultOutputFilename(opts.SourceDir)
		}
	}
	if strings.ToLower(filepath.Ext(outputFile)) != ".csv" {
		outputFile += ".csv"
	}
	outputPath := filepath.Join(opts.OutputDir, outputFile)

	options := scanOptions{
		HashAlgorithm:    hashAlgorithm(opts.HashAlgorithm),
		ExcludeHidden:    opts.ExcludeHidden,
		ExcludeSystem:    opts.ExcludeSystem,
		CreateXLSX:       opts.CreateXLSX,
		PreserveZeros:    opts.PreserveZeros,
		DeleteCSV:        opts.DeleteCSV,
		ExcludedExts:     excludedMap,
		ExcludedExtsText: opts.ExcludedExts,
		FoldersOnly:      opts.FoldersOnly,
		FolderDepth:      opts.FolderDepth,
		AgencyTemplate:   opts.AgencyTemplate,
		AgencyFields: agencyTemplateFields{
			RG:               opts.AgencyFields.RG,
			SG:               opts.AgencyFields.SG,
			Series:           opts.AgencyFields.Series,
			RCSeries:         opts.AgencyFields.RCSeries,
			DeptOrganization: opts.AgencyFields.DeptOrganization,
			Division:         opts.AgencyFields.Division,
			Section:          opts.AgencyFields.Section,
			Unit:             opts.AgencyFields.Unit,
			RCSeriesName:     opts.AgencyFields.RCSeriesName,
			BeginDate:        opts.AgencyFields.BeginDate,
			EndDate:          opts.AgencyFields.EndDate,
			Description:      opts.AgencyFields.Description,
			Location:         opts.AgencyFields.Location,
			MaterialType:     opts.AgencyFields.MaterialType,
			Comments:         opts.AgencyFields.Comments,
			Confidential:     opts.AgencyFields.Confidential,
			DispositionDate:  opts.AgencyFields.DispositionDate,
			BoxNum:           opts.AgencyFields.BoxNum,
			TDNum:            opts.AgencyFields.TDNum,
			LocationID:       opts.AgencyFields.LocationID,
			RecordLevel:      opts.AgencyFields.RecordLevel,
		},
	}
	if options.AgencyTemplate {
		normalizedFields, err := normalizeAgencyTemplateFields(options.AgencyFields)
		if err != nil {
			return err
		}
		options.AgencyFields = normalizedFields
	}

	ctx, cancel := context.WithCancel(context.Background())
	token := setActiveScanCancel(cancel)

	go func() {
		defer clearActiveScanCancel(token)
		defer cancel()

		progressDone := make(chan struct{})
		go func() {
			defer close(progressDone)
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case t := <-ticker.C:
					stats := currentProgress()
					wailsRuntime.EventsEmit(a.ctx, "scan:progress", ScanProgressPayload{
						Phase:       progressPhaseLabel(stats.phase),
						Files:       stats.files,
						Directories: stats.directories,
						Bytes:       stats.bytes,
						Filtered:    stats.filtered,
						TotalFiles:  stats.totalFiles,
						TotalDirs:   stats.totalDirectories,
						TotalBytes:  stats.totalBytes,
						CurrentItem: stats.currentItem,
						Percent:     progressFraction(stats),
						ETASecs:     progressETA(stats, t).Seconds(),
						ElapsedSecs: time.Since(stats.startedAt).Seconds(),
					})
				}
			}
		}()

		done, scanErr := runScanWithContext(ctx, opts.SourceDir, outputPath, options)
		cancel()
		<-progressDone

		if scanErr != nil {
			if isContextCanceled(scanErr) {
				wailsRuntime.EventsEmit(a.ctx, "scan:canceled")
			} else {
				wailsRuntime.EventsEmit(a.ctx, "scan:error", scanErr.Error())
			}
			return
		}

		topByCount := make([]SummaryEntry, len(done.topByCount))
		for i, e := range done.topByCount {
			topByCount[i] = SummaryEntry{Label: e.Label, Count: e.Count, Bytes: e.Bytes}
		}
		topBySize := make([]SummaryEntry, len(done.topBySize))
		for i, e := range done.topBySize {
			topBySize[i] = SummaryEntry{Label: e.Label, Count: e.Count, Bytes: e.Bytes}
		}

		wailsRuntime.EventsEmit(a.ctx, "scan:done", ScanDonePayload{
			Files:           done.files,
			Directories:     done.directories,
			Bytes:           done.bytes,
			Errors:          done.errors,
			Filtered:        done.filtered,
			FilteredHidden:  done.filteredHidden,
			FilteredSystem:  done.filteredSystem,
			FilteredExts:    done.filteredExts,
			FilteredOSNoise: done.filteredOSNoise,
			SourceName:      done.sourceName,
			OutputPath:      done.outputPath,
			OutputPaths:     done.outputPaths,
			XLSXPath:        done.xlsxPath,
			XLSXPaths:       done.xlsxPaths,
			ReportPath:      done.reportPath,
			ElapsedSecs:     done.elapsed.Seconds(),
			TopByCount:      topByCount,
			TopBySize:       topBySize,
			HashWorkers:     done.hashWorkers,
			HashAlgorithm:   string(done.hashAlgorithm),
			CreateXLSX:      done.createXLSX,
			PreserveZeros:   done.preserveZeros,
			DeleteCSV:       done.deleteCSV,
			CSVDeleted:      done.csvDeleted,
			MaxRowsPerCSV:   done.maxRowsPerCSV,
			CSVPartCount:    done.csvPartCount,
			XLSXPartCount:   done.xlsxPartCount,
			FilteredSamples: done.filteredSamples,
			FirstCSVItem:    done.firstCSVItem,
			LastCSVItem:     done.lastCSVItem,
			FoldersOnly:     done.foldersOnly,
		})
	}()

	return nil
}

func (a *App) CancelScan() {
	cancelActiveScan()
}

func (a *App) CheckOutputExists(opts ScanOptions) bool {
	outputFile := strings.TrimSpace(opts.OutputFile)
	if outputFile == "" {
		if opts.FoldersOnly {
			outputFile = defaultFolderListFilename(opts.SourceDir)
		} else {
			outputFile = defaultOutputFilename(opts.SourceDir)
		}
	}
	if strings.ToLower(filepath.Ext(outputFile)) != ".csv" {
		outputFile += ".csv"
	}
	outputPath := filepath.Join(opts.OutputDir, outputFile)
	firstPart := csvOutputPathForPart(outputPath, 1)
	_, err := os.Stat(firstPart)
	return err == nil
}

func (a *App) ValidateScanPaths(sourceDir, outputDir string) string {
	if sourceDir == "" || outputDir == "" {
		return ""
	}
	info, err := os.Stat(sourceDir)
	if err != nil {
		return "Source folder does not exist."
	}
	if !info.IsDir() {
		return "Source path is not a folder."
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "Cannot create output folder: " + err.Error()
	}
	testFile := filepath.Join(outputDir, ".clg-write-test")
	f, err := os.Create(testFile)
	if err != nil {
		return "Output folder is not writable."
	}
	f.Close()
	_ = os.Remove(testFile)
	return ""
}

func (a *App) StartEmailCopy(source, dest string) error {
	ctx, cancel := context.WithCancel(context.Background())
	a.emailCancel = cancel
	go func() {
		defer cancel()
		defer func() { a.emailCancel = nil }()
		startedAt := time.Now()
		manifestPath, copied, err := copyEmailFilesWithProgress(ctx, source, dest, func(p emailCopyProgress) {
			wailsRuntime.EventsEmit(a.ctx, "email:progress", EmailProgressPayload{
				Phase:   p.Phase,
				Copied:  p.Copied,
				Scanned: p.Scanned,
				Matched: p.Matched,
				Total:   p.Total,
			})
		})
		if err != nil {
			if isContextCanceled(err) {
				wailsRuntime.EventsEmit(a.ctx, "email:canceled")
			} else {
				wailsRuntime.EventsEmit(a.ctx, "email:error", err.Error())
			}
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "email:done", EmailDonePayload{
			SourceDir:    source,
			DestDir:      dest,
			ManifestPath: manifestPath,
			Copied:       copied,
			ElapsedSecs:  time.Since(startedAt).Seconds(),
		})
	}()
	return nil
}

func (a *App) CancelEmailCopy() {
	if a.emailCancel != nil {
		a.emailCancel()
	}
}

// startCloneScanProgressTicker emits clone:progress events every 250 ms during a scan phase.
// Returns a stop func that blocks until the goroutine exits.
func (a *App) startCloneScanProgressTicker(drivePhase string) func() {
	tickCtx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		var prevBytes uint64
		var prevTick time.Time
		for {
			select {
			case <-tickCtx.Done():
				return
			case t := <-ticker.C:
				stats := currentProgress()
				var bps float64
				if !prevTick.IsZero() {
					dt := t.Sub(prevTick).Seconds()
					if dt > 0 && stats.bytes >= prevBytes {
						bps = float64(stats.bytes-prevBytes) / dt
					}
				}
				prevBytes = stats.bytes
				prevTick = t
				wailsRuntime.EventsEmit(a.ctx, "clone:progress", CloneProgressPayload{
					Phase:       drivePhase,
					SubPhase:    progressPhaseLabel(stats.phase),
					Percent:     progressFraction(stats),
					CurrentItem: stats.currentItem,
					Files:       stats.files,
					TotalFiles:  stats.totalFiles,
					Bytes:       stats.bytes,
					TotalBytes:  stats.totalBytes,
					BytesPerSec: bps,
					ETASecs:     progressETA(stats, t).Seconds(),
					ElapsedSecs: time.Since(stats.startedAt).Seconds(),
				})
			}
		}
	}()
	return func() { stop(); <-done }
}

func (a *App) StartCloneCompare(opts CloneCompareOptions) error {
	if opts.DriveB != "" && filepath.Clean(opts.DriveA) == filepath.Clean(opts.DriveB) {
		return fmt.Errorf("1st Drive and 2nd Drive must be different folders")
	}
	go func() {
		hashAlgo := hashAlgorithm(opts.HashAlgorithm)
		if hashAlgo == "" {
			hashAlgo = hashAlgorithmBLAKE3
		}

		scanOpts := scanOptions{
			HashAlgorithm: hashAlgo,
			ExcludeHidden: true,
			ExcludeSystem: true,
			DeleteCSV:     false,
		}

		outputFile := defaultOutputFilename(opts.DriveA)
		outputPath := filepath.Join(opts.OutputDir, outputFile)

		ctx, cancel := context.WithCancel(context.Background())
		token := setActiveScanCancel(cancel)
		a.cloneCancel = cancel

		wailsRuntime.EventsEmit(a.ctx, "clone:progress", CloneProgressPayload{Phase: "scan-a"})
		stopTickerA := a.startCloneScanProgressTicker("scan-a")
		driveAResult, err := runScanWithContext(ctx, opts.DriveA, outputPath, scanOpts)
		stopTickerA()
		if err != nil {
			clearActiveScanCancel(token)
			cancel()
			a.cloneCancel = nil
			if isContextCanceled(err) {
				wailsRuntime.EventsEmit(a.ctx, "clone:canceled")
			} else {
				wailsRuntime.EventsEmit(a.ctx, "clone:error", err.Error())
			}
			return
		}

		driveB := opts.DriveB
		if driveB == "" {
			// Single-drive mode: pause and wait for user to swap drives and provide Drive B path.
			ch := make(chan string, 1)
			a.cloneDriveBCh = ch
			wailsRuntime.EventsEmit(a.ctx, "clone:awaiting-drive-b", nil)
			select {
			case path, ok := <-ch:
				a.cloneDriveBCh = nil
				if !ok {
					clearActiveScanCancel(token)
					cancel()
					a.cloneCancel = nil
					wailsRuntime.EventsEmit(a.ctx, "clone:canceled")
					return
				}
				driveB = path
			case <-ctx.Done():
				a.cloneDriveBCh = nil
				clearActiveScanCancel(token)
				a.cloneCancel = nil
				wailsRuntime.EventsEmit(a.ctx, "clone:canceled")
				return
			}
		}

		wailsRuntime.EventsEmit(a.ctx, "clone:progress", CloneProgressPayload{Phase: "scan-b"})
		stopTickerB := a.startCloneScanProgressTicker("scan-b")
		driveBOutputPath := cloneOutputPathForDriveB(outputPath)
		driveBResult, err := runScanWithContext(ctx, driveB, driveBOutputPath, scanOpts)
		stopTickerB()
		clearActiveScanCancel(token)
		if err != nil {
			cancel()
			a.cloneCancel = nil
			if isContextCanceled(err) {
				wailsRuntime.EventsEmit(a.ctx, "clone:canceled")
			} else {
				wailsRuntime.EventsEmit(a.ctx, "clone:error", err.Error())
			}
			return
		}

		compareCtx, compareCancel := context.WithCancel(context.Background())
		a.cloneCancel = compareCancel
		cancel()

		diffPath := cloneDiffCSVPath(outputPath)
		reportPath := cloneDiffReportPath(outputPath)

		result, compareErr := compareScanOutputs(
			compareCtx,
			driveAResult,
			driveBResult,
			diffPath,
			reportPath,
			opts.SoftCompare,
			func(p cloneCompareProgress) {
				total := driveAResult.files
				if driveBResult.files > total {
					total = driveBResult.files
				}
				wailsRuntime.EventsEmit(a.ctx, "clone:progress", CloneProgressPayload{
					Phase:       "diff",
					Percent:     compareProgressFraction(p),
					Compared:    p.compared,
					Total:       total,
					Differences: p.differences,
					CurrentItem: p.currentItem,
				})
			},
			func(row DiffRowPayload) {
				wailsRuntime.EventsEmit(a.ctx, "clone:diff-row", row)
			},
		)
		compareCancel()
		a.cloneCancel = nil

		if compareErr != nil {
			if isContextCanceled(compareErr) {
				wailsRuntime.EventsEmit(a.ctx, "clone:canceled")
			} else {
				wailsRuntime.EventsEmit(a.ctx, "clone:error", compareErr.Error())
			}
			return
		}

		wailsRuntime.EventsEmit(a.ctx, "clone:done", CloneDonePayload{
			DiffPath:          result.diffPath,
			ReportPath:        result.reportPath,
			HashAlgorithm:     string(result.hashAlgorithm),
			ElapsedSecs:       result.elapsed.Seconds(),
			Verdict:           string(result.verdict),
			Compared:          result.compared,
			Differences:       result.differences,
			MovedFiles:        result.movedFiles,
			DuplicatesOnB:     result.duplicatesOnB,
			DuplicatesOnA:     result.duplicatesOnA,
			MissingNoMatch:    result.missingNoMatch,
			ExtraNoMatch:      result.extraNoMatch,
			SizeMismatches:    result.sizeMismatches,
			HashMismatches:    result.hashMismatches,
			ExcludedSystem:    result.excludedSystem,
			MetadataOnlyDiffs: result.metadataOnlyDiffs,
			SoftCompare:       result.softCompare,
		})
	}()
	return nil
}

func (a *App) ResumeCloneWithDriveB(path string) error {
	ch := a.cloneDriveBCh
	if ch == nil {
		return fmt.Errorf("no clone compare awaiting drive B")
	}
	ch <- path
	return nil
}

func (a *App) CancelCloneCompare() {
	if ch := a.cloneDriveBCh; ch != nil {
		a.cloneDriveBCh = nil
		close(ch)
	}
	if a.cloneCancel != nil {
		a.cloneCancel()
	}
	cancelActiveScan()
}

func isContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
