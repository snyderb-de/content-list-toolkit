package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type cloneVerdict string

const (
	verdictExactClone    cloneVerdict = "Exact Clone"
	verdictContentClone  cloneVerdict = "Content Clone"
	verdictMetadataClone cloneVerdict = "Metadata Clone"
	verdictNotAClone     cloneVerdict = "Not a Clone"
)

var pdfIDRe = regexp.MustCompile(`/ID\s*\[<[0-9a-fA-F]+><[0-9a-fA-F]+>\]`)

const pdfSoftTailSize = 2048

func pdfNormalizedTail(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := info.Size() - pdfSoftTailSize
	if offset < 0 {
		offset = 0
	}
	buf := make([]byte, pdfSoftTailSize)
	n, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return pdfIDRe.ReplaceAll(buf[:n], []byte("/ID[<0><0>]")), nil
}

func pdfSoftMatch(pathA, pathB string) bool {
	tailA, err := pdfNormalizedTail(pathA)
	if err != nil {
		return false
	}
	tailB, err := pdfNormalizedTail(pathB)
	if err != nil {
		return false
	}
	return bytes.Equal(tailA, tailB)
}

func softIndexKey(fileName string, size uint64) string {
	return fileName + "::" + strconv.FormatUint(size, 10)
}

type cloneCompareProgress struct {
	compared    uint64
	total       uint64
	differences uint64
	currentItem string
}

type cloneVerificationDone struct {
	driveA              scanDoneMsg
	driveB              scanDoneMsg
	diffPath            string
	reportPath          string
	hashAlgorithm       hashAlgorithm
	elapsed             time.Duration
	compared            uint64
	differences         uint64
	movedFiles          uint64
	duplicatesOnB       uint64
	duplicatesOnA       uint64
	missingNoMatch      uint64
	extraNoMatch        uint64
	sizeMismatches      uint64
	hashMismatches      uint64
	excludedSystem      uint64
	metadataOnlyDiffs   uint64
	verdict             cloneVerdict
	softCompare         bool
	csvDeferredDeletion bool
}

type scanCSVRow struct {
	fileName      string
	extension     string
	size          uint64
	relativePath  string
	hashAlgorithm string
	hashValue     string
}

type scanCSVIterator struct {
	paths       []string
	pathIndex   int
	file        *os.File
	reader      *csv.Reader
	currentPath string
}

func compareProgressFraction(progress cloneCompareProgress) float64 {
	if progress.total == 0 {
		return 0
	}
	value := float64(progress.compared) / float64(progress.total)
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func cloneOutputPathForDriveB(outputPath string) string {
	ext := filepath.Ext(outputPath)
	base := strings.TrimSuffix(outputPath, ext)
	return base + "-clone-b" + ext
}

func cloneDiffCSVPath(outputPath string) string {
	ext := filepath.Ext(outputPath)
	base := strings.TrimSuffix(outputPath, ext)
	return base + "-clone-differences" + ext
}

func cloneDiffReportPath(outputPath string) string {
	ext := filepath.Ext(outputPath)
	base := strings.TrimSuffix(outputPath, ext)
	return base + "-clone-report.txt"
}

func newScanCSVIterator(paths []string) (*scanCSVIterator, error) {
	iter := &scanCSVIterator{
		paths: append([]string{}, paths...),
	}
	if err := iter.openNext(); err != nil {
		return nil, err
	}
	return iter, nil
}

func (it *scanCSVIterator) openNext() error {
	_ = it.closeCurrent()
	for it.pathIndex < len(it.paths) {
		nextPath := it.paths[it.pathIndex]
		it.pathIndex++
		file, err := os.Open(nextPath)
		if err != nil {
			return err
		}
		reader := csv.NewReader(file)
		if _, err := reader.Read(); err != nil {
			file.Close()
			if errors.Is(err, io.EOF) {
				continue
			}
			return err
		}
		it.file = file
		it.reader = reader
		it.currentPath = nextPath
		return nil
	}
	it.file = nil
	it.reader = nil
	it.currentPath = ""
	return io.EOF
}

func (it *scanCSVIterator) closeCurrent() error {
	if it.file == nil {
		return nil
	}
	err := it.file.Close()
	it.file = nil
	it.reader = nil
	it.currentPath = ""
	return err
}

func (it *scanCSVIterator) Close() error {
	return it.closeCurrent()
}

func (it *scanCSVIterator) Next() (*scanCSVRow, error) {
	for {
		if it.reader == nil {
			return nil, io.EOF
		}
		row, err := it.reader.Read()
		if err == nil {
			if len(row) < 7 {
				return nil, fmt.Errorf("scan csv row in %s is missing columns", it.currentPath)
			}
			size, parseErr := strconv.ParseUint(strings.TrimSpace(row[2]), 10, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("scan csv row in %s has invalid size %q: %w", it.currentPath, row[2], parseErr)
			}
			return &scanCSVRow{
				fileName:      spreadsheetOriginalCell(row[0]),
				extension:     row[1],
				size:          size,
				relativePath:  spreadsheetOriginalCell(row[4]),
				hashAlgorithm: row[5],
				hashValue:     row[6],
			}, nil
		}
		if errors.Is(err, io.EOF) {
			if openErr := it.openNext(); openErr != nil {
				if errors.Is(openErr, io.EOF) {
					return nil, io.EOF
				}
				return nil, openErr
			}
			continue
		}
		return nil, err
	}
}

func computeVerdict(r cloneVerificationDone) cloneVerdict {
	if r.missingNoMatch > 0 || r.extraNoMatch > 0 || r.hashMismatches > 0 || r.sizeMismatches > 0 {
		return verdictNotAClone
	}
	if r.metadataOnlyDiffs > 0 {
		return verdictMetadataClone
	}
	if r.movedFiles > 0 || r.duplicatesOnA > 0 || r.duplicatesOnB > 0 {
		return verdictContentClone
	}
	return verdictExactClone
}

func compareScanOutputs(
	ctx context.Context,
	driveA scanDoneMsg,
	driveB scanDoneMsg,
	diffPath string,
	reportPath string,
	softCompare bool,
	progress func(cloneCompareProgress),
	diffRow func(DiffRowPayload),
) (cloneVerificationDone, error) {
	startedAt := time.Now()
	result := cloneVerificationDone{
		driveA:         driveA,
		driveB:         driveB,
		diffPath:       diffPath,
		reportPath:     reportPath,
		hashAlgorithm:  driveA.hashAlgorithm,
		excludedSystem: driveA.filteredOSNoise + driveB.filteredOSNoise,
		softCompare:    softCompare,
	}

	driveAIter, err := newScanCSVIterator(driveA.outputPaths)
	if err != nil {
		return result, err
	}
	defer driveAIter.Close()

	driveBIter, err := newScanCSVIterator(driveB.outputPaths)
	if err != nil {
		return result, err
	}
	defer driveBIter.Close()

	if err := os.MkdirAll(filepath.Dir(diffPath), 0o755); err != nil {
		return result, err
	}
	diffFile, err := os.Create(diffPath)
	if err != nil {
		return result, err
	}
	writer := csv.NewWriter(diffFile)
	writeRow := func(values []string) error {
		return writer.Write(spreadsheetSafeRow(values))
	}
	writeErr := func(err error) (cloneVerificationDone, error) {
		writer.Flush()
		_ = diffFile.Close()
		cleanupScanArtifacts(diffPath, result.reportPath)
		return result, err
	}

	if err := writeRow([]string{
		"Difference Type",
		"1st Drive Path From Root Folder",
		"1st Drive File Name",
		"2nd Drive Path From Root Folder",
		"2nd Drive File Name",
		"1st Drive Size in Bytes",
		"2nd Drive Size in Bytes",
		"1st Drive Hash Algorithm",
		"2nd Drive Hash Algorithm",
		"1st Drive Hash Value",
		"2nd Drive Hash Value",
	}); err != nil {
		return writeErr(err)
	}

	nextProgress := func(currentItem string) {
		if progress == nil {
			return
		}
		total := driveA.files
		if driveB.files > total {
			total = driveB.files
		}
		progress(cloneCompareProgress{
			compared:    result.compared,
			total:       total,
			differences: result.differences,
			currentItem: currentItem,
		})
	}

	readA := func() (*scanCSVRow, error) {
		r, err := driveAIter.Next()
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return r, err
	}
	readB := func() (*scanCSVRow, error) {
		r, err := driveBIter.Next()
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return r, err
	}

	nextA, err := readA()
	if err != nil {
		return writeErr(err)
	}
	nextB, err := readB()
	if err != nil {
		return writeErr(err)
	}

	// ── Pass 1: streaming sorted merge ───────────────────────────────────────
	// Path matches are resolved immediately (exact or hash mismatch).
	// Path-only rows are held in memory for Pass 2 hash cross-reference.
	unmatchedA := make(map[string][]scanCSVRow) // hash → A rows with no path match in B
	unmatchedB := make(map[string][]scanCSVRow) // hash → B rows with no path match in A
	softBIndex := make(map[string][]scanCSVRow) // filename::size → B rows (for PDF soft compare)

	for nextA != nil || nextB != nil {
		select {
		case <-ctx.Done():
			return writeErr(ctx.Err())
		default:
		}

		switch {
		case nextA != nil && nextB != nil && nextA.relativePath == nextB.relativePath:
			result.compared++
			sizeMismatch := nextA.size != nextB.size
			hashMismatch := nextA.hashValue != nextB.hashValue || nextA.hashAlgorithm != nextB.hashAlgorithm
			var diffType string
			switch {
			case sizeMismatch && hashMismatch:
				diffType = "size and hash mismatch"
				result.sizeMismatches++
				result.hashMismatches++
			case sizeMismatch:
				diffType = "size mismatch"
				result.sizeMismatches++
			case hashMismatch:
				diffType = "hash mismatch"
				result.hashMismatches++
			}
			if diffType != "" {
				result.differences++
				if err := writeRow([]string{
					diffType,
					nextA.relativePath,
					nextA.fileName,
					nextB.relativePath,
					nextB.fileName,
					strconv.FormatUint(nextA.size, 10),
					strconv.FormatUint(nextB.size, 10),
					nextA.hashAlgorithm,
					nextB.hashAlgorithm,
					nextA.hashValue,
					nextB.hashValue,
				}); err != nil {
					return writeErr(err)
				}
				if diffRow != nil {
					diffRow(DiffRowPayload{
						DiffType: diffType,
						PathA:    nextA.relativePath,
						PathB:    nextB.relativePath,
						SizeA:    strconv.FormatUint(nextA.size, 10),
						SizeB:    strconv.FormatUint(nextB.size, 10),
						HashA:    nextA.hashValue,
						HashB:    nextB.hashValue,
					})
				}
			}
			nextProgress(nextA.relativePath)
			if nextA, err = readA(); err != nil {
				return writeErr(err)
			}
			if nextB, err = readB(); err != nil {
				return writeErr(err)
			}

		case nextB == nil || (nextA != nil && nextA.relativePath < nextB.relativePath):
			// A path has no match in B — hold for hash cross-reference
			unmatchedA[nextA.hashValue] = append(unmatchedA[nextA.hashValue], *nextA)
			nextProgress(nextA.relativePath)
			if nextA, err = readA(); err != nil {
				return writeErr(err)
			}

		default:
			// B path has no match in A — hold for hash cross-reference
			unmatchedB[nextB.hashValue] = append(unmatchedB[nextB.hashValue], *nextB)
			if softCompare && strings.ToLower(filepath.Ext(nextB.fileName)) == ".pdf" {
				key := softIndexKey(nextB.fileName, nextB.size)
				softBIndex[key] = append(softBIndex[key], *nextB)
			}
			nextProgress(nextB.relativePath)
			if nextB, err = readB(); err != nil {
				return writeErr(err)
			}
		}
	}

	// ── Pass 2: in-memory hash cross-reference ────────────────────────────────
	writeDiffRow := func(cols [11]string) error {
		return writeRow(cols[:])
	}
	emitDiff := func(payload DiffRowPayload) {
		result.differences++
		if diffRow != nil {
			diffRow(payload)
		}
	}

	for hash, aRows := range unmatchedA {
		bRows := unmatchedB[hash]
		if len(bRows) == 0 {
			// No hash match in B — try PDF soft compare before flagging as missing
			for _, r := range aRows {
				softMatched := false
				if softCompare && driveA.sourceDir != "" && driveB.sourceDir != "" &&
					strings.ToLower(filepath.Ext(r.fileName)) == ".pdf" {
					key := softIndexKey(r.fileName, r.size)
					if bCandidates, ok := softBIndex[key]; ok && len(bCandidates) > 0 {
						bMatch := bCandidates[0]
						pathA := filepath.Join(driveA.sourceDir, filepath.FromSlash(r.relativePath))
						pathB := filepath.Join(driveB.sourceDir, filepath.FromSlash(bMatch.relativePath))
						if pdfSoftMatch(pathA, pathB) {
							emitDiff(DiffRowPayload{
								DiffType: "metadata-only (PDF document IDs)",
								PathA:    r.relativePath,
								PathB:    bMatch.relativePath,
								SizeA:    strconv.FormatUint(r.size, 10),
								SizeB:    strconv.FormatUint(bMatch.size, 10),
								HashA:    r.hashValue,
								HashB:    bMatch.hashValue,
							})
							result.metadataOnlyDiffs++
							if err := writeDiffRow([11]string{
								"metadata-only (PDF document IDs)",
								r.relativePath, r.fileName,
								bMatch.relativePath, bMatch.fileName,
								strconv.FormatUint(r.size, 10), strconv.FormatUint(bMatch.size, 10),
								r.hashAlgorithm, bMatch.hashAlgorithm,
								r.hashValue, bMatch.hashValue,
							}); err != nil {
								return writeErr(err)
							}
							// Remove consumed B row from both indices so it's not flagged as extra
							softBIndex[key] = bCandidates[1:]
							if len(softBIndex[key]) == 0 {
								delete(softBIndex, key)
							}
							bHash := bMatch.hashValue
							remaining := unmatchedB[bHash]
							for i, rb := range remaining {
								if rb.relativePath == bMatch.relativePath {
									unmatchedB[bHash] = append(remaining[:i], remaining[i+1:]...)
									break
								}
							}
							if len(unmatchedB[bHash]) == 0 {
								delete(unmatchedB, bHash)
							}
							softMatched = true
						}
					}
				}
				if !softMatched {
					emitDiff(DiffRowPayload{
						DiffType: "missing from 2nd Drive (no match)",
						PathA:    r.relativePath,
						SizeA:    strconv.FormatUint(r.size, 10),
						HashA:    r.hashValue,
					})
					result.missingNoMatch++
					if err := writeDiffRow([11]string{
						"missing from 2nd Drive (no match)",
						r.relativePath, r.fileName,
						"", "",
						strconv.FormatUint(r.size, 10), "",
						r.hashAlgorithm, "",
						r.hashValue, "",
					}); err != nil {
						return writeErr(err)
					}
				}
			}
			continue
		}
		// Hash present on both sides — cross-match as moved/renamed + duplicates
		matchCount := len(aRows)
		if len(bRows) < matchCount {
			matchCount = len(bRows)
		}
		for i := 0; i < matchCount; i++ {
			a, b := aRows[i], bRows[i]
			emitDiff(DiffRowPayload{
				DiffType: "moved/renamed",
				PathA:    a.relativePath,
				PathB:    b.relativePath,
				SizeA:    strconv.FormatUint(a.size, 10),
				SizeB:    strconv.FormatUint(b.size, 10),
				HashA:    a.hashValue,
				HashB:    b.hashValue,
			})
			result.movedFiles++
			if err := writeDiffRow([11]string{
				"moved/renamed",
				a.relativePath, a.fileName,
				b.relativePath, b.fileName,
				strconv.FormatUint(a.size, 10), strconv.FormatUint(b.size, 10),
				a.hashAlgorithm, b.hashAlgorithm,
				a.hashValue, b.hashValue,
			}); err != nil {
				return writeErr(err)
			}
		}
		// Extra B copies beyond what A had
		for i := matchCount; i < len(bRows); i++ {
			b := bRows[i]
			emitDiff(DiffRowPayload{
				DiffType: "duplicate on 2nd Drive",
				PathB:    b.relativePath,
				SizeB:    strconv.FormatUint(b.size, 10),
				HashB:    b.hashValue,
			})
			result.duplicatesOnB++
			if err := writeDiffRow([11]string{
				"duplicate on 2nd Drive",
				"", "",
				b.relativePath, b.fileName,
				"", strconv.FormatUint(b.size, 10),
				"", b.hashAlgorithm,
				"", b.hashValue,
			}); err != nil {
				return writeErr(err)
			}
		}
		// Extra A copies beyond what B had
		for i := matchCount; i < len(aRows); i++ {
			a := aRows[i]
			emitDiff(DiffRowPayload{
				DiffType: "duplicate on 1st Drive",
				PathA:    a.relativePath,
				SizeA:    strconv.FormatUint(a.size, 10),
				HashA:    a.hashValue,
			})
			result.duplicatesOnA++
			if err := writeDiffRow([11]string{
				"duplicate on 1st Drive",
				a.relativePath, a.fileName,
				"", "",
				strconv.FormatUint(a.size, 10), "",
				a.hashAlgorithm, "",
				a.hashValue, "",
			}); err != nil {
				return writeErr(err)
			}
		}
		delete(unmatchedB, hash)
	}

	// Remaining unmatchedB entries have no hash anywhere on A (alarming)
	for _, bRows := range unmatchedB {
		for _, b := range bRows {
			emitDiff(DiffRowPayload{
				DiffType: "extra on 2nd Drive (no match)",
				PathB:    b.relativePath,
				SizeB:    strconv.FormatUint(b.size, 10),
				HashB:    b.hashValue,
			})
			result.extraNoMatch++
			if err := writeDiffRow([11]string{
				"extra on 2nd Drive (no match)",
				"", "",
				b.relativePath, b.fileName,
				"", strconv.FormatUint(b.size, 10),
				"", b.hashAlgorithm,
				"", b.hashValue,
			}); err != nil {
				return writeErr(err)
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return writeErr(err)
	}
	if err := diffFile.Close(); err != nil {
		return writeErr(err)
	}

	result.verdict = computeVerdict(result)
	result.elapsed = time.Since(startedAt)
	if err := writeCloneVerificationReport(result.reportPath, result); err != nil {
		cleanupScanArtifacts(diffPath, result.reportPath)
		return result, err
	}
	return result, nil
}

func writeCloneVerificationReport(reportPath string, result cloneVerificationDone) error {
	return os.WriteFile(reportPath, []byte(buildCloneVerificationReport(result)), 0o644)
}

func buildCloneVerificationReport(result cloneVerificationDone) string {
	lines := []string{
		"Clone Verification Report",
		fmt.Sprintf("Verdict: %s", result.verdict),
		strings.Repeat("━", 38),
		fmt.Sprintf("1st Drive folder: %s", valueOrDefault(result.driveA.sourceName, "unknown")),
		fmt.Sprintf("2nd Drive folder: %s", valueOrDefault(result.driveB.sourceName, "unknown")),
		fmt.Sprintf("1st Drive content list: %s", filepath.Base(result.driveA.outputPath)),
		fmt.Sprintf("2nd Drive content list: %s", filepath.Base(result.driveB.outputPath)),
		fmt.Sprintf("1st Drive summary report: %s", filepath.Base(result.driveA.reportPath)),
		fmt.Sprintf("2nd Drive summary report: %s", filepath.Base(result.driveB.reportPath)),
		fmt.Sprintf("Differences CSV: %s", filepath.Base(result.diffPath)),
		fmt.Sprintf("Verification hash: %s", result.hashAlgorithm.OptionLabel()),
		"",
		fmt.Sprintf("Exact path + content matches: %d", result.compared),
		fmt.Sprintf("Content matches (moved/renamed): %d", result.movedFiles),
		fmt.Sprintf("Metadata-only matches (PDF document IDs): %d", result.metadataOnlyDiffs),
		fmt.Sprintf("Hash mismatches: %d", result.hashMismatches),
		"",
		fmt.Sprintf("⚠ Missing from 2nd Drive (no hash match): %d", result.missingNoMatch),
		fmt.Sprintf("⚠ Extra on 2nd Drive (no hash match): %d", result.extraNoMatch),
		"",
		fmt.Sprintf("Duplicates on 2nd Drive: %d", result.duplicatesOnB),
		fmt.Sprintf("Duplicates on 1st Drive: %d", result.duplicatesOnA),
		fmt.Sprintf("System paths excluded: %d", result.excludedSystem),
		fmt.Sprintf("Finished in: %s", result.elapsed.Round(time.Millisecond)),
		"",
	}
	lines = append(lines, verdictSummaryLines(result)...)
	return strings.Join(lines, "\n") + "\n"
}

func verdictSummaryLines(result cloneVerificationDone) []string {
	switch result.verdict {
	case verdictMetadataClone:
		lines := []string{
			"METADATA CLONE — All file content verified present on both drives.",
			fmt.Sprintf("%d PDF file(s) differ only in embedded document IDs (export metadata), not content.", result.metadataOnlyDiffs),
			"No files are missing or corrupted. Both drives were independently exported from the same source.",
		}
		if result.movedFiles > 0 {
			lines = append(lines, fmt.Sprintf("%d file(s) also detected at different paths (folder renamed or moved).", result.movedFiles))
		}
		return lines
	case verdictExactClone:
		return []string{
			"EXACT CLONE — All files verified present on both drives at identical paths.",
			"No files are missing, moved, or corrupted.",
		}
	case verdictContentClone:
		lines := []string{
			fmt.Sprintf("CONTENT CLONE — All files verified present on both drives."),
			fmt.Sprintf("%d file(s) detected at different paths (folder renamed or moved).", result.movedFiles),
			"No files are missing or corrupted.",
		}
		if result.duplicatesOnB > 0 {
			lines = append(lines, fmt.Sprintf("%d extra duplicate(s) found on 2nd Drive.", result.duplicatesOnB))
		}
		if result.duplicatesOnA > 0 {
			lines = append(lines, fmt.Sprintf("%d extra duplicate(s) found on 1st Drive.", result.duplicatesOnA))
		}
		return lines
	default:
		lines := []string{"NOT A CLONE — Verification failed."}
		if result.missingNoMatch > 0 {
			lines = append(lines, fmt.Sprintf("%d file(s) missing from 2nd Drive with no hash match anywhere.", result.missingNoMatch))
		}
		if result.extraNoMatch > 0 {
			lines = append(lines, fmt.Sprintf("%d extra file(s) on 2nd Drive with no hash match anywhere.", result.extraNoMatch))
		}
		if result.hashMismatches > 0 {
			lines = append(lines, fmt.Sprintf("%d file(s) at matching paths have different hash values (possible corruption).", result.hashMismatches))
		}
		return lines
	}
}

func buildCloneVerificationSummary(result cloneVerificationDone) string {
	lines := []string{
		fmt.Sprintf("Verdict: %s", result.verdict),
		fmt.Sprintf("1st Drive folder: %s", valueOrDefault(result.driveA.sourceName, "unknown")),
		fmt.Sprintf("2nd Drive folder: %s", valueOrDefault(result.driveB.sourceName, "unknown")),
		fmt.Sprintf("Differences CSV: %s", filepath.Base(result.diffPath)),
		fmt.Sprintf("Clone report: %s", filepath.Base(result.reportPath)),
		fmt.Sprintf("Verification hash: %s", result.hashAlgorithm.OptionLabel()),
		fmt.Sprintf("Exact matches: %d", result.compared),
		fmt.Sprintf("Moved/renamed: %d", result.movedFiles),
		fmt.Sprintf("Hash mismatches: %d", result.hashMismatches),
		fmt.Sprintf("Missing (no match): %d", result.missingNoMatch),
		fmt.Sprintf("Extra (no match): %d", result.extraNoMatch),
		fmt.Sprintf("Finished in: %s", result.elapsed.Round(time.Millisecond)),
	}
	return strings.Join(lines, "\n")
}

func deleteDeferredScanCSVs(done *scanDoneMsg, deleteRequested bool) error {
	if done == nil {
		return nil
	}
	if !deleteRequested || !done.createXLSX {
		done.deleteCSV = deleteRequested
		return nil
	}
	for _, csvPath := range done.outputPaths {
		if err := os.Remove(csvPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	done.deleteCSV = true
	done.csvDeleted = true
	return writeScanReport(done.reportPath, *done)
}
