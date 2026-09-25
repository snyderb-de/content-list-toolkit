package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xuri/excelize/v2"
)

type summaryEntry struct {
	Label string
	Count uint64
	Bytes uint64
}

type scanDoneMsg struct {
	files           uint64
	directories     uint64
	bytes           uint64
	errors          uint64
	filtered        uint64
	sourceName      string
	sourceDir       string
	outputPath      string
	outputPaths     []string
	xlsxPath        string
	xlsxPaths       []string
	reportPath      string
	elapsed         time.Duration
	topByCount      []summaryEntry
	topBySize       []summaryEntry
	hashWorkers     int
	hashAlgorithm   hashAlgorithm
	excludeHidden   bool
	excludeSystem   bool
	createXLSX      bool
	preserveZeros   bool
	deleteCSV       bool
	csvDeleted      bool
	maxRowsPerCSV   uint64
	csvPartCount    int
	xlsxPartCount   int
	filteredHidden  uint64
	filteredSystem  uint64
	filteredExts    uint64
	filteredOSNoise uint64
	filteredSamples []string
	firstCSVItem    string
	lastCSVItem     string
	foldersOnly     bool
	agencyTemplate  bool
}

type agencyTemplateFields struct {
	RG               string
	SG               string
	Series           string
	RCSeries         string
	DeptOrganization string
	Division         string
	Section          string
	Unit             string
	RCSeriesName     string
	BeginDate        string
	EndDate          string
	Description      string
	Location         string
	MaterialType     string
	Comments         string
	Confidential     string
	DispositionDate  string
	BoxNum           string
	TDNum            string
	LocationID       string
	RecordLevel      string
}

type scanOptions struct {
	HashAlgorithm    hashAlgorithm
	ExcludeHidden    bool
	ExcludeSystem    bool
	CreateXLSX       bool
	PreserveZeros    bool
	DeleteCSV        bool
	MaxRowsPerCSV    uint64
	ExcludedExts     map[string]struct{}
	ExcludedExtsText string
	FoldersOnly      bool
	FolderDepth      int
	AgencyTemplate   bool
	AgencyFields     agencyTemplateFields
	hashFileFn       func(context.Context, string, hashAlgorithm) (string, error) // tests only
}

type scanWork struct {
	index        uint64
	path         string
	relative     string
	name         string
	ext          string
	size         uint64
	dateCreated  string
	dateModified string
}

type scanResult struct {
	index uint64
	work  scanWork
	hash  string
	err   error
}

type scannerStats struct {
	files       atomic.Uint64
	directories atomic.Uint64
	bytes       atomic.Uint64
	errors      atomic.Uint64
	filtered    atomic.Uint64
}

type emailCopyProgress struct {
	Phase       string
	Scanned     uint64
	Matched     uint64
	Copied      uint64
	Total       uint64
	CurrentRel  string
	CurrentName string
}

var emailExtensions = map[string]struct{}{
	".dbx":            {},
	".eml":            {},
	".emlx":           {},
	".emlxpart":       {},
	".mbox":           {},
	".mbx":            {},
	".msg":            {},
	".olk14msgsource": {},
	".olk15message":   {},
	".ost":            {},
	".pst":            {},
	".rge":            {},
	".tbb":            {},
	".wdseml":         {},
}

type scanCountTotals struct {
	files       uint64
	directories uint64
	bytes       uint64
}

func sortedEmailExtensions() []string {
	values := make([]string, 0, len(emailExtensions))
	for ext := range emailExtensions {
		values = append(values, ext)
	}
	slices.Sort(values)
	return values
}

func countScanTargets(ctx context.Context, sourceDir string, options scanOptions, startedAt time.Time) (scanCountTotals, error) {
	totals := scanCountTotals{}
	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if walkErr != nil {
			return nil
		}

		if d.IsDir() {
			if path != sourceDir && isAlwaysExcludedDir(d.Name()) {
				return filepath.SkipDir
			}
			if path != sourceDir && options.ExcludeHidden && isHiddenName(d.Name()) {
				setProgress(globalProgress{
					phase:          progressPhaseCounting,
					files:          totals.files,
					directories:    totals.directories,
					currentItem:    filepath.ToSlash(path),
					startedAt:      startedAt,
					phaseStartedAt: startedAt,
				})
				return filepath.SkipDir
			}
			totals.directories++
			setProgress(globalProgress{
				phase:          progressPhaseCounting,
				files:          totals.files,
				directories:    totals.directories,
				currentItem:    filepath.ToSlash(path),
				startedAt:      startedAt,
				phaseStartedAt: startedAt,
			})
			return nil
		}

		if _, skipped := shouldSkipFile(path, d.Name(), options); skipped {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		totals.files++
		totals.bytes += uint64(info.Size())
		setProgress(globalProgress{
			phase:          progressPhaseCounting,
			files:          totals.files,
			directories:    totals.directories,
			bytes:          totals.bytes,
			currentItem:    filepath.ToSlash(path),
			startedAt:      startedAt,
			phaseStartedAt: startedAt,
		})
		return nil
	})
	return totals, err
}

type reportWriter interface {
	WriteHeader() error
	WriteRow([]string) error
	Finalize(uint64) error
	Close() error
	CSVPaths() []string
}

type csvReportWriter struct {
	baseOutputPath    string
	maxRowsPerFile    uint64
	headers           []string
	rowsInCurrentFile uint64
	currentPart       int
	outputPaths       []string
	file              *os.File
	buffer            *bufio.Writer
	writer            *csv.Writer
}

const defaultMaxRowsPerCSV uint64 = 300000

var standardReportHeaders = []string{
	"File Name",
	"Extension",
	"Size in Bytes",
	"Size in Human Readable",
	"Path From Root Folder",
	"Hash Algorithm",
	"Hash Value",
	"Date Created",
	"Date Modified",
}

var agencyTemplateHeaders = []string{
	"RG",
	"SubGr",
	"Series",
	"Sub_Series",
	"RC_Series",
	"Dept_Organization",
	"Division",
	"Section",
	"Unit",
	"RC_Series_Name",
	"Begin_Date",
	"End_Date",
	"File_Num",
	"File_Name",
	"Description",
	"Location",
	"First_Name",
	"Middle_Name",
	"Last_Name",
	"Date_Of_Birth",
	"File_Type",
	"Material_Type",
	"File_Format",
	"Comments",
	"Confidential",
	"Disposition_Date",
	"Box_Num",
	"Barcode",
	"TD_Num",
	"Location_ID",
	"Record_Level",
}

func newReportWriter(outputPath string, maxRowsPerFile uint64, headers []string) (reportWriter, error) {
	if maxRowsPerFile == 0 {
		maxRowsPerFile = defaultMaxRowsPerCSV
	}
	if len(headers) == 0 {
		headers = standardReportHeaders
	}
	writer := &csvReportWriter{
		baseOutputPath: outputPath,
		maxRowsPerFile: maxRowsPerFile,
		headers:        slices.Clone(headers),
		outputPaths:    make([]string, 0, 4),
	}
	if err := writer.openPart(1); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *csvReportWriter) WriteHeader() error {
	return w.writer.Write(w.headers)
}

func (w *csvReportWriter) WriteRow(values []string) error {
	if w.rowsInCurrentFile >= w.maxRowsPerFile {
		if err := w.openPart(w.currentPart + 1); err != nil {
			return err
		}
		if err := w.WriteHeader(); err != nil {
			return err
		}
	}
	w.rowsInCurrentFile++
	return w.writer.Write(spreadsheetSafeRow(values))
}

func (w *csvReportWriter) Finalize(_ uint64) error {
	return w.flushCurrent()
}

func (w *csvReportWriter) Close() error {
	return w.closeCurrent()
}

func (w *csvReportWriter) CSVPaths() []string {
	return slices.Clone(w.outputPaths)
}

func (w *csvReportWriter) outputPathForPart(part int) string {
	return csvOutputPathForPart(w.baseOutputPath, part)
}

func (w *csvReportWriter) openPart(part int) error {
	if err := w.closeCurrent(); err != nil {
		return err
	}

	pathValue := w.outputPathForPart(part)
	if err := os.MkdirAll(filepath.Dir(pathValue), 0o755); err != nil {
		return err
	}
	file, err := os.Create(pathValue)
	if err != nil {
		return err
	}

	w.file = file
	w.buffer = bufio.NewWriterSize(file, 1<<20)
	w.writer = csv.NewWriter(w.buffer)
	w.rowsInCurrentFile = 0
	w.currentPart = part
	w.outputPaths = append(w.outputPaths, pathValue)
	return nil
}

func (w *csvReportWriter) flushCurrent() error {
	if w.writer != nil {
		w.writer.Flush()
		if err := w.writer.Error(); err != nil {
			return err
		}
	}
	if w.buffer != nil {
		if err := w.buffer.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func (w *csvReportWriter) closeCurrent() error {
	if err := w.flushCurrent(); err != nil {
		return err
	}
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		w.buffer = nil
		w.writer = nil
		return err
	}
	w.buffer = nil
	w.writer = nil
	return nil
}

func agencyFileFormat(ext string) string {
	return strings.ToUpper(strings.TrimPrefix(ext, "."))
}

func agencyLocation(fields agencyTemplateFields, relative string) string {
	if strings.TrimSpace(fields.Location) != "" {
		return fields.Location
	}
	dir := filepath.ToSlash(filepath.Dir(relative))
	if dir == "." {
		return ""
	}
	return dir
}

func normalizeAgencyTemplateFields(fields agencyTemplateFields) (agencyTemplateFields, error) {
	var err error
	if fields.RG, err = normalizeAgencyCode("RG", fields.RG, 4); err != nil {
		return fields, err
	}
	if fields.SG, err = normalizeAgencyCode("SG", fields.SG, 3); err != nil {
		return fields, err
	}
	if fields.Series, err = normalizeAgencyCode("Series", fields.Series, 3); err != nil {
		return fields, err
	}
	return fields, nil
}

func normalizeAgencyCode(label, value string, width int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("%s must contain only digits", label)
		}
	}
	if len(value) > width {
		return "", fmt.Errorf("%s must be %d digits or fewer", label, width)
	}
	return strings.Repeat("0", width-len(value)) + value, nil
}

func agencyTemplateRow(work scanWork, fields agencyTemplateFields) []string {
	comments := strings.TrimSpace(fields.Comments)
	materialType := fields.MaterialType
	if strings.TrimSpace(materialType) == "" {
		materialType = "Born Digital"
	}
	recordLevel := fields.RecordLevel
	if strings.TrimSpace(recordLevel) == "" {
		recordLevel = "Item"
	}
	return []string{
		fields.RG,
		fields.SG,
		fields.Series,
		"",
		fields.RCSeries,
		fields.DeptOrganization,
		fields.Division,
		fields.Section,
		fields.Unit,
		fields.RCSeriesName,
		fields.BeginDate,
		fields.EndDate,
		"",
		work.name,
		fields.Description,
		agencyLocation(fields, work.relative),
		"",
		"",
		"",
		"",
		"",
		materialType,
		agencyFileFormat(work.ext),
		comments,
		fields.Confidential,
		fields.DispositionDate,
		fields.BoxNum,
		"",
		fields.TDNum,
		fields.LocationID,
		recordLevel,
	}
}

func runScan(sourceDir, outputPath string, options scanOptions) (scanDoneMsg, error) {
	return runScanWithContext(context.Background(), sourceDir, outputPath, options)
}

func runScanWithContext(parent context.Context, sourceDir, outputPath string, options scanOptions) (scanDoneMsg, error) {
	if options.FoldersOnly {
		return runFolderOnlyScanWithContext(parent, sourceDir, outputPath, options)
	}
	if options.AgencyTemplate {
		normalizedFields, err := normalizeAgencyTemplateFields(options.AgencyFields)
		if err != nil {
			return scanDoneMsg{}, err
		}
		options.AgencyFields = normalizedFields
	}
	startedAt := time.Now()
	ctx, cancel := context.WithCancel(parent)
	token := setActiveScanCancel(cancel)
	defer clearActiveScanCancel(token)
	defer cancel()

	setProgress(globalProgress{
		phase:          progressPhaseCounting,
		startedAt:      startedAt,
		phaseStartedAt: startedAt,
	})

	counts, err := countScanTargets(ctx, sourceDir, options, startedAt)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			cleanupScanArtifacts(outputPath, "", "")
		}
		return scanDoneMsg{}, err
	}

	headers := standardReportHeaders
	if options.AgencyTemplate {
		headers = agencyTemplateHeaders
	}
	reportWriter, err := newReportWriter(outputPath, options.MaxRowsPerCSV, headers)
	if err != nil {
		return scanDoneMsg{}, err
	}
	writerClosed := false
	defer func() {
		if !writerClosed {
			_ = reportWriter.Close()
		}
	}()

	if err := reportWriter.WriteHeader(); err != nil {
		return scanDoneMsg{}, err
	}

	stats := &scannerStats{}
	hashWorkers := 1
	if options.HashAlgorithm.Enabled() {
		hashWorkers = max(2, runtime.NumCPU())
	}

	scanStartedAt := time.Now()
	setProgress(globalProgress{
		phase:            progressPhaseScanning,
		totalFiles:       counts.files,
		totalDirectories: counts.directories,
		totalBytes:       counts.bytes,
		currentItem:      "waiting for first file",
		startedAt:        startedAt,
		phaseStartedAt:   scanStartedAt,
	})

	workCh := make(chan scanWork, hashWorkers*4)
	resultCh := make(chan scanResult, hashWorkers*4)
	// Slots remain held until results are written in order, bounding pending.
	workSlots := make(chan struct{}, hashWorkers*4)
	walkErrCh := make(chan error, 1)
	typeTotals := make(map[string]summaryEntry)
	pending := make(map[uint64]scanResult)
	filteredHidden := uint64(0)
	filteredSystem := uint64(0)
	filteredExts := uint64(0)
	filteredOSNoise := uint64(0)
	filteredSamples := make([]string, 0, 8)
	firstCSVItem := ""
	lastCSVItem := ""
	var expected uint64

	hashFn := options.hashFileFn
	if hashFn == nil {
		hashFn = hashFile
	}
	var workerWG sync.WaitGroup
	for range hashWorkers {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for work := range workCh {
				hashValue := ""
				var resultErr error
				hashValue, resultErr = hashFn(ctx, work.path, options.HashAlgorithm)
				select {
				case resultCh <- scanResult{index: work.index, work: work, hash: hashValue, err: resultErr}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		workerWG.Wait()
		close(resultCh)
	}()

	go func() {
		defer close(workCh)
		var index uint64
		walkErrCh <- filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if walkErr != nil {
				stats.errors.Add(1)
				return nil
			}

			if d.IsDir() {
				if path != sourceDir && isAlwaysExcludedDir(d.Name()) {
					stats.filtered.Add(1)
					filteredOSNoise++
					filteredSamples = appendFilteredSample(filteredSamples, fmt.Sprintf("%s -> os noise directory", filepath.ToSlash(path)))
					return filepath.SkipDir
				}
				if path != sourceDir && options.ExcludeHidden && isHiddenName(d.Name()) {
					stats.filtered.Add(1)
					filteredHidden++
					filteredSamples = appendFilteredSample(filteredSamples, fmt.Sprintf("%s -> hidden directory", filepath.ToSlash(path)))
					setProgress(globalProgress{
						phase:            progressPhaseScanning,
						files:            stats.files.Load(),
						directories:      stats.directories.Load(),
						bytes:            stats.bytes.Load(),
						filtered:         stats.filtered.Load(),
						totalFiles:       counts.files,
						totalDirectories: counts.directories,
						totalBytes:       counts.bytes,
						currentItem:      filepath.ToSlash(path),
						startedAt:        startedAt,
						phaseStartedAt:   scanStartedAt,
					})
					return filepath.SkipDir
				}
				stats.directories.Add(1)
				setProgress(globalProgress{
					phase:            progressPhaseScanning,
					files:            stats.files.Load(),
					directories:      stats.directories.Load(),
					bytes:            stats.bytes.Load(),
					filtered:         stats.filtered.Load(),
					totalFiles:       counts.files,
					totalDirectories: counts.directories,
					totalBytes:       counts.bytes,
					currentItem:      filepath.ToSlash(path),
					startedAt:        startedAt,
					phaseStartedAt:   scanStartedAt,
				})
				return nil
			}

			if reason, skipped := shouldSkipFile(path, d.Name(), options); skipped {
				stats.filtered.Add(1)
				switch reason {
				case "hidden path":
					filteredHidden++
				case "system file":
					filteredSystem++
				case "excluded extension":
					filteredExts++
				case "os noise":
					filteredOSNoise++
				}
				filteredSamples = appendFilteredSample(filteredSamples, fmt.Sprintf("%s -> %s", filepath.ToSlash(path), reason))
				setProgress(globalProgress{
					phase:            progressPhaseScanning,
					files:            stats.files.Load(),
					directories:      stats.directories.Load(),
					bytes:            stats.bytes.Load(),
					filtered:         stats.filtered.Load(),
					totalFiles:       counts.files,
					totalDirectories: counts.directories,
					totalBytes:       counts.bytes,
					currentItem:      filepath.ToSlash(path),
					startedAt:        startedAt,
					phaseStartedAt:   scanStartedAt,
				})
				return nil
			}

			info, err := d.Info()
			if err != nil {
				stats.errors.Add(1)
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			createdAt, createdAvailable := fileCreationTime(info)
			modifiedAt := info.ModTime()

			relative, err := filepath.Rel(sourceDir, path)
			if err != nil {
				stats.errors.Add(1)
				return nil
			}

			select {
			case workSlots <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			work := scanWork{
				index:        index,
				path:         path,
				relative:     filepath.ToSlash(relative),
				name:         filepath.Base(path),
				ext:          normalizeExt(filepath.Ext(path)),
				size:         uint64(info.Size()),
				dateCreated:  formatFileTimestamp(createdAt, createdAvailable),
				dateModified: formatFileTimestamp(modifiedAt, !modifiedAt.IsZero()),
			}
			index++

			select {
			case workCh <- work:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	for result := range resultCh {
		pending[result.index] = result
		for {
			ready, ok := pending[expected]
			if !ok {
				break
			}
			delete(pending, expected)
			expected++
			<-workSlots

			if ready.err != nil {
				if errors.Is(ready.err, context.Canceled) {
					cancel()
					continue
				}
				stats.errors.Add(1)
				setProgress(globalProgress{
					phase:            progressPhaseScanning,
					files:            stats.files.Load(),
					directories:      stats.directories.Load(),
					bytes:            stats.bytes.Load(),
					filtered:         stats.filtered.Load(),
					totalFiles:       counts.files,
					totalDirectories: counts.directories,
					totalBytes:       counts.bytes,
					currentItem:      ready.work.relative,
					startedAt:        startedAt,
					phaseStartedAt:   scanStartedAt,
				})
				continue
			}

			row := []string{
				ready.work.name,
				ready.work.ext,
				fmt.Sprintf("%d", ready.work.size),
				humanBytes(ready.work.size),
				ready.work.relative,
				options.HashAlgorithm.CSVName(),
				ready.hash,
				ready.work.dateCreated,
				ready.work.dateModified,
			}
			if options.AgencyTemplate {
				row = agencyTemplateRow(ready.work, options.AgencyFields)
			}
			if err := reportWriter.WriteRow(row); err != nil {
				cancel()
				return scanDoneMsg{}, err
			}

			stats.files.Add(1)
			stats.bytes.Add(ready.work.size)
			if firstCSVItem == "" {
				firstCSVItem = ready.work.relative
			}
			lastCSVItem = ready.work.relative

			key := summaryKey(ready.work.ext)
			entry := typeTotals[key]
			entry.Label = key
			entry.Count++
			entry.Bytes += ready.work.size
			typeTotals[key] = entry

			setProgress(globalProgress{
				phase:            progressPhaseScanning,
				files:            stats.files.Load(),
				directories:      stats.directories.Load(),
				bytes:            stats.bytes.Load(),
				filtered:         stats.filtered.Load(),
				totalFiles:       counts.files,
				totalDirectories: counts.directories,
				totalBytes:       counts.bytes,
				currentItem:      ready.work.relative,
				startedAt:        startedAt,
				phaseStartedAt:   scanStartedAt,
			})
		}
	}

	if walkErr := <-walkErrCh; walkErr != nil && !errors.Is(walkErr, context.Canceled) {
		return scanDoneMsg{}, walkErr
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		_ = reportWriter.Close()
		writerClosed = true
		cleanupScanArtifacts(reportWriter.CSVPaths()...)
		return scanDoneMsg{}, ctx.Err()
	}
	if err := reportWriter.Finalize(stats.files.Load()); err != nil {
		return scanDoneMsg{}, err
	}
	if err := reportWriter.Close(); err != nil {
		return scanDoneMsg{}, err
	}
	writerClosed = true

	csvPaths := reportWriter.CSVPaths()
	if len(csvPaths) == 0 {
		csvPaths = []string{outputPath}
	}
	maxRowsPerCSV := options.MaxRowsPerCSV
	if maxRowsPerCSV == 0 {
		maxRowsPerCSV = defaultMaxRowsPerCSV
	}

	xlsxPaths := make([]string, 0, len(csvPaths))
	csvDeleted := false
	if options.CreateXLSX {
		xlsxStartedAt := time.Now()
		for index, csvPartPath := range csvPaths {
			nextXLSXPath := strings.TrimSuffix(csvPartPath, filepath.Ext(csvPartPath)) + ".xlsx"
			currentLabel := filepath.Base(nextXLSXPath)
			if len(csvPaths) > 1 {
				currentLabel = fmt.Sprintf("%s (%d/%d)", currentLabel, index+1, len(csvPaths))
			}
			setProgress(globalProgress{
				phase:            progressPhaseXLSX,
				files:            stats.files.Load(),
				directories:      stats.directories.Load(),
				bytes:            stats.bytes.Load(),
				filtered:         stats.filtered.Load(),
				totalFiles:       counts.files,
				totalDirectories: counts.directories,
				totalBytes:       counts.bytes,
				currentItem:      currentLabel,
				startedAt:        startedAt,
				phaseStartedAt:   xlsxStartedAt,
			})
			if err := convertCSVToXLSX(csvPartPath, nextXLSXPath, options.PreserveZeros); err != nil {
				if errors.Is(err, context.Canceled) {
					cleanupPaths := append([]string{}, csvPaths...)
					cleanupPaths = append(cleanupPaths, xlsxPaths...)
					cleanupScanArtifacts(cleanupPaths...)
				}
				return scanDoneMsg{}, err
			}
			xlsxPaths = append(xlsxPaths, nextXLSXPath)
		}
		if options.DeleteCSV {
			for _, csvPartPath := range csvPaths {
				if err := os.Remove(csvPartPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return scanDoneMsg{}, err
				}
			}
			csvDeleted = true
		}
	}
	xlsxPath := ""
	if len(xlsxPaths) > 0 {
		xlsxPath = xlsxPaths[0]
	}

	reportPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + "-report.txt"
	report := scanDoneMsg{
		files:           stats.files.Load(),
		directories:     stats.directories.Load(),
		bytes:           stats.bytes.Load(),
		errors:          stats.errors.Load(),
		filtered:        stats.filtered.Load(),
		sourceName:      folderDisplayName(sourceDir),
		sourceDir:       sourceDir,
		outputPath:      csvPaths[0],
		outputPaths:     csvPaths,
		xlsxPath:        xlsxPath,
		xlsxPaths:       xlsxPaths,
		reportPath:      reportPath,
		elapsed:         time.Since(startedAt),
		topByCount:      summarizeByCount(typeTotals, 8),
		topBySize:       summarizeBySize(typeTotals, 8),
		hashWorkers:     hashWorkers,
		hashAlgorithm:   options.HashAlgorithm,
		excludeHidden:   options.ExcludeHidden,
		excludeSystem:   options.ExcludeSystem,
		createXLSX:      options.CreateXLSX,
		preserveZeros:   options.PreserveZeros,
		deleteCSV:       options.DeleteCSV,
		csvDeleted:      csvDeleted,
		maxRowsPerCSV:   maxRowsPerCSV,
		csvPartCount:    len(csvPaths),
		xlsxPartCount:   len(xlsxPaths),
		filteredHidden:  filteredHidden,
		filteredSystem:  filteredSystem,
		filteredExts:    filteredExts,
		filteredOSNoise: filteredOSNoise,
		filteredSamples: filteredSamples,
		firstCSVItem:    firstCSVItem,
		lastCSVItem:     lastCSVItem,
		agencyTemplate:  options.AgencyTemplate,
	}
	if err := writeScanReport(reportPath, report); err != nil {
		return scanDoneMsg{}, err
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		cleanupPaths := append([]string{}, csvPaths...)
		cleanupPaths = append(cleanupPaths, xlsxPaths...)
		cleanupPaths = append(cleanupPaths, reportPath)
		cleanupScanArtifacts(cleanupPaths...)
		return scanDoneMsg{}, ctx.Err()
	}

	return report, nil
}

func defaultFolderListFilename(sourceDir string) string {
	stamp := time.Now().Format("2006-01-02T15-04-05")
	name := filepath.Base(sourceDir)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "folder-list"
	}
	return fmt.Sprintf("%s-folder-list-%s.csv", name, stamp)
}

func folderRelDepth(relSlash string) int {
	if relSlash == "" || relSlash == "." {
		return 0
	}
	return strings.Count(relSlash, "/") + 1
}

func runFolderOnlyScanWithContext(parent context.Context, sourceDir, outputPath string, options scanOptions) (scanDoneMsg, error) {
	startedAt := time.Now()
	ctx, cancel := context.WithCancel(parent)
	token := setActiveScanCancel(cancel)
	defer clearActiveScanCancel(token)
	defer cancel()

	setProgress(globalProgress{
		phase:          progressPhaseCounting,
		startedAt:      startedAt,
		phaseStartedAt: startedAt,
	})

	var totalDirs uint64
	countErr := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if walkErr != nil || !d.IsDir() || path == sourceDir {
			return nil
		}
		if isAlwaysExcludedDir(d.Name()) {
			return filepath.SkipDir
		}
		if options.ExcludeHidden && isHiddenName(d.Name()) {
			return filepath.SkipDir
		}
		relative, _ := filepath.Rel(sourceDir, path)
		relSlash := filepath.ToSlash(relative)
		depth := folderRelDepth(relSlash)
		if options.FolderDepth > 0 && depth > options.FolderDepth {
			return filepath.SkipDir
		}
		totalDirs++
		setProgress(globalProgress{
			phase:          progressPhaseCounting,
			directories:    totalDirs,
			currentItem:    filepath.ToSlash(path),
			startedAt:      startedAt,
			phaseStartedAt: startedAt,
		})
		if options.FolderDepth > 0 && depth == options.FolderDepth {
			return filepath.SkipDir
		}
		return nil
	})
	if countErr != nil {
		return scanDoneMsg{}, countErr
	}

	csvPath := csvOutputPathForPart(outputPath, 1)
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		return scanDoneMsg{}, err
	}
	f, err := os.Create(csvPath)
	if err != nil {
		return scanDoneMsg{}, err
	}
	buf := bufio.NewWriterSize(f, 1<<20)
	csvWriter := csv.NewWriter(buf)

	fileClosed := false
	closeFile := func() {
		if !fileClosed {
			csvWriter.Flush()
			buf.Flush() //nolint:errcheck
			f.Close()   //nolint:errcheck
			fileClosed = true
		}
	}
	defer closeFile()

	if err := csvWriter.Write([]string{"Path From Root Folder"}); err != nil {
		return scanDoneMsg{}, err
	}

	scanStartedAt := time.Now()
	var dirCount uint64
	var filteredCount uint64
	var filteredHidden uint64
	var filteredOSNoise uint64
	filteredSamples := make([]string, 0, 8)
	firstCSVItem := ""
	lastCSVItem := ""

	setProgress(globalProgress{
		phase:            progressPhaseScanning,
		totalDirectories: totalDirs,
		currentItem:      "waiting for first folder",
		startedAt:        startedAt,
		phaseStartedAt:   scanStartedAt,
	})

	walkErr := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if walkErr != nil || !d.IsDir() || path == sourceDir {
			return nil
		}
		if isAlwaysExcludedDir(d.Name()) {
			filteredCount++
			filteredOSNoise++
			filteredSamples = appendFilteredSample(filteredSamples, fmt.Sprintf("%s -> os noise directory", filepath.ToSlash(path)))
			return filepath.SkipDir
		}
		if options.ExcludeHidden && isHiddenName(d.Name()) {
			filteredCount++
			filteredHidden++
			filteredSamples = appendFilteredSample(filteredSamples, fmt.Sprintf("%s -> hidden directory", filepath.ToSlash(path)))
			return filepath.SkipDir
		}
		relative, relErr := filepath.Rel(sourceDir, path)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(relative)
		depth := folderRelDepth(relSlash)
		if options.FolderDepth > 0 && depth > options.FolderDepth {
			return filepath.SkipDir
		}
		if err := csvWriter.Write(spreadsheetSafeRow([]string{relSlash})); err != nil {
			cancel()
			return err
		}
		dirCount++
		if firstCSVItem == "" {
			firstCSVItem = relSlash
		}
		lastCSVItem = relSlash
		setProgress(globalProgress{
			phase:            progressPhaseScanning,
			directories:      dirCount,
			totalDirectories: totalDirs,
			filtered:         filteredCount,
			currentItem:      relSlash,
			startedAt:        startedAt,
			phaseStartedAt:   scanStartedAt,
		})
		if options.FolderDepth > 0 && depth == options.FolderDepth {
			return filepath.SkipDir
		}
		return nil
	})

	csvWriter.Flush()
	if flushErr := csvWriter.Error(); flushErr != nil {
		return scanDoneMsg{}, flushErr
	}
	closeFile()

	if walkErr != nil && !errors.Is(walkErr, context.Canceled) {
		return scanDoneMsg{}, walkErr
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		cleanupScanArtifacts(csvPath)
		return scanDoneMsg{}, ctx.Err()
	}

	maxRowsPerCSV := options.MaxRowsPerCSV
	if maxRowsPerCSV == 0 {
		maxRowsPerCSV = defaultMaxRowsPerCSV
	}

	reportPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + "-report.txt"
	report := scanDoneMsg{
		files:           0,
		directories:     dirCount,
		bytes:           0,
		errors:          0,
		filtered:        filteredCount,
		sourceName:      folderDisplayName(sourceDir),
		sourceDir:       sourceDir,
		outputPath:      csvPath,
		outputPaths:     []string{csvPath},
		xlsxPath:        "",
		xlsxPaths:       []string{},
		reportPath:      reportPath,
		elapsed:         time.Since(startedAt),
		hashAlgorithm:   hashAlgorithmOff,
		foldersOnly:     true,
		excludeHidden:   options.ExcludeHidden,
		maxRowsPerCSV:   maxRowsPerCSV,
		csvPartCount:    1,
		xlsxPartCount:   0,
		filteredHidden:  filteredHidden,
		filteredOSNoise: filteredOSNoise,
		filteredSamples: filteredSamples,
		firstCSVItem:    firstCSVItem,
		lastCSVItem:     lastCSVItem,
	}
	if err := writeScanReport(reportPath, report); err != nil {
		return scanDoneMsg{}, err
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		cleanupScanArtifacts(csvPath, reportPath)
		return scanDoneMsg{}, ctx.Err()
	}
	return report, nil
}

func cleanupScanArtifacts(paths ...string) {
	for _, pathValue := range paths {
		if strings.TrimSpace(pathValue) == "" {
			continue
		}
		_ = os.Remove(pathValue)
	}
}

func writeScanReport(reportPath string, done scanDoneMsg) error {
	return os.WriteFile(reportPath, []byte(buildScanReport(done)), 0o644)
}

func buildScanReport(done scanDoneMsg) string {
	if done.foldersOnly {
		lines := []string{
			"Folder List Report",
			fmt.Sprintf("Selected folder: %s", valueOrDefault(done.sourceName, "unknown")),
			fmt.Sprintf("Saved folder list: %s", filepath.Base(done.outputPath)),
			fmt.Sprintf("Summary report: %s", filepath.Base(done.reportPath)),
			fmt.Sprintf("Folders in CSV: %d", done.directories),
			fmt.Sprintf("Folders skipped: %d", done.filtered),
			fmt.Sprintf("OS noise excluded: %d", done.filteredOSNoise),
			fmt.Sprintf("First folder in CSV: %s", valueOrDefault(done.firstCSVItem, "none")),
			fmt.Sprintf("Last folder in CSV: %s", valueOrDefault(done.lastCSVItem, "none")),
			fmt.Sprintf("Finished in: %s", done.elapsed.Round(time.Millisecond)),
		}
		return strings.Join(lines, "\n") + "\n"
	}
	lines := []string{
		"Content List Report",
		fmt.Sprintf("Selected folder: %s", valueOrDefault(done.sourceName, "unknown")),
		fmt.Sprintf("Saved file list: %s", filepath.Base(done.outputPath)),
		fmt.Sprintf("CSV files created: %d", done.csvPartCount),
		fmt.Sprintf("Rows per CSV max: %d", done.maxRowsPerCSV),
		fmt.Sprintf("CSV parts: %s", summarizeOutputParts(done.outputPaths)),
		fmt.Sprintf("Excel copy: %s", baseNameOrFallback(done.xlsxPath, "not created")),
		fmt.Sprintf("XLSX files created: %d", done.xlsxPartCount),
		fmt.Sprintf("XLSX parts: %s", summarizeOutputParts(done.xlsxPaths)),
		fmt.Sprintf("Summary report: %s", filepath.Base(done.reportPath)),
		fmt.Sprintf("Agency template: %s", onOff(done.agencyTemplate)),
		fmt.Sprintf("Files included: %d", done.files),
		fmt.Sprintf("Folders counted: %d", done.directories),
		fmt.Sprintf("Total size: %s", humanBytes(done.bytes)),
		fmt.Sprintf("Items skipped: %d", done.filtered),
		fmt.Sprintf("OS noise excluded: %d", done.filteredOSNoise),
		fmt.Sprintf("Verification hash: %s", done.hashAlgorithm.OptionLabel()),
		fmt.Sprintf("First file in CSV: %s", valueOrDefault(done.firstCSVItem, "none")),
		fmt.Sprintf("Last file in CSV: %s", valueOrDefault(done.lastCSVItem, "none")),
		fmt.Sprintf("Delete CSV after XLSX: %s", onOff(done.deleteCSV && done.createXLSX)),
		fmt.Sprintf("CSV removed after XLSX: %s", onOff(done.csvDeleted)),
		fmt.Sprintf("Finished in: %s", done.elapsed.Round(time.Millisecond)),
		"",
	}
	lines = append(lines, renderReportSummaryLines("Top extensions by file count", done.topByCount, func(entry summaryEntry) string {
		return fmt.Sprintf("%s files, %s", formatUint(entry.Count), humanBytes(entry.Bytes))
	})...)
	lines = append(lines, "")
	lines = append(lines, renderReportSummaryLines("Top extensions by total size", done.topBySize, func(entry summaryEntry) string {
		return fmt.Sprintf("%s, %s files", humanBytes(entry.Bytes), formatUint(entry.Count))
	})...)
	return strings.Join(lines, "\n") + "\n"
}

func renderReportSummaryLines(title string, items []summaryEntry, formatter func(summaryEntry) string) []string {
	lines := []string{title}
	if len(items) == 0 {
		return append(lines, "No files were written.")
	}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("%s: %s", item.Label, formatter(item)))
	}
	return lines
}

func baseNameOrFallback(pathValue, fallback string) string {
	if strings.TrimSpace(pathValue) == "" {
		return fallback
	}
	return filepath.Base(pathValue)
}

func summarizeOutputParts(paths []string) string {
	if len(paths) == 0 {
		return "none"
	}
	const maxShown = 4
	labels := make([]string, 0, min(len(paths), maxShown))
	for index, pathValue := range paths {
		if index >= maxShown {
			break
		}
		labels = append(labels, filepath.Base(pathValue))
	}
	if len(paths) > maxShown {
		return fmt.Sprintf("%s (+%d more)", strings.Join(labels, ", "), len(paths)-maxShown)
	}
	return strings.Join(labels, ", ")
}

func csvOutputPathForPart(baseOutputPath string, part int) string {
	if part <= 0 {
		part = 1
	}
	ext := filepath.Ext(baseOutputPath)
	base := strings.TrimSuffix(baseOutputPath, ext)
	return fmt.Sprintf("%s-%03d%s", base, part, ext)
}

func copyEmailFiles(sourceDir, destDir string) (string, uint64, error) {
	return copyEmailFilesWithProgress(context.Background(), sourceDir, destDir, nil)
}

func copyEmailFilesWithProgress(ctx context.Context, sourceDir, destDir string, progress func(emailCopyProgress)) (string, uint64, error) {
	sourceAbs, err := filepath.Abs(sourceDir)
	if err != nil {
		return "", 0, err
	}
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return "", 0, err
	}
	if sourceAbs == destAbs {
		return "", 0, fmt.Errorf("destination folder must be different from the source folder")
	}
	if isPathWithin(destAbs, sourceAbs) {
		return "", 0, fmt.Errorf("destination folder cannot be inside the source folder")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", 0, err
	}
	// Resolve selected-root aliases before walking. Rooted opens below also
	// enforce containment if a path component changes during the copy.
	sourceReal, err := filepath.EvalSymlinks(sourceAbs)
	if err != nil {
		return "", 0, err
	}
	destReal, err := filepath.EvalSymlinks(destAbs)
	if err != nil {
		return "", 0, err
	}
	if sourceReal == destReal || isPathWithin(destReal, sourceReal) {
		return "", 0, fmt.Errorf("destination folder cannot be inside the source folder")
	}
	sourceRoot, err := os.OpenRoot(sourceAbs)
	if err != nil {
		return "", 0, err
	}
	defer sourceRoot.Close()
	destRoot, err := os.OpenRoot(destAbs)
	if err != nil {
		return "", 0, err
	}
	defer destRoot.Close()
	sourceRootInfo, err := sourceRoot.Stat(".")
	if err != nil {
		return "", 0, err
	}
	destRootInfo, err := destRoot.Stat(".")
	if err != nil {
		return "", 0, err
	}
	if os.SameFile(sourceRootInfo, destRootInfo) {
		return "", 0, fmt.Errorf("destination folder must be different from the source folder")
	}

	timestamp := time.Now().Format("2006-01-02T15-04-05")
	var manifestPath string
	var manifestFile *os.File
	for part := 0; part < 100; part++ {
		name := fmt.Sprintf("email-copy-manifest-%s.csv", timestamp)
		if part > 0 {
			name = fmt.Sprintf("email-copy-manifest-%s-%d.csv", timestamp, part)
		}
		manifestFile, err = destRoot.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", 0, err
		}
		manifestPath = filepath.Join(destDir, name)
		break
	}
	if manifestFile == nil {
		return "", 0, fmt.Errorf("could not create a unique email-copy manifest")
	}
	defer manifestFile.Close()

	writer := csv.NewWriter(manifestFile)
	defer writer.Flush()

	if err := writer.Write([]string{
		"Source Path",
		"Destination Path",
		"Relative Path",
		"File Name",
		"Extension",
		"Size in Bytes",
	}); err != nil {
		return "", 0, err
	}

	matches := make([]string, 0, 256)
	var scanned uint64
	err = filepath.WalkDir(sourceAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil || d.IsDir() {
			return nil
		}
		scanned++
		if progress != nil {
			progress(emailCopyProgress{
				Phase:       "scanning",
				Scanned:     scanned,
				Matched:     uint64(len(matches)),
				CurrentName: d.Name(),
			})
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if _, ok := emailExtensions[ext]; !ok {
			return nil
		}

		matches = append(matches, path)
		if progress != nil {
			progress(emailCopyProgress{
				Phase:       "scanning",
				Scanned:     scanned,
				Matched:     uint64(len(matches)),
				CurrentName: d.Name(),
			})
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	if progress != nil {
		progress(emailCopyProgress{
			Phase:   "copying",
			Scanned: scanned,
			Matched: uint64(len(matches)),
			Total:   uint64(len(matches)),
		})
	}

	var copied uint64
	for _, path := range matches {
		if ctx.Err() != nil {
			return manifestPath, copied, ctx.Err()
		}
		relative, err := filepath.Rel(sourceAbs, path)
		if err != nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		targetPath := filepath.Join(destAbs, relative)
		info, err := copyFileWithinRoots(sourceRoot, destRoot, relative)
		if err != nil {
			return "", 0, err
		}

		if err := writer.Write(spreadsheetSafeRow([]string{
			path,
			targetPath,
			filepath.ToSlash(relative),
			filepath.Base(path),
			ext,
			fmt.Sprintf("%d", info.Size()),
		})); err != nil {
			return "", 0, err
		}

		copied++
		if progress != nil {
			progress(emailCopyProgress{
				Phase:       "copying",
				Scanned:     scanned,
				Matched:     uint64(len(matches)),
				Copied:      copied,
				Total:       uint64(len(matches)),
				CurrentRel:  filepath.ToSlash(relative),
				CurrentName: filepath.Base(path),
			})
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", 0, err
	}

	return manifestPath, copied, nil
}

func isPathWithin(candidate, root string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyFileWithinRoots(sourceRoot, destRoot *os.Root, relative string) (os.FileInfo, error) {
	sourceFile, err := sourceRoot.Open(relative)
	if err != nil {
		return nil, err
	}
	defer sourceFile.Close()
	info, err := sourceFile.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("email source is not a regular file: %s", relative)
	}

	if err := destRoot.MkdirAll(filepath.Dir(relative), 0o755); err != nil {
		return nil, err
	}
	if destInfo, err := destRoot.Lstat(relative); err == nil {
		if destInfo.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("email destination is a symlink: %s", relative)
		}
		if !destInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("email destination is not a regular file: %s", relative)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Write a new inode, then replace the destination directory entry. This
	// cannot truncate a source file or an outside file reached by a hard link.
	temporary := filepath.Join(filepath.Dir(relative), ".content-list-copy-"+rand.Text())
	tempFile, err := destRoot.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return nil, err
	}
	defer destRoot.Remove(temporary)
	defer tempFile.Close()
	if _, err := io.Copy(tempFile, sourceFile); err != nil {
		return nil, err
	}
	if err := tempFile.Sync(); err != nil {
		return nil, err
	}
	_ = tempFile.Chmod(info.Mode())
	if err := tempFile.Close(); err != nil {
		return nil, err
	}
	if err := destRoot.Rename(temporary, relative); err != nil {
		return nil, err
	}
	return info, nil
}

func convertCSVToXLSX(csvPath, xlsxPath string, preserveZeros bool) error {
	file, err := os.Open(csvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReaderSize(file, 1<<20))

	workbook := excelize.NewFile()
	defer workbook.Close()

	sheet := workbook.GetSheetName(workbook.GetActiveSheetIndex())
	streamWriter, err := workbook.NewStreamWriter(sheet)
	if err != nil {
		return err
	}

	textStyleID := 0
	if preserveZeros {
		textStyleID, err = workbook.NewStyle(&excelize.Style{NumFmt: 49})
		if err != nil {
			return err
		}
	}

	rowCount := 0
	maxCols := 0
	for {
		row, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
		for i := range row {
			row[i] = spreadsheetOriginalCell(row[i])
		}
		rowCount++
		if len(row) > maxCols {
			maxCols = len(row)
		}

		cellName, err := excelize.CoordinatesToCellName(1, rowCount)
		if err != nil {
			return err
		}

		cells := make([]interface{}, 0, len(row))
		for colIndex, value := range row {
			if preserveZeros {
				cells = append(cells, excelize.Cell{StyleID: textStyleID, Value: value})
				continue
			}
			if rowCount > 1 && colIndex == 2 {
				cells = append(cells, parseUintString(value))
				continue
			}
			cells = append(cells, value)
		}

		if err := streamWriter.SetRow(cellName, cells); err != nil {
			return err
		}
	}

	if err := streamWriter.Flush(); err != nil {
		return err
	}

	if rowCount > 1 && maxCols > 0 {
		lastCell, err := excelize.CoordinatesToCellName(maxCols, rowCount)
		if err == nil {
			showRows := true
			_ = workbook.AddTable(sheet, &excelize.Table{
				Range:             "A1:" + lastCell,
				Name:              "ContentList",
				StyleName:         "TableStyleMedium2",
				ShowFirstColumn:   false,
				ShowLastColumn:    false,
				ShowRowStripes:    &showRows,
				ShowColumnStripes: false,
			})
		}
	}

	return workbook.SaveAs(xlsxPath)
}

func parseUintString(value string) interface{} {
	if value == "" {
		return value
	}
	var parsed uint64
	for _, char := range value {
		if char < '0' || char > '9' {
			return value
		}
		parsed = parsed*10 + uint64(char-'0')
	}
	return parsed
}
