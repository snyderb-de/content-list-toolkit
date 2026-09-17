package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func imagesFolder(t *testing.T, names ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestBuildAccessFileListWritesBothColumns(t *testing.T) {
	dir := imagesFolder(t, "9200-B35-002_0061d.pdf", "9200-B35-002_0062d.pdf")

	app := newApp("")
	result, err := app.BuildAccessFileList(dir)
	if err != nil {
		t.Fatalf("BuildAccessFileList: %v", err)
	}
	if result.Files != 2 {
		t.Fatalf("Files = %d, want 2", result.Files)
	}

	rows, _, err := readXLSXRows(result.Path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if rows[0][0] != itemNumberHeader || rows[0][1] != fileNameHeader {
		t.Fatalf("headers = %q, want the Access column names", rows[0])
	}
	// Item Number is the name without its extension; File Name keeps it.
	if rows[1][0] != "9200-B35-002_0061d" {
		t.Fatalf("Item Number = %q", rows[1][0])
	}
	if rows[1][1] != "9200-B35-002_0061d.pdf" {
		t.Fatalf("File Name = %q", rows[1][1])
	}
}

// Plain string ordering puts _0100 before _0061 once a batch passes ninety-nine
// files. A list ordered differently from the images it describes misfiles every
// row after the first mismatch.
func TestAccessFileListOrdersNumbersByValue(t *testing.T) {
	dir := imagesFolder(t,
		"item_0100d.pdf", "item_0061d.pdf", "item_0002d.pdf", "item_0999d.pdf", "item_1000d.pdf",
	)

	app := newApp("")
	result, err := app.BuildAccessFileList(dir)
	if err != nil {
		t.Fatalf("BuildAccessFileList: %v", err)
	}
	rows, _, err := readXLSXRows(result.Path)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"item_0002d.pdf", "item_0061d.pdf", "item_0100d.pdf", "item_0999d.pdf", "item_1000d.pdf"}
	for i, expected := range want {
		if got := rows[i+1][1]; got != expected {
			t.Fatalf("row %d = %q, want %q", i+1, got, expected)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{a: "item_2.pdf", b: "item_10.pdf", want: true},
		{a: "item_10.pdf", b: "item_2.pdf", want: false},
		{a: "item_0061.pdf", b: "item_0100.pdf", want: true},
		// Leading zeros must not decide the order.
		{a: "item_061.pdf", b: "item_0061.pdf", want: false},
		{a: "a.pdf", b: "b.pdf", want: true},
		{a: "A.pdf", b: "b.pdf", want: true},
		{a: "same.pdf", b: "same.pdf", want: false},
	} {
		if got := naturalLess(tc.a, tc.b); got != tc.want {
			t.Fatalf("naturalLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// A hidden file or a .DS_Store pasted into a collection sheet is somebody's
// afternoon. They are dropped, and counted so the screen can say so.
func TestAccessFileListSkipsNoiseAndCountsIt(t *testing.T) {
	dir := imagesFolder(t, "item_0001d.pdf", ".DS_Store", ".hidden.pdf", "Thumbs.db")
	if err := os.MkdirAll(filepath.Join(dir, "subfolder"), 0o755); err != nil {
		t.Fatal(err)
	}

	app := newApp("")
	result, err := app.BuildAccessFileList(dir)
	if err != nil {
		t.Fatalf("BuildAccessFileList: %v", err)
	}
	if result.Files != 1 {
		t.Fatalf("Files = %d, want 1", result.Files)
	}
	if result.Skipped != 4 {
		t.Fatalf("Skipped = %d, want 4 (.DS_Store, .hidden.pdf, Thumbs.db, subfolder)", result.Skipped)
	}
}

// Only the chosen folder. The process puts one collection's images in one
// folder, and descending would mix in a neighbouring batch.
func TestAccessFileListDoesNotDescend(t *testing.T) {
	dir := imagesFolder(t, "item_0001d.pdf")
	nested := filepath.Join(dir, "other-batch")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "item_9999d.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := newApp("")
	result, err := app.BuildAccessFileList(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 {
		t.Fatalf("Files = %d, want 1 — the nested batch must not be listed", result.Files)
	}
}

// Excel reads 0061 as the number 61 and drops the zeros. An item number is an
// identifier, so every cell is written as text.
func TestAccessFileListKeepsLeadingZeros(t *testing.T) {
	dir := imagesFolder(t, "0061d.pdf")

	app := newApp("")
	result, err := app.BuildAccessFileList(dir)
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := readXLSXRows(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if rows[1][0] != "0061d" {
		t.Fatalf("Item Number = %q, want the leading zeros kept", rows[1][0])
	}
}

func TestBuildAccessFileListReportsUsableErrors(t *testing.T) {
	app := newApp("")

	if _, err := app.BuildAccessFileList(""); err == nil ||
		!strings.Contains(err.Error(), "choose the folder") {
		t.Fatalf("expected a prompt to choose a folder, got %v", err)
	}
	if _, err := app.BuildAccessFileList(filepath.Join(t.TempDir(), "absent")); err == nil ||
		!strings.Contains(err.Error(), "not a folder") {
		t.Fatalf("expected a missing-folder error, got %v", err)
	}
	empty := filepath.Join(t.TempDir(), "Images")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.BuildAccessFileList(empty); err == nil ||
		!strings.Contains(err.Error(), "no files") {
		t.Fatalf("expected an empty-folder error, got %v", err)
	}
}

func TestAccessFileListPathSitsBesideTheFolder(t *testing.T) {
	got := accessFileListPath(filepath.Join("X", "Collection", "Images"))
	want := filepath.Join("X", "Collection", "Images-access-file-list.xlsx")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
