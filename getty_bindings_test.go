package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These exercise the binding the screen actually calls, rather than the
// checking code underneath it, so a mistake in the glue shows up here instead
// of in a manual click-through.

func TestCheckGettyTagsWritesBothOutputs(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial\u00A0photographs; landscapes; city plans"),
		rowWithTags("aerial photographs; landscapes"),
	})

	app := newApp("")
	result, err := app.CheckGettyTags(GettyCheckOptions{
		SheetPath:    path,
		Source:       gettySourceNone,
		WriteCleaned: true,
		WriteReport:  true,
	})
	if err != nil {
		t.Fatalf("CheckGettyTags: %v", err)
	}

	if result.CleanedPath == "" {
		t.Fatal("expected a cleaned copy to be written")
	}
	if _, err := os.Stat(result.CleanedPath); err != nil {
		t.Fatalf("cleaned copy missing: %v", err)
	}
	if result.ReportPath == "" {
		t.Fatal("expected a report to be written")
	}
	if _, err := os.Stat(result.ReportPath); err != nil {
		t.Fatalf("report missing: %v", err)
	}
	if result.Summary == "" {
		t.Fatal("expected a summary for the screen to show")
	}
	if result.Elapsed == "" {
		t.Fatal("expected an elapsed time rendered by the binding")
	}
}

// Writing an identical file would only raise the question of which one to
// upload.
func TestCheckGettyTagsSkipsTheCleanedCopyWhenNothingChanged(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; landscapes; city plans"),
	})

	app := newApp("")
	result, err := app.CheckGettyTags(GettyCheckOptions{
		SheetPath:    path,
		Source:       gettySourceNone,
		WriteCleaned: true,
		WriteReport:  false,
	})
	if err != nil {
		t.Fatalf("CheckGettyTags: %v", err)
	}
	if result.CleanedPath != "" {
		t.Fatalf("a clean sheet needs no cleaned copy, got %q", result.CleanedPath)
	}
	if _, err := os.Stat(gettyCleanedPath(path)); err == nil {
		t.Fatal("no cleaned file should have been created")
	}
}

func TestCheckGettyTagsVerifiesAgainstATermList(t *testing.T) {
	listPath := filepath.Join(t.TempDir(), "AAT Tags.csv")
	if err := os.WriteFile(listPath, []byte("aerial photographs\nlandscapes\ncity plans\n"), 0o644); err != nil {
		t.Fatalf("write list: %v", err)
	}
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; invented term; city plans"),
	})

	app := newApp("")
	result, err := app.CheckGettyTags(GettyCheckOptions{
		SheetPath:      path,
		Source:         gettySourceFile,
		VocabularyPath: listPath,
		WriteReport:    true,
	})
	if err != nil {
		t.Fatalf("CheckGettyTags: %v", err)
	}
	if result.Report.VocabularySource == "" {
		t.Fatal("the report should record which list answered")
	}
	if len(result.Report.Rows) != 1 {
		t.Fatalf("expected 1 row with findings, got %d", len(result.Report.Rows))
	}
}

func TestCheckGettyTagsReportsUsableErrors(t *testing.T) {
	app := newApp("")

	for _, tc := range []struct {
		name    string
		options GettyCheckOptions
		want    string
	}{
		{
			name:    "no sheet chosen",
			options: GettyCheckOptions{Source: gettySourceNone},
			want:    "choose an exported sheet",
		},
		{
			name:    "sheet does not exist",
			options: GettyCheckOptions{SheetPath: filepath.Join(t.TempDir(), "absent.xlsx"), Source: gettySourceNone},
			want:    "cannot be opened",
		},
		{
			name:    "term list chosen but no path",
			options: GettyCheckOptions{SheetPath: writeXLSX(t, [][]string{mainTableHeader}), Source: gettySourceFile},
			want:    "choose a term list",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := app.CheckGettyTags(tc.options)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}

// An Access database is the likeliest wrong choice, since it is what the
// workflow starts from. The error has to say what to do instead.
func TestCheckGettyTagsGuidesTheUserOffAnAccessFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CONTENTdm template.accdb")
	if err := os.WriteFile(path, []byte("not really a database"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	app := newApp("")
	_, err := app.CheckGettyTags(GettyCheckOptions{SheetPath: path, Source: gettySourceNone})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "export the Main table") {
		t.Fatalf("error = %q, want it to say what to do instead", err.Error())
	}
}

func TestBuildVocabularySelectsTheRightSource(t *testing.T) {
	app := newApp("")

	if v, err := app.buildVocabulary(GettyCheckOptions{Source: gettySourceNone}); err != nil || v != nil {
		t.Fatalf("structure-only should use no vocabulary, got %v, %v", v, err)
	}
	if v, err := app.buildVocabulary(GettyCheckOptions{Source: ""}); err != nil || v != nil {
		t.Fatalf("an unset source should use no vocabulary, got %v, %v", v, err)
	}

	live, err := app.buildVocabulary(GettyCheckOptions{Source: gettySourceLive})
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	if !strings.Contains(live.SourceName(), "vocab.getty.edu") {
		t.Fatalf("live source name = %q", live.SourceName())
	}

	if _, err := app.buildVocabulary(GettyCheckOptions{Source: "nonsense"}); err == nil {
		t.Fatal("expected an error for an unknown source")
	}
}

// Editing writes to the cleaned copy. The export is the record of what Access
// held, and editing it in place destroys the only thing a correction can be
// checked against.
func TestSaveGettyTagEditsNeverTouchesTheSource(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("landscapes; invented term; city plans"),
	})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	app := newApp("")
	result, err := app.SaveGettyTagEdits(
		GettyCheckOptions{SheetPath: path, Source: gettySourceNone},
		[]GettyTagEdit{{Row: 2, Tags: "coastal landscapes; aerial photographs; city plans"}},
	)
	if err != nil {
		t.Fatalf("SaveGettyTagEdits: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read source: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("the source file was modified")
	}
	if result.CleanedPath != gettyCleanedPath(path) {
		t.Fatalf("CleanedPath = %q", result.CleanedPath)
	}
}

func TestSaveGettyTagEditsAppliesTheEdit(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("landscapes; invented term; city plans"),
		rowWithTags("aerial photographs; landscapes; city plans"),
	})

	app := newApp("")
	result, err := app.SaveGettyTagEdits(
		GettyCheckOptions{SheetPath: path, Source: gettySourceNone},
		[]GettyTagEdit{{Row: 2, Tags: "coastal landscapes; aerial photographs; city plans"}},
	)
	if err != nil {
		t.Fatalf("SaveGettyTagEdits: %v", err)
	}

	rows, err := readXLSXRowsForTest(t, result.CleanedPath)
	if err != nil {
		t.Fatalf("read cleaned: %v", err)
	}
	want := "coastal landscapes; aerial photographs; city plans"
	if got := rows[1][len(mainTableHeader)-1]; got != want {
		t.Fatalf("edited row = %q, want %q", got, want)
	}
	// The untouched row keeps what it had.
	if got := rows[2][len(mainTableHeader)-1]; got != "aerial photographs; landscapes; city plans" {
		t.Fatalf("untouched row changed: %q", got)
	}
}

// A term pasted from the Getty website arrives with the same invisible
// characters as anything else pasted from the Getty website.
func TestSaveGettyTagEditsCleansWhatWasTyped(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("landscapes; invented term; city plans"),
	})

	app := newApp("")
	result, err := app.SaveGettyTagEdits(
		GettyCheckOptions{SheetPath: path, Source: gettySourceNone},
		[]GettyTagEdit{{Row: 2, Tags: "\uFEFFcoastal\u00A0landscapes;aerial photographs ; city plans  "}},
	)
	if err != nil {
		t.Fatalf("SaveGettyTagEdits: %v", err)
	}

	rows, err := readXLSXRowsForTest(t, result.CleanedPath)
	if err != nil {
		t.Fatalf("read cleaned: %v", err)
	}
	want := "coastal landscapes; aerial photographs; city plans"
	if got := rows[1][len(mainTableHeader)-1]; got != want {
		t.Fatalf("edited row = %q, want %q", got, want)
	}
}

// An edit can introduce a problem as easily as fix one, so the saved file is
// re-checked and the fresh report returned.
func TestSaveGettyTagEditsReportsWhatWasActuallySaved(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; landscapes; city plans"),
	})

	app := newApp("")
	result, err := app.SaveGettyTagEdits(
		GettyCheckOptions{SheetPath: path, Source: gettySourceNone},
		[]GettyTagEdit{{Row: 2, Tags: "only one tag"}},
	)
	if err != nil {
		t.Fatalf("SaveGettyTagEdits: %v", err)
	}
	if len(result.Report.Rows) != 1 {
		t.Fatalf("expected the re-check to flag the edit, got %+v", result.Report.Rows)
	}
	if !hasIssue(result.Report.Rows[0].Result, tagIssueTagCount) {
		t.Fatalf("expected a tag-count finding on the saved file, got %+v", result.Report.Rows[0].Result.Issues)
	}
}

func TestSaveGettyTagEditsIgnoresBlankEdits(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; landscapes; city plans"),
	})

	app := newApp("")
	result, err := app.SaveGettyTagEdits(
		GettyCheckOptions{SheetPath: path, Source: gettySourceNone},
		[]GettyTagEdit{{Row: 2, Tags: "   "}},
	)
	if err != nil {
		t.Fatalf("SaveGettyTagEdits: %v", err)
	}
	rows, err := readXLSXRowsForTest(t, result.CleanedPath)
	if err != nil {
		t.Fatalf("read cleaned: %v", err)
	}
	if got := rows[1][len(mainTableHeader)-1]; got != "aerial photographs; landscapes; city plans" {
		t.Fatalf("a blank edit must not erase the cell, got %q", got)
	}
}

func readXLSXRowsForTest(t *testing.T, path string) ([][]string, error) {
	t.Helper()
	rows, _, err := readXLSXRows(path)
	return rows, err
}
