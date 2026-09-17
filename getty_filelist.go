package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// The CONTENTdm process gets filenames into Access by way of a Command Prompt
// session: change directory through the share by tab-completion, run
// `dir /b > filenames.csv`, then paste the result into the File-Name column.
//
// This does the same job from the folder picker, and does it better in one
// respect: Access wants the name twice, once with its extension in
// "File Name (Cdm)" and once without in "Item Number". `dir /b` produces one
// of those, leaving the other to be derived by hand down a column of hundreds
// of rows.
//
// The headers below are taken from a real export so the result can be pasted
// straight in.

const (
	itemNumberHeader = "Item Number"
	fileNameHeader   = "File Name (Cdm)"
)

// AccessFileListResult tells the screen what was written.
type AccessFileListResult struct {
	Path    string `json:"path"`
	Files   int    `json:"files"`
	Skipped int    `json:"skipped"`
	First   string `json:"first,omitempty"`
	Last    string `json:"last,omitempty"`
	Elapsed string `json:"elapsed"`
}

// BuildAccessFileList writes a two-column sheet of the files in one folder,
// ready to paste into the Access Main table.
//
// Only the chosen folder is read, not its subfolders. The process puts the
// images for one collection in a single Images folder, and descending would
// silently mix in files from a neighbouring batch.
func (a *App) BuildAccessFileList(imagesDir string) (AccessFileListResult, error) {
	started := time.Now()

	if strings.TrimSpace(imagesDir) == "" {
		return AccessFileListResult{}, fmt.Errorf("choose the folder holding the images")
	}
	info, err := os.Stat(imagesDir)
	if err != nil || !info.IsDir() {
		return AccessFileListResult{}, fmt.Errorf("%s is not a folder that can be read", filepath.Base(imagesDir))
	}

	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return AccessFileListResult{}, fmt.Errorf("could not read %s: %w", filepath.Base(imagesDir), err)
	}

	names, skipped := collectAccessFileNames(entries)
	if len(names) == 0 {
		return AccessFileListResult{}, fmt.Errorf("%s holds no files to list", filepath.Base(imagesDir))
	}

	path := accessFileListPath(imagesDir)
	if err := writeAccessFileList(path, names); err != nil {
		return AccessFileListResult{}, err
	}

	return AccessFileListResult{
		Path:    path,
		Files:   len(names),
		Skipped: skipped,
		First:   names[0],
		Last:    names[len(names)-1],
		Elapsed: time.Since(started).Round(time.Millisecond).String(),
	}, nil
}

// collectAccessFileNames keeps the files worth listing and counts what it
// dropped, so the screen can say so rather than quietly returning fewer rows
// than the folder appears to hold.
func collectAccessFileNames(entries []os.DirEntry) (names []string, skipped int) {
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			skipped++
			continue
		}
		// .DS_Store and Thumbs.db are never records, and a hidden file pasted
		// into a collection sheet is somebody's afternoon.
		if isAlwaysExcludedFile(name) || isHiddenName(name) {
			skipped++
			continue
		}
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
	return names, skipped
}

// naturalLess orders names the way the numbering implies.
//
// Plain string ordering puts _0100 before _0061 once a batch passes ninety-nine
// files, and a list in the wrong order against images in the right order is
// worse than no list at all: every row is then subtly misfiled.
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		aDigit, bDigit := isASCIIDigit(a[ai]), isASCIIDigit(b[bi])
		if aDigit && bDigit {
			aStart, bStart := ai, bi
			for ai < len(a) && isASCIIDigit(a[ai]) {
				ai++
			}
			for bi < len(b) && isASCIIDigit(b[bi]) {
				bi++
			}
			// Compare by value, so leading zeros do not decide the order.
			aNum, _ := strconv.Atoi(a[aStart:ai])
			bNum, _ := strconv.Atoi(b[bStart:bi])
			if aNum != bNum {
				return aNum < bNum
			}
			continue
		}
		if a[ai] != b[bi] {
			return lowerByte(a[ai]) < lowerByte(b[bi])
		}
		ai++
		bi++
	}
	return len(a)-ai < len(b)-bi
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// accessFileListPath names the sheet after the folder it describes, beside it.
func accessFileListPath(imagesDir string) string {
	base := filepath.Base(filepath.Clean(imagesDir))
	if base == "." || base == string(filepath.Separator) {
		base = "images"
	}
	return filepath.Join(filepath.Dir(filepath.Clean(imagesDir)), base+"-access-file-list.xlsx")
}

func writeAccessFileList(path string, names []string) error {
	file := excelize.NewFile()
	defer file.Close()

	sheet := file.GetSheetList()[0]
	if err := file.SetCellStr(sheet, "A1", itemNumberHeader); err != nil {
		return err
	}
	if err := file.SetCellStr(sheet, "B1", fileNameHeader); err != nil {
		return err
	}

	for i, name := range names {
		row := i + 2
		// SetCellStr throughout: an item number like 0061 is an identifier, and
		// Excel would read it as the number 61 and drop the leading zeros.
		if err := file.SetCellStr(sheet, fmt.Sprintf("A%d", row), strings.TrimSuffix(name, filepath.Ext(name))); err != nil {
			return err
		}
		if err := file.SetCellStr(sheet, fmt.Sprintf("B%d", row), name); err != nil {
			return err
		}
	}

	// Wide enough to read without dragging the column out.
	_ = file.SetColWidth(sheet, "A", "B", 34)

	if err := file.SaveAs(path); err != nil {
		return fmt.Errorf("could not write %s: %w", filepath.Base(path), err)
	}
	return nil
}
