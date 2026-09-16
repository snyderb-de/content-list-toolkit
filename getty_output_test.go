package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestGettyOutputPathsSitBesideTheSource(t *testing.T) {
	for _, tc := range []struct {
		source      string
		wantCleaned string
		wantReport  string
	}{
		{
			source:      filepath.Join("X", "collection.xlsx"),
			wantCleaned: filepath.Join("X", "collection-tags-cleaned.xlsx"),
			wantReport:  filepath.Join("X", "collection-tags-report.txt"),
		},
		{
			source:      "export.csv",
			wantCleaned: "export-tags-cleaned.csv",
			wantReport:  "export-tags-report.txt",
		},
	} {
		if got := gettyCleanedPath(tc.source); got != tc.wantCleaned {
			t.Fatalf("gettyCleanedPath(%q) = %q, want %q", tc.source, got, tc.wantCleaned)
		}
		if got := gettyReportPath(tc.source); got != tc.wantReport {
			t.Fatalf("gettyReportPath(%q) = %q, want %q", tc.source, got, tc.wantReport)
		}
	}
}

// The source is the record. A corrected copy is written beside it, never over
// it.
func TestWriteCleanedSheetLeavesTheSourceUntouched(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial\u00A0photographs; landscapes; city plans"),
	})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	if err := writeCleanedSheet(report, nil, gettyCleanedPath(path)); err != nil {
		t.Fatalf("writeCleanedSheet: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read source: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("the source file was modified")
	}
}

func TestCleanedXLSXCarriesTheCorrectedTags(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial\u00A0photographs;\u200Blandscapes ; city\tplans"),
		rowWithTags("aerial photographs; landscapes; city plans"),
	})

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	cleanedPath := gettyCleanedPath(path)
	if err := writeCleanedSheet(report, nil, cleanedPath); err != nil {
		t.Fatalf("writeCleanedSheet: %v", err)
	}

	file, err := excelize.OpenFile(cleanedPath)
	if err != nil {
		t.Fatalf("open cleaned: %v", err)
	}
	defer file.Close()

	rows, err := file.GetRows(file.GetSheetList()[0])
	if err != nil {
		t.Fatalf("read cleaned rows: %v", err)
	}
	want := "aerial photographs; landscapes; city plans"
	if got := rows[1][report.ColumnIndex]; got != want {
		t.Fatalf("row 2 = %q, want %q", got, want)
	}
	// The already-clean row must be untouched.
	if got := rows[2][report.ColumnIndex]; got != want {
		t.Fatalf("row 3 = %q, want %q", got, want)
	}
	// Every other column survives.
	if got := rows[1][2]; got != "Aerial view of Dover" {
		t.Fatalf("Title column = %q, want it preserved", got)
	}
}

// Re-checking the cleaned copy must find nothing. If cleaning did not converge,
// the output is not actually ready to upload.
func TestCleanedOutputPassesASecondCheck(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("\uFEFFaerial\u00A0photographs;\u200Blandscapes ; city\tplans  "),
	})

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	cleanedPath := gettyCleanedPath(path)
	if err := writeCleanedSheet(report, nil, cleanedPath); err != nil {
		t.Fatalf("writeCleanedSheet: %v", err)
	}

	second, err := checkTagSheet(cleanedPath)
	if err != nil {
		t.Fatalf("re-check: %v", err)
	}
	if len(second.Rows) != 0 {
		t.Fatalf("the cleaned copy still has findings: %+v", second.Rows)
	}
}

func TestCleanedCSVKeepsEveryOtherColumn(t *testing.T) {
	path := writeText(t, "export.csv",
		"Title,Tags,Notes\n"+
			"Aerial view,\"aerial\u00A0photographs;landscapes; city plans\",keep me\n")

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	cleanedPath := gettyCleanedPath(path)
	if err := writeCleanedSheet(report, nil, cleanedPath); err != nil {
		t.Fatalf("writeCleanedSheet: %v", err)
	}

	rows, err := readDelimitedRows(cleanedPath, ',')
	if err != nil {
		t.Fatalf("read cleaned: %v", err)
	}
	if rows[1][1] != "aerial photographs; landscapes; city plans" {
		t.Fatalf("Tags = %q", rows[1][1])
	}
	if rows[1][0] != "Aerial view" || rows[1][2] != "keep me" {
		t.Fatalf("neighbouring columns changed: %q", rows[1])
	}
}

func TestCleanedTabDelimitedRoundTrips(t *testing.T) {
	path := writeText(t, "export.txt",
		"Title\tTags\n"+
			"Aerial view\taerial\u00A0photographs; landscapes; city plans\n")

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	cleanedPath := gettyCleanedPath(path)
	if err := writeCleanedSheet(report, nil, cleanedPath); err != nil {
		t.Fatalf("writeCleanedSheet: %v", err)
	}

	second, err := checkTagSheet(cleanedPath)
	if err != nil {
		t.Fatalf("re-check: %v", err)
	}
	if len(second.Rows) != 0 {
		t.Fatalf("cleaned tab-delimited copy still has findings: %+v", second.Rows)
	}
}

func TestReportNamesTheFindingsByRow(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial\u00A0photographs; landscapes; city plans"),
		rowWithTags("landscapes; Landscapes; city plans"),
	})

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	text := buildGettyTagReport(report)

	for _, want := range []string{
		"Getty Tag Report",
		"Tags column: AF",
		"Would have failed to upload: 1",
		"Still need review: 1",
		"Row 2",
		"Row 3",
		"ghost-characters",
		"duplicate-tag",
		"Fixed:",
		"Review:",
		"Legend",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}

// A structure-only run must not imply a vocabulary was consulted, and must not
// carry an attribution for data it never used.
func TestReportSaysWhenNoVocabularyWasConsulted(t *testing.T) {
	path := writeXLSX(t, [][]string{mainTableHeader, rowWithTags("a; b")})
	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	text := buildGettyTagReport(report)

	if !strings.Contains(text, "structure only") {
		t.Fatalf("report should say no vocabulary was consulted:\n%s", text)
	}
	if strings.Contains(text, "ODC-By") {
		t.Fatal("attribution must not appear when no Getty data was used")
	}
}

// ODC-By 1.0 requires crediting the Getty Research Institute for results
// derived from their vocabulary.
func TestReportCreditsGettyWhenTermsWereVerified(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; invented term; city plans"),
	})
	report, err := checkTagSheetWithVocabulary(context.Background(), path, verifyVocabulary(t))
	if err != nil {
		t.Fatalf("checkTagSheetWithVocabulary: %v", err)
	}
	text := buildGettyTagReport(report)

	if !strings.Contains(text, "ODC-By") || !strings.Contains(text, "J. Paul Getty Trust") {
		t.Fatalf("report must credit Getty when terms were verified:\n%s", text)
	}
	if !strings.Contains(text, "unknown-term") {
		t.Fatalf("report should carry the unknown term finding:\n%s", text)
	}
}

func TestReportReadsPlainlyForACleanSheet(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; landscapes; city plans"),
	})
	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	text := buildGettyTagReport(report)

	if !strings.Contains(text, "No problems found") {
		t.Fatalf("a clean sheet should say so plainly:\n%s", text)
	}
	if strings.Contains(text, "Findings by row") {
		t.Fatal("a clean sheet should not print an empty findings section")
	}
}

func TestWriteGettyTagReportWritesTheFile(t *testing.T) {
	path := writeXLSX(t, [][]string{mainTableHeader, rowWithTags("a; b")})
	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	reportPath := gettyReportPath(path)
	if err := writeGettyTagReport(reportPath, report); err != nil {
		t.Fatalf("writeGettyTagReport: %v", err)
	}
	contents, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.HasPrefix(string(contents), "Getty Tag Report") {
		t.Fatalf("unexpected report start: %q", string(contents[:40]))
	}
}

func TestIssuePrefixStatesWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		issue TagIssue
		want  string
	}{
		{
			name:  "repaired",
			issue: TagIssue{Kind: tagIssueGhostCharacters, Severity: severityBlocking, Repaired: true},
			want:  "Fixed:",
		},
		{
			name:  "blocking and unrepaired",
			issue: TagIssue{Kind: tagIssueGhostCharacters, Severity: severityBlocking},
			want:  "Must Fix:",
		},
		{
			// Not blocking, but the substantive finding, so it must not be
			// filed alongside judgement calls.
			name:  "term absent from the vocabulary",
			issue: TagIssue{Kind: tagIssueUnknownTerm, Severity: severityReview},
			want:  "Must Fix:",
		},
		{
			name:  "damaged text",
			issue: TagIssue{Kind: tagIssueDamagedText, Severity: severityReview},
			want:  "Must Fix:",
		},
		{
			name:  "judgement call",
			issue: TagIssue{Kind: tagIssueTagCount, Severity: severityReview},
			want:  "Review:",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := issuePrefix(tc.issue); got != tc.want {
				t.Fatalf("issuePrefix = %q, want %q", got, tc.want)
			}
		})
	}
}

// The "would have failed to upload" count is only useful if the rows it counts
// can be found. A blocking row is named as such in the findings list.
func TestReportNamesWhichRowsWouldHaveBlockedTheUpload(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("landscapes; Landscapes; city plans"),
		rowWithTags("aerial\u00A0photographs; landscapes; city plans"),
	})
	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	text := buildGettyTagReport(report)

	if !strings.Contains(text, "Row 3  (would have stopped the upload)") {
		t.Fatalf("the blocking row should be marked:\n%s", text)
	}
	if strings.Contains(text, "Row 2  (would have stopped") {
		t.Fatalf("a duplicate tag does not block the upload:\n%s", text)
	}
}
