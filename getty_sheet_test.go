package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// mainTableHeader mirrors the column order of the [Main] table in the
// CONTENTdm Access template, so tests exercise a Tags column sitting well to
// the right of column A rather than a convenient first position.
var mainTableHeader = []string{
	"ID", "Resource-ID", "Title", "Creator", "Topic(subject)", "Location(TGN)",
	"Description", "Publisher", "Date-Digital", "Date-Original", "Type",
	"Contributing-Institution", "Collection", "Rights-Management", "File-Name",
	"RG", "Sub-Group", "Series", "Sub-Series", "RG-Name", "SubGroup-Name",
	"Series-Name", "SubSeries-Name", "Confidential", "Folder-Name", "Notes",
	"Folder-Number", "Box-Number", "Barcode", "Record Type",
	"Record Level Name", "Tags",
}

func rowWithTags(tags string) []string {
	row := make([]string, len(mainTableHeader))
	row[0] = "1"
	row[2] = "Aerial view of Dover"
	row[len(row)-1] = tags
	return row
}

func writeXLSX(t *testing.T, rows [][]string) string {
	t.Helper()
	file := excelize.NewFile()
	t.Cleanup(func() { file.Close() })

	sheet := file.GetSheetList()[0]
	for r, row := range rows {
		for c, value := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				t.Fatalf("cell name: %v", err)
			}
			if err := file.SetCellStr(sheet, cell, value); err != nil {
				t.Fatalf("set cell: %v", err)
			}
		}
	}

	path := filepath.Join(t.TempDir(), "export.xlsx")
	if err := file.SaveAs(path); err != nil {
		t.Fatalf("save xlsx: %v", err)
	}
	return path
}

func writeText(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestCheckTagSheetFindsTagsInAnXLSXExport(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; landscapes; city plans"),
		rowWithTags("aerial\u00A0photographs; landscapes; city plans"),
		rowWithTags("aerial photographs; landscapes"),
	})

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	if report.Format != sheetFormatXLSX {
		t.Fatalf("format = %q, want xlsx", report.Format)
	}
	if report.TotalRows != 3 {
		t.Fatalf("TotalRows = %d, want 3", report.TotalRows)
	}
	// Tags is the 32nd column of the Main table.
	if report.ColumnLetter != "AF" {
		t.Fatalf("ColumnLetter = %q, want AF", report.ColumnLetter)
	}
	// The clean row produces no output at all.
	if len(report.Rows) != 2 {
		t.Fatalf("expected 2 rows with findings, got %d", len(report.Rows))
	}
	if report.Rows[0].Number != 3 {
		t.Fatalf("first finding is on row %d, want 3 (header counts as row 1)", report.Rows[0].Number)
	}
	if report.BlockingRows() != 1 {
		t.Fatalf("BlockingRows = %d, want 1", report.BlockingRows())
	}
	if report.ReviewRows() != 1 {
		t.Fatalf("ReviewRows = %d, want 1", report.ReviewRows())
	}
}

func TestCheckTagSheetReadsCSV(t *testing.T) {
	path := writeText(t, "export.csv",
		strings.Join(mainTableHeader, ",")+"\n"+
			`1,,Aerial view,,,,,,,,,,,,,,,,,,,,,,,,,,,,,"aerial`+"\u00A0"+`photographs; landscapes; city plans"`+"\n")

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	if report.Format != sheetFormatCSV {
		t.Fatalf("format = %q, want csv", report.Format)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("expected 1 row with findings, got %d", len(report.Rows))
	}
	if !report.Rows[0].Result.OriginalBlocksUpload() {
		t.Fatal("a no-break space should be reported as blocking")
	}
}

// The tab-delimited file is what the upload actually consumes, so a file that
// already failed can be checked directly.
func TestCheckTagSheetReadsTabDelimited(t *testing.T) {
	path := writeText(t, "export.tsv",
		strings.Join(mainTableHeader, "\t")+"\n"+
			strings.Join(rowWithTags("aerial photographs; landscapes; city plans"), "\t")+"\n")

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	if report.Format != sheetFormatTSV {
		t.Fatalf("format = %q, want tab-delimited", report.Format)
	}
	if len(report.Rows) != 0 {
		t.Fatalf("expected a clean sheet, got %+v", report.Rows)
	}
}

// Excel writes a BOM when saving CSV as UTF-8. Left in place it becomes part
// of the first header name.
func TestCheckTagSheetStripsAUTF8BOM(t *testing.T) {
	path := writeText(t, "export.csv",
		"\uFEFF"+strings.Join([]string{"Tags", "Title"}, ",")+"\n"+
			"aerial photographs; landscapes; city plans,Aerial view\n")

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	if report.ColumnIndex != 0 {
		t.Fatalf("ColumnIndex = %d, want 0", report.ColumnIndex)
	}
}

// The header cell is as likely to carry a pasted invisible character as the
// data is.
func TestFindTagsColumnToleratesAGhostCharacterInTheHeader(t *testing.T) {
	column, err := findTagsColumn([]string{"Title", "\uFEFFTags\u00A0", "Notes"})
	if err != nil {
		t.Fatalf("findTagsColumn: %v", err)
	}
	if column != 1 {
		t.Fatalf("column = %d, want 1", column)
	}
}

func TestFindTagsColumnIsCaseInsensitive(t *testing.T) {
	for _, header := range []string{"tags", "TAGS", "Tags"} {
		if _, err := findTagsColumn([]string{"Title", header}); err != nil {
			t.Fatalf("%q: %v", header, err)
		}
	}
}

// A missing column usually means the wrong export was picked, so the error
// names what was actually in the file.
func TestFindTagsColumnErrorListsTheColumnsItFound(t *testing.T) {
	_, err := findTagsColumn([]string{"Title", "Creator", "Notes"})
	if !errors.Is(err, errNoTagsColumn) {
		t.Fatalf("expected errNoTagsColumn, got %v", err)
	}
	for _, want := range []string{"Title", "Creator", "Notes"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should name the columns found, got %q", err.Error())
		}
	}
}

func TestCheckTagSheetCountsEmptyCells(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; landscapes; city plans"),
		rowWithTags(""),
		rowWithTags("   "),
		{"1", "", "Short row that stops before the Tags column"},
	})

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	if report.TotalRows != 4 {
		t.Fatalf("TotalRows = %d, want 4", report.TotalRows)
	}
	if report.EmptyCells != 3 {
		t.Fatalf("EmptyCells = %d, want 3", report.EmptyCells)
	}
	if len(report.Rows) != 0 {
		t.Fatalf("empty cells are not findings, got %+v", report.Rows)
	}
}

func TestDetectSheetFormatGuidesTheUserOffUnsupportedFiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "access database", path: "CONTENTdm template.accdb", want: "export the Main table"},
		{name: "legacy excel", path: "export.xls", want: "re-save it as .xlsx"},
		{name: "unrelated", path: "notes.pdf", want: "unsupported file type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := detectSheetFormat(tc.path)
			if err == nil {
				t.Fatalf("expected an error for %s", tc.path)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestSummaryReportsACleanSheetPlainly(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; landscapes; city plans"),
	})
	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	if got := report.Summary(); !strings.Contains(got, "clean") {
		t.Fatalf("Summary = %q, want it to say the sheet is clean", got)
	}
}
