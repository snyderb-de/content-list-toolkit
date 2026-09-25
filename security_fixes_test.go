package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestSpreadsheetSafeCellAndRoundTrip(t *testing.T) {
	for _, value := range []string{"=1+1", "+1+1", "-1+1", "@SUM(1)", "  =1+1", "\t=1+1", "\r=1+1", "\n=1+1", "＝1+1", "\x1b=1+1"} {
		safe := spreadsheetSafeCell(value)
		if !strings.HasPrefix(safe, "\t") || safe == value {
			t.Errorf("unsafe cell %q was not encoded: %q", value, safe)
		}
		if got := spreadsheetOriginalCell(safe); got != value {
			t.Errorf("round trip %q: got %q", value, got)
		}
	}
	for _, value := range []string{"alpha", "a=1+1", "  alpha", "-1", "+1.5", ""} {
		if got := spreadsheetSafeCell(value); got != value {
			t.Errorf("ordinary cell %q changed to %q", value, got)
		}
	}
}

func TestScanCSVFormulaTextAndXLSXValue(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "=1+1"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	done, err := runScan(source, filepath.Join(workspace, "report.csv"), scanOptions{CreateXLSX: true})
	if err != nil {
		t.Fatal(err)
	}
	rows := readCSVRows(t, done.outputPath)
	if len(rows) != 2 || rows[1][0] != "\t=1+1" || rows[1][4] != "\t=1+1" {
		t.Fatalf("formula-leading name was not encoded in CSV: %q", rows)
	}
	rawCSV, err := os.ReadFile(done.outputPath)
	if err != nil || !strings.Contains(string(rawCSV), "\"\t=1+1\"") {
		t.Fatalf("protective tab must be inside a quoted CSV field: %q, %v", rawCSV, err)
	}
	book, err := excelize.OpenFile(done.xlsxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	value, err := book.GetCellValue("Sheet1", "A2")
	if err != nil || value != "=1+1" {
		t.Fatalf("XLSX should preserve the original name as text: %q, %v", value, err)
	}
	formula, err := book.GetCellFormula("Sheet1", "A2")
	if err != nil || formula != "" {
		t.Fatalf("XLSX must not contain a formula: %q, %v", formula, err)
	}
}

func TestFolderAndCloneCSVFormulaText(t *testing.T) {
	workspace := t.TempDir()
	sourceA := filepath.Join(workspace, "source-a")
	sourceB := filepath.Join(workspace, "source-b")
	for _, source := range []string{sourceA, sourceB} {
		if err := os.Mkdir(source, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(sourceA, "=1+1"), 0o755); err != nil {
		t.Fatal(err)
	}
	folders, err := runScan(sourceA, filepath.Join(workspace, "folders.csv"), scanOptions{FoldersOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if rows := readCSVRows(t, folders.outputPath); len(rows) != 2 || rows[1][0] != "\t=1+1" {
		t.Fatalf("folder-only CSV did not encode name: %q", rows)
	}
	if err := os.WriteFile(filepath.Join(sourceA, "=1+1"+".txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := runScan(sourceA, filepath.Join(workspace, "a.csv"), scanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := runScan(sourceB, filepath.Join(workspace, "b.csv"), scanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	diffPath := filepath.Join(workspace, "diff.csv")
	_, err = compareScanOutputs(context.Background(), a, b, diffPath, filepath.Join(workspace, "diff-report.txt"), false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := readCSVRows(t, diffPath)
	if len(rows) != 2 || rows[1][1] != "\t=1+1.txt" || rows[1][2] != "\t=1+1.txt" {
		t.Fatalf("clone CSV did not encode restored name: %q", rows)
	}
}

func TestGettyDelimitedOutputsRefuseFormulaCells(t *testing.T) {
	workspace := t.TempDir()
	input := filepath.Join(workspace, "input.csv")
	output := filepath.Join(workspace, "cleaned.csv")
	if err := os.WriteFile(input, []byte("Tags,Other\nnormal,=1+1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := TagSheetReport{Path: input, ColumnIndex: 0}
	if err := writeCleanedDelimited(report, nil, output, ','); err == nil || !strings.Contains(err.Error(), "spreadsheet formula") {
		t.Fatalf("expected explicit formula rejection, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("rejected output must not be created: %v", err)
	}
	if err := os.WriteFile(input, []byte("Tags,Other\nnormal,text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeCleanedDelimited(report, nil, output, ','); err != nil {
		t.Fatalf("ordinary cleaned CSV failed: %v", err)
	}
	if rows := readCSVRows(t, output); len(rows) != 2 || rows[1][1] != "text" {
		t.Fatalf("ordinary cleaned CSV changed: %q", rows)
	}
	if err := os.WriteFile(input, []byte("Tags,Other\nnormal,-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeCleanedDelimited(report, nil, output, ','); err != nil {
		t.Fatalf("signed numeric metadata should remain usable: %v", err)
	}
	if rows := readCSVRows(t, output); rows[1][1] != "-1" {
		t.Fatalf("signed numeric metadata changed: %q", rows)
	}
	termsPath := filepath.Join(workspace, "terms.csv")
	if err := writeTermList(termsPath, []vocabularyTerm{{Term: "=1+1"}}); err == nil {
		t.Fatal("formula-leading Getty term should be rejected")
	}
	if _, err := os.Stat(termsPath); !os.IsNotExist(err) {
		t.Fatalf("rejected term list must not be created: %v", err)
	}
}

func TestEmailCopyRejectsOutsideSourceAndDestinationLinks(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "source")
	dest := filepath.Join(workspace, "dest")
	outside := filepath.Join(workspace, "outside.eml")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "link.eml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, err := copyEmailFiles(source, dest); err == nil {
		t.Fatal("outside source symlink was copied")
	}
	if err := os.Remove(filepath.Join(source, "link.eml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mail.eml"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "mail.eml")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := copyEmailFiles(source, dest); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("outside destination symlink should be rejected, got %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "outside" {
		t.Fatalf("outside file changed: %q, %v", data, err)
	}
}

func TestGettyArchiveLimitsRejectAmplifiedInput(t *testing.T) {
	terms := term("1001", "3001", "alpha", termPreferred) + term("1002", "3002", "beta", termPreferred)
	languages := language(aatEnglishLanguageID, "1001") + language(aatEnglishLanguageID, "1002")
	archive := writeTestArchive(t, terms, languages)
	if _, err := extractEnglishTermsWithLimits(archive, gettyArchiveLimits{maxTableBytes: 1 << 20, maxEntries: 1}); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("expected record limit, got %v", err)
	}
	if _, err := extractEnglishTermsWithLimits(archive, gettyArchiveLimits{maxTableBytes: 20, maxEntries: 10}); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expected decompressed size limit, got %v", err)
	}
	if _, err := extractEnglishTermsWithLimits(archive, gettyArchiveLimits{maxTableBytes: 1 << 20, maxEntries: 10, maxRetainedBytes: 300}); err == nil || !strings.Contains(err.Error(), "retained data limit") {
		t.Fatalf("expected retained-data limit, got %v", err)
	}
	got, err := extractEnglishTermsWithLimits(archive, gettyArchiveLimits{maxTableBytes: 1 << 20, maxEntries: 2})
	if err != nil || len(got) != 2 {
		t.Fatalf("ordinary archive should remain usable: %v, %v", got, err)
	}
}

func TestTerminalTextEscapesFilesystemControls(t *testing.T) {
	name := "mail\x1b]52;c;payload\a\rspoof\u202e"
	got := safeTerminalText(name)
	if strings.ContainsAny(got, "\x1b\a\r") || strings.ContainsRune(got, '\u202e') {
		t.Fatalf("terminal control remained in display text: %q", got)
	}
	if !strings.Contains(got, `\x1B`) || !strings.Contains(got, `\u202E`) {
		t.Fatalf("control characters should be visible: %q", got)
	}
	item := dirItem{name: name, path: "/root/" + name}
	if item.Title() != got || !strings.Contains(item.Description(), `\x1B`) {
		t.Fatalf("directory picker did not escape the name: %q, %q", item.Title(), item.Description())
	}
}

func TestEmailCopyRefusesAliasOfSourceInsideDestination(t *testing.T) {
	workspace := t.TempDir()
	dest := filepath.Join(workspace, "dest")
	source := filepath.Join(dest, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(source, "mail.eml")
	if err := os.WriteFile(original, []byte("keep this mail"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, filepath.Join(dest, "mail.eml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, err := copyEmailFiles(source, dest); err == nil {
		t.Fatal("copy should reject a destination alias of its own source")
	}
	data, err := os.ReadFile(original)
	if err != nil || string(data) != "keep this mail" {
		t.Fatalf("source mail was truncated: %q, %v", data, err)
	}
}

func TestGettyArchiveRejectsManyMembers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "many.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for i := 0; i < maxGettyArchiveFiles+1; i++ {
		if _, err := writer.Create("empty-" + strings.Repeat("x", i%3) + string(rune('a'+i%26))); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := archivePublished(path); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("expected member-count limit before archive parsing, got %v", err)
	}
	if _, err := extractEnglishTerms(path); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("expected member-count limit before term parsing, got %v", err)
	}
}

func TestEmailCopyDoesNotWriteThroughDestinationHardLink(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "source")
	dest := filepath.Join(workspace, "dest")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mail.eml"), []byte("new mail"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(workspace, "outside.eml")
	if err := os.WriteFile(outside, []byte("keep outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(dest, "mail.eml")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, copied, err := copyEmailFiles(source, dest); err != nil || copied != 1 {
		t.Fatalf("copy should safely replace a hard-link entry: %d, %v", copied, err)
	}
	for path, want := range map[string]string{outside: "keep outside", filepath.Join(dest, "mail.eml"): "new mail"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("unexpected content at %s: %q, %v", path, data, err)
		}
	}
}

func TestGettyArchiveRejectsForgedDirectorySize(t *testing.T) {
	archive := writeTestArchive(t, term("1001", "3001", "alpha", termPreferred), language(aatEnglishLanguageID, "1001"))
	contents, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	end := bytes.LastIndex(contents, []byte("PK\x05\x06"))
	if end < 0 {
		t.Fatal("fixture has no ZIP end record")
	}
	binary.LittleEndian.PutUint32(contents[end+12:end+16], 1)
	if err := os.WriteFile(archive, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := extractEnglishTerms(archive); err == nil || !strings.Contains(err.Error(), "directory bounds") {
		t.Fatalf("forged directory size should be rejected before ZIP parsing: %v", err)
	}
}

func TestScanBackpressuresWhenFirstHashIsSlow(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	window := max(2, runtime.NumCPU()) * 4
	for i := 0; i < window+20; i++ {
		if err := os.WriteFile(filepath.Join(source, fmt.Sprintf("%05d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var started atomic.Int64
	hashFn := func(ctx context.Context, path string, _ hashAlgorithm) (string, error) {
		started.Add(1)
		if filepath.Base(path) == "00000.txt" {
			select {
			case <-release:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		return "hash", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := runScanWithContext(ctx, source, filepath.Join(workspace, "report.csv"), scanOptions{
			HashAlgorithm: hashAlgorithmSHA256,
			hashFileFn:    hashFn,
		})
		finished <- err
	}()
	deadline := time.After(8 * time.Second)
	for started.Load() < int64(window) {
		select {
		case <-deadline:
			t.Fatalf("scan never filled its bounded work window: %d of %d", started.Load(), window)
		case <-time.After(time.Millisecond):
		}
	}
	time.Sleep(50 * time.Millisecond)
	if got := started.Load(); got > int64(window) {
		t.Fatalf("started %d hashes while the first result was blocked; limit is %d", got, window)
	}
	unblock()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("ordinary scan did not finish after the slow hash completed: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("scan remained blocked after the slow hash completed")
	}
}

func TestEmailCopyKeepsSeparateManifestsForRepeatedRuns(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "source")
	dest := filepath.Join(workspace, "dest")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mail.eml"), []byte("mail"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, _, err := copyEmailFiles(source, dest)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := copyEmailFiles(source, dest)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("repeated copies reused one manifest: %s", first)
	}
	for _, path := range []string{first, second} {
		if rows := readCSVRows(t, path); len(rows) != 2 {
			t.Fatalf("manifest %s was lost: %q", path, rows)
		}
	}
}
