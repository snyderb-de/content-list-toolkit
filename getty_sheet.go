package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// The CONTENTdm workflow exports the Access [Main] table to a spreadsheet,
// which is then saved as a tab-delimited file for upload. The check runs on
// the spreadsheet, before the tab-delimited conversion that the invisible
// characters break.
//
// Both spreadsheet and delimited text are accepted, because the export lands
// in whichever the person doing the work reached for.

// tagsColumnHeader is the column in the Access [Main] table that holds the
// Getty AAT terms.
const tagsColumnHeader = "Tags"

type sheetFormat string

const (
	sheetFormatXLSX sheetFormat = "xlsx"
	sheetFormatCSV  sheetFormat = "csv"
	sheetFormatTSV  sheetFormat = "tab-delimited"
)

// errNoTagsColumn is returned when the file has no Tags column at all, which
// usually means a different export was selected by mistake.
var errNoTagsColumn = errors.New("no Tags column")

// TagRow is one row of the source with its verdict.
type TagRow struct {
	// Number is the row as a spreadsheet shows it, counting the header as 1,
	// so a report line can be found by scrolling to it.
	Number int            `json:"number"`
	Result TagCheckResult `json:"result"`
}

// TagChange is one Tags cell as it was and as it ended up.
type TagChange struct {
	Row    int    `json:"row"`
	Before string `json:"before"`
	After  string `json:"after"`
	// ByHand separates a correction someone made from one the cleaning made,
	// because they answer different questions later: the first is a cataloguing
	// decision, the second is this tool doing its job.
	ByHand bool `json:"byHand"`
}

// TagSheetReport is the outcome of checking one exported sheet.
type TagSheetReport struct {
	Path         string      `json:"path"`
	Format       sheetFormat `json:"format"`
	SheetName    string      `json:"sheetName,omitempty"`
	ColumnLetter string      `json:"columnLetter,omitempty"`
	ColumnIndex  int         `json:"columnIndex"`

	TotalRows  int `json:"totalRows"`
	EmptyCells int `json:"emptyCells"`

	// VocabularySource names where terms were verified, or is empty when the
	// run checked structure only. A report has to say which, because a term
	// absent from an exported list means something weaker than one Getty
	// itself does not hold.
	VocabularySource string `json:"vocabularySource,omitempty"`

	// SourcePath is the file the corrections came from, when this report
	// describes a cleaned copy rather than the original export.
	SourcePath string `json:"sourcePath,omitempty"`

	// Changes records what was altered on the way to this file. A report on a
	// corrected sheet finds nothing wrong, which is the point — but "nothing
	// wrong" with no account of what changed is not a record of anything.
	Changes []TagChange `json:"changes,omitempty"`

	// Rows carries only the rows with something to say. A clean sheet of ten
	// thousand records produces an empty list rather than ten thousand
	// confirmations.
	Rows []TagRow `json:"rows"`
}

// BlockingRows counts rows whose Tags cell would have broken the upload.
func (r TagSheetReport) BlockingRows() int {
	count := 0
	for _, row := range r.Rows {
		if row.Result.OriginalBlocksUpload() {
			count++
		}
	}
	return count
}

// ReviewRows counts rows that still want a person after cleaning.
func (r TagSheetReport) ReviewRows() int {
	count := 0
	for _, row := range r.Rows {
		if row.Result.NeedsReview() {
			count++
		}
	}
	return count
}

// RepairedRows counts rows the cleaned output fixes outright.
func (r TagSheetReport) RepairedRows() int {
	count := 0
	for _, row := range r.Rows {
		if row.Result.Changed() {
			count++
		}
	}
	return count
}

// Summary is the one-line verdict for the top of a report.
func (r TagSheetReport) Summary() string {
	if len(r.Rows) == 0 {
		return fmt.Sprintf("%s: all %s clean", filepath.Base(r.Path),
			pluralize(r.TotalRows, "row", "rows"))
	}
	return fmt.Sprintf("%s: %s checked, %d would have failed to upload, %d repaired, %d need review",
		filepath.Base(r.Path), pluralize(r.TotalRows, "row", "rows"),
		r.BlockingRows(), r.RepairedRows(), r.ReviewRows())
}

// detectSheetFormat picks a reader from the file extension. The tab-delimited
// case matters because it is the format the upload itself consumes, so a file
// that already failed can be checked directly.
func detectSheetFormat(path string) (sheetFormat, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx", ".xlsm":
		return sheetFormatXLSX, nil
	case ".csv":
		return sheetFormatCSV, nil
	case ".tsv", ".tab", ".txt":
		return sheetFormatTSV, nil
	case ".xls":
		return "", fmt.Errorf("%s is the older Excel format; re-save it as .xlsx", filepath.Base(path))
	case ".accdb", ".mdb":
		return "", fmt.Errorf("%s is an Access database; export the Main table to .xlsx or .csv first", filepath.Base(path))
	default:
		return "", fmt.Errorf("unsupported file type %q; expected .xlsx, .csv, or a tab-delimited export", filepath.Ext(path))
	}
}

// checkTagSheet reads an exported sheet and checks the structure of every Tags
// cell in it, without consulting a vocabulary.
func checkTagSheet(path string) (TagSheetReport, error) {
	return checkTagSheetWithVocabulary(context.Background(), path, nil)
}

// checkTagSheetWithVocabulary additionally verifies every term against the
// given vocabulary. A nil vocabulary checks structure only.
func checkTagSheetWithVocabulary(ctx context.Context, path string, vocabulary gettyVocabulary) (TagSheetReport, error) {
	format, err := detectSheetFormat(path)
	if err != nil {
		return TagSheetReport{}, err
	}

	report := TagSheetReport{Path: path, Format: format}
	if vocabulary != nil {
		report.VocabularySource = vocabulary.SourceName()
	}

	var rows [][]string
	switch format {
	case sheetFormatXLSX:
		rows, report.SheetName, err = readXLSXRows(path)
	case sheetFormatCSV:
		rows, err = readDelimitedRows(path, ',')
	case sheetFormatTSV:
		rows, err = readDelimitedRows(path, '\t')
	}
	if err != nil {
		return TagSheetReport{}, err
	}
	if len(rows) == 0 {
		return TagSheetReport{}, fmt.Errorf("%s is empty", filepath.Base(path))
	}

	column, err := findTagsColumn(rows[0])
	if err != nil {
		return TagSheetReport{}, err
	}
	report.ColumnIndex = column
	if letter, lerr := excelize.ColumnNumberToName(column + 1); lerr == nil {
		report.ColumnLetter = letter
	}

	for i, row := range rows[1:] {
		number := i + 2 // header is row 1
		if column >= len(row) {
			// Trailing empty cells are omitted by both readers rather than
			// padded, so a short row means the cell is blank.
			report.EmptyCells++
			report.TotalRows++
			continue
		}
		value := row[column]
		report.TotalRows++
		if strings.TrimSpace(value) == "" {
			report.EmptyCells++
			continue
		}
		result := checkTagCell(value)
		verifyTags(ctx, vocabulary, &result)
		if !result.OK() {
			report.Rows = append(report.Rows, TagRow{Number: number, Result: result})
		}
	}

	return report, nil
}

// findTagsColumn locates the Tags column by header name.
//
// Headers are normalized before comparison because the header cell is as
// likely to carry a pasted no-break space as the data is, and a column that
// cannot be found reads to the user as a broken tool rather than a stray
// invisible character.
func findTagsColumn(header []string) (int, error) {
	want := strings.ToLower(tagsColumnHeader)
	for i, cell := range header {
		if strings.ToLower(normalizeTagText(cell).Cleaned) == want {
			return i, nil
		}
	}

	available := make([]string, 0, len(header))
	for _, cell := range header {
		if cleaned := normalizeTagText(cell).Cleaned; cleaned != "" {
			available = append(available, cleaned)
		}
	}
	if len(available) == 0 {
		return 0, fmt.Errorf("%w: the first row has no column names", errNoTagsColumn)
	}
	return 0, fmt.Errorf("%w: expected a column named %q, found %s",
		errNoTagsColumn, tagsColumnHeader, strings.Join(available, ", "))
}

// readXLSXRows reads the first worksheet, which is where an Access export of
// the Main table lands.
func readXLSXRows(path string) ([][]string, string, error) {
	file, err := excelize.OpenFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer file.Close()

	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return nil, "", fmt.Errorf("%s has no worksheets", filepath.Base(path))
	}
	name := sheets[0]

	rows, err := file.GetRows(name)
	if err != nil {
		return nil, "", fmt.Errorf("read sheet %q: %w", name, err)
	}
	return rows, name, nil
}

// readDelimitedRows reads comma or tab separated text.
//
// FieldsPerRecord is disabled because an export with ragged trailing columns
// is still worth checking; refusing the whole file over a short row would send
// the user back to Excel for no reason. LazyQuotes is on for the same reason:
// a stray quote inside a title should not stop the check.
func readDelimitedRows(path string, delimiter rune) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer file.Close()

	reader := csv.NewReader(newBOMStrippingReader(file))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	return rows, nil
}

// newBOMStrippingReader drops a leading UTF-8 byte order mark.
//
// Excel writes one when saving CSV as UTF-8, and left in place it becomes part
// of the first header name, so the Tags column goes missing when it happens to
// be first.
func newBOMStrippingReader(r io.Reader) io.Reader {
	bom := []byte{0xEF, 0xBB, 0xBF}
	buffered := make([]byte, len(bom))

	n, err := io.ReadFull(r, buffered)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		// Let the real read surface the error in context.
		return io.MultiReader(strings.NewReader(string(buffered[:n])), r)
	}
	if n == len(bom) && string(buffered) == string(bom) {
		return r
	}
	return io.MultiReader(strings.NewReader(string(buffered[:n])), r)
}
