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
		rowWithTags("aerial photographs; landscapes; city plans"),
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
