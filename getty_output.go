package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// The check produces two files beside the source: a corrected copy and a
// plain-text report. The source is never modified in place. Archival metadata
// is the record, and a tool that silently rewrites it leaves nothing to
// compare against when a correction turns out to be wrong.

// gettyCleanedPath names the corrected copy, keeping the original extension so
// it opens in the same application.
func gettyCleanedPath(sourcePath string) string {
	ext := filepath.Ext(sourcePath)
	return strings.TrimSuffix(sourcePath, ext) + "-tags-cleaned" + ext
}

// gettyReportPath names the report, matching the -report.txt convention the
// scan and clone reports already use.
func gettyReportPath(sourcePath string) string {
	ext := filepath.Ext(sourcePath)
	return strings.TrimSuffix(sourcePath, ext) + "-tags-report.txt"
}

// writeCleanedSheet writes a copy of the source with the Tags column corrected.
//
// Only cells the check actually changed are rewritten, so a sheet with two bad
// rows out of four thousand comes back byte-identical everywhere else.
func writeCleanedSheet(report TagSheetReport, destination string) error {
	corrections := map[int]string{}
	for _, row := range report.Rows {
		if row.Result.Changed() {
			corrections[row.Number] = row.Result.Cleaned
		}
	}

	switch report.Format {
	case sheetFormatXLSX:
		return writeCleanedXLSX(report, corrections, destination)
	case sheetFormatCSV:
		return writeCleanedDelimited(report, corrections, destination, ',')
	case sheetFormatTSV:
		return writeCleanedDelimited(report, corrections, destination, '\t')
	default:
		return fmt.Errorf("cannot write a cleaned copy of a %s file", report.Format)
	}
}

// writeCleanedXLSX edits the workbook in place and saves it elsewhere, rather
// than building a new one, so column widths, formats, and every untouched
// column survive exactly as the export produced them.
func writeCleanedXLSX(report TagSheetReport, corrections map[int]string, destination string) error {
	file, err := excelize.OpenFile(report.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(report.Path), err)
	}
	defer file.Close()

	for number, cleaned := range corrections {
		cell, err := excelize.CoordinatesToCellName(report.ColumnIndex+1, number)
		if err != nil {
			return fmt.Errorf("row %d: %w", number, err)
		}
		// SetCellStr rather than SetCellValue: a tag list is text, and a value
		// like "1890s" must not be coerced into a number or a date.
		if err := file.SetCellStr(report.SheetName, cell, cleaned); err != nil {
			return fmt.Errorf("write %s: %w", cell, err)
		}
	}

	if err := file.SaveAs(destination); err != nil {
		return fmt.Errorf("save %s: %w", filepath.Base(destination), err)
	}
	return nil
}

func writeCleanedDelimited(report TagSheetReport, corrections map[int]string, destination string, delimiter rune) error {
	var rows [][]string
	var err error
	if delimiter == '\t' {
		rows, err = readDelimitedRows(report.Path, '\t')
	} else {
		rows, err = readDelimitedRows(report.Path, ',')
	}
	if err != nil {
		return err
	}

	for number, cleaned := range corrections {
		index := number - 1 // report rows count the header as 1
		if index < 0 || index >= len(rows) {
			return fmt.Errorf("row %d is outside %s", number, filepath.Base(report.Path))
		}
		if report.ColumnIndex >= len(rows[index]) {
			continue
		}
		rows[index][report.ColumnIndex] = cleaned
	}

	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(destination), err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Comma = delimiter
	if err := writer.WriteAll(rows); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(destination), err)
	}
	writer.Flush()
	return writer.Error()
}

func writeGettyTagReport(reportPath string, report TagSheetReport) error {
	return os.WriteFile(reportPath, []byte(buildGettyTagReport(report)), 0o644)
}

func buildGettyTagReport(report TagSheetReport) string {
	lines := []string{
		"Getty Tag Report",
		fmt.Sprintf("Checked file: %s", filepath.Base(report.Path)),
		fmt.Sprintf("File type: %s", report.Format),
	}
	if report.SheetName != "" {
		lines = append(lines, fmt.Sprintf("Worksheet: %s", report.SheetName))
	}
	lines = append(lines,
		fmt.Sprintf("Tags column: %s", valueOrDefault(report.ColumnLetter, fmt.Sprintf("index %d", report.ColumnIndex))),
		fmt.Sprintf("Vocabulary: %s", valueOrDefault(report.VocabularySource, "not checked — structure only")),
		strings.Repeat("━", 38),
		fmt.Sprintf("Rows checked: %d", report.TotalRows),
		fmt.Sprintf("Empty Tags cells: %d", report.EmptyCells),
		fmt.Sprintf("Rows with findings: %d", len(report.Rows)),
		"",
		fmt.Sprintf("⚠ Would have failed to upload: %d", report.BlockingRows()),
		fmt.Sprintf("Repaired in the cleaned copy: %d", report.RepairedRows()),
		fmt.Sprintf("⚠ Still need review: %d", report.ReviewRows()),
		"",
	)

	if len(report.Rows) == 0 {
		lines = append(lines, "No problems found. The Tags column is ready to upload.", "")
		lines = append(lines, gettyAttribution(report)...)
		return strings.Join(lines, "\n") + "\n"
	}

	lines = append(lines, "Findings by row", strings.Repeat("─", 38))
	for _, row := range report.Rows {
		// Name the blocking rows here as well as in the count above, so the
		// "would have failed to upload" total can actually be traced to rows.
		heading := fmt.Sprintf("Row %d", row.Number)
		if row.Result.OriginalBlocksUpload() {
			heading += " ⚠ would have stopped the upload"
		}
		lines = append(lines, heading)
		lines = append(lines, fmt.Sprintf("  was: %s", row.Result.Original))
		if row.Result.Changed() {
			lines = append(lines, fmt.Sprintf("  now: %s", row.Result.Cleaned))
		}
		for _, issue := range row.Result.Issues {
			lines = append(lines, fmt.Sprintf("  %s %s: %s",
				issueMarker(issue), issue.Kind, issue.Detail))
		}
		lines = append(lines, "")
	}

	lines = append(lines,
		"Legend",
		strings.Repeat("─", 38),
		"  ✓  repaired in the cleaned copy, nothing to do",
		"  ?  needs a person to decide",
		"  ⚠  marks a row whose Tags cell would have stopped the upload;",
		"     it is repaired in the cleaned copy unless a ⚠ appears on the finding itself",
		"",
	)
	lines = append(lines, gettyAttribution(report)...)
	return strings.Join(lines, "\n") + "\n"
}

// issueMarker distinguishes the three things a reader needs to tell apart at a
// glance: what was fixed, what would have broken the upload, and what is left
// for them to decide.
func issueMarker(issue TagIssue) string {
	switch {
	case issue.Repaired:
		return "✓"
	case issue.Severity == severityBlocking:
		return "⚠"
	default:
		return "?"
	}
}

// gettyAttribution credits the Getty Research Institute whenever the report
// contains results derived from their vocabulary, as ODC-By 1.0 requires.
func gettyAttribution(report TagSheetReport) []string {
	if report.VocabularySource == "" {
		return nil
	}
	return []string{
		strings.Repeat("─", 38),
		"Term verification uses the Getty Art & Architecture Thesaurus (AAT),",
		"J. Paul Getty Trust, under the Open Data Commons Attribution License",
		"(ODC-By) 1.0.",
	}
}
