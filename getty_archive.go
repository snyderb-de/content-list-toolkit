package main

import (
	"archive/zip"
	"bufio"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Getty publishes its vocabulary as bulk relational archives. This module
// turns one of those archives into the term list the offline check reads.
//
// The app does not fetch the archive. It used to, and the reason it no longer
// does is worth recording: aatdownloads.getty.edu answers on plain HTTP and
// has nothing listening on 443, so that download could not be encrypted, and
// this app makes no unencrypted connections. Getty's live endpoint is HTTPS
// and is reached normally.
//
// So the archive arrives the way any other file does — someone fetches it and
// points the app at it — and everything here reads from disk.
//
// One thing about the data is worth stating wherever a list from it is shown:
// the archives are the datasets Getty froze in January 2026 and has said it
// will not refresh, so the live endpoint is the only current source. A list
// built here is a dated snapshot, and the date is written into the filename so
// it cannot be mistaken for current.

const (
	// AAT tags terms with a language, and English is three of them. 70051 is
	// English, confirmed against a known concept: "aerial photographs", term
	// 1000128222 of subject 300128222, the same subject the live endpoint
	// returns for that term.
	//
	// 70052 and 70053 are American and British English. They were missed at
	// first, and the cost was a real false negative: "place settings" — a tag
	// taken from a live sheet and confirmed valid at vocab.getty.edu — is
	// American English, so the list did not hold it and the check called an
	// accepted term invalid. Between them the two dialects carry about 8,000
	// English terms.
	//
	// The archive has no language table to look these up in, so they were
	// identified from the terms themselves: 70052 holds "corn knives" and
	// "curry combs", 70053 "full-size mould" and "horn centre".
	aatEnglishLanguageID = "70051"
	// aatAmericanEnglishLanguageID and aatBritishEnglishLanguageID are the
	// dialects. A term in one of them is an English term.
	aatAmericanEnglishLanguageID = "70052"
	aatBritishEnglishLanguageID  = "70053"

	// termPreferred marks a concept's preferred label in TERM; "V" marks a
	// variant. Both are real AAT terms and both are kept, but which is which is
	// recorded: this catalogue accepts the preferred term only, so a variant
	// has to be recognised as a variant and answered with the term to use
	// instead.
	termPreferred = "P"
)

// GettyImportResult tells the screen what was written and how old it is.
type GettyImportResult struct {
	Path string `json:"path"`
	// Terms is how many distinct English terms were written.
	Terms int `json:"terms"`
	// Archive is the file it came from, and Published when Getty last changed
	// it, so a stale list can be recognised as stale.
	Archive   string `json:"archive"`
	Published string `json:"published"`
	Elapsed   string `json:"elapsed"`
}

// ImportGettyVocabulary reads a relational archive already on disk and writes
// the English term list beside it.
//
// The list is written next to the archive rather than into a folder chosen
// separately: the archive is already somewhere deliberate, and one dialog is
// one fewer thing to get wrong.
func (a *App) ImportGettyVocabulary(archivePath string) (GettyImportResult, error) {
	started := time.Now()

	published, err := archivePublished(archivePath)
	if err != nil {
		return GettyImportResult{}, err
	}

	terms, err := extractEnglishTerms(archivePath)
	if err != nil {
		return GettyImportResult{}, err
	}

	// The date goes in the filename because these archives are frozen. A list
	// called "AAT terms.csv" tells nobody how old it is.
	name := fmt.Sprintf("AAT terms %s (English).csv", published.Format("2006-01-02"))
	destination := filepath.Join(filepath.Dir(archivePath), name)
	if err := writeTermList(destination, terms); err != nil {
		return GettyImportResult{}, err
	}

	return GettyImportResult{
		Path:      destination,
		Terms:     len(terms),
		Archive:   filepath.Base(archivePath),
		Published: published.Format("2 January 2006"),
		Elapsed:   time.Since(started).Round(time.Second).String(),
	}, nil
}

const (
	maxGettyArchiveFiles   = 512
	maxGettyDirectoryBytes = 1 << 20
)

// archive/zip builds its member list before returning from OpenReader. Check
// the end-of-directory record first so a ZIP with millions of tiny members
// cannot exhaust memory while that list is built. Getty's relational archive
// is well below these limits and does not need ZIP64.
func checkGettyArchiveDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	const endLen = 22
	const maxComment = 65535
	if info.Size() < endLen {
		return fmt.Errorf("Getty archive has no ZIP directory")
	}
	tailLen := min(info.Size(), int64(endLen+maxComment))
	tail := make([]byte, tailLen)
	if _, err := file.ReadAt(tail, info.Size()-tailLen); err != nil {
		return err
	}
	for i := len(tail) - endLen; i >= 0; i-- {
		if string(tail[i:i+4]) != "PK\x05\x06" {
			continue
		}
		commentLen := int(binary.LittleEndian.Uint16(tail[i+20 : i+22]))
		if i+endLen+commentLen > len(tail) {
			continue
		}
		entries := binary.LittleEndian.Uint16(tail[i+10 : i+12])
		directoryBytes := binary.LittleEndian.Uint32(tail[i+12 : i+16])
		directoryOffset := binary.LittleEndian.Uint32(tail[i+16 : i+20])
		if entries == 0xffff || directoryBytes == 0xffffffff || directoryOffset == 0xffffffff {
			return fmt.Errorf("Getty archive ZIP64 directory is not supported")
		}
		if entries > maxGettyArchiveFiles || directoryBytes > maxGettyDirectoryBytes {
			return fmt.Errorf("Getty archive has too many ZIP members or directory bytes")
		}
		endOffset := uint64(info.Size()-tailLen) + uint64(i)
		if uint64(directoryOffset)+uint64(directoryBytes) != endOffset {
			return fmt.Errorf("Getty archive has invalid ZIP directory bounds")
		}
		return nil
	}
	return fmt.Errorf("Getty archive has no ZIP directory")
}

// archivePublished dates the archive from the newest file inside it, which is
// when Getty built it.
//
// The file's own modification time is not used: copying an archive between
// machines or unpacking and repacking it rewrites that, and a list labelled
// with the day it was copied claims to be newer than its data.
func archivePublished(archivePath string) (time.Time, error) {
	if err := checkGettyArchiveDirectory(archivePath); err != nil {
		return time.Time{}, err
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s could not be opened as an archive: %w", filepath.Base(archivePath), err)
	}
	defer reader.Close()
	if len(reader.File) > maxGettyArchiveFiles {
		return time.Time{}, fmt.Errorf("Getty archive has too many ZIP members")
	}

	var newest time.Time
	for _, file := range reader.File {
		if modified := file.Modified; modified.After(newest) {
			newest = modified
		}
	}
	if newest.IsZero() {
		return time.Time{}, fmt.Errorf("%s carries no dates; it may not be a Getty vocabulary archive", filepath.Base(archivePath))
	}
	return newest, nil
}

// vocabularyTerm is one line of the written list: an AAT term, and — when the
// term is a variant — the preferred term of the same concept, which is what a
// cataloguer has to use in its place.
type vocabularyTerm struct {
	Term string
	// Preferred is empty for a preferred term, so the list says "this is the
	// one" by saying nothing.
	Preferred string
}

// extractEnglishTerms reads the two tables that together say which terms are
// English. TERM holds the text, whether it is preferred, and which concept it
// belongs to; LANGUAGE_RELS says what language each term is in. Without the
// second, a list built from the first would carry every language Getty holds.
type gettyArchiveLimits struct {
	maxTableBytes    int64
	maxEntries       int
	maxRetainedBytes int64
}

type gettyRetainedBudget struct {
	remaining int64
}

func (b *gettyRetainedBudget) reserve(bytes int64) error {
	if bytes > b.remaining {
		return fmt.Errorf("Getty archive exceeds the retained data limit")
	}
	b.remaining -= bytes
	return nil
}

var defaultGettyArchiveLimits = gettyArchiveLimits{
	maxTableBytes:    1 << 30, // AAT's relational tables can be large.
	maxEntries:       1_000_000,
	maxRetainedBytes: 256 << 20,
}

func extractEnglishTerms(archivePath string) ([]vocabularyTerm, error) {
	return extractEnglishTermsWithLimits(archivePath, defaultGettyArchiveLimits)
}

func extractEnglishTermsWithLimits(archivePath string, limits gettyArchiveLimits) ([]vocabularyTerm, error) {
	if limits.maxRetainedBytes <= 0 {
		limits.maxRetainedBytes = defaultGettyArchiveLimits.maxRetainedBytes
	}
	budget := &gettyRetainedBudget{remaining: limits.maxRetainedBytes}
	if err := checkGettyArchiveDirectory(archivePath); err != nil {
		return nil, err
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("the downloaded archive could not be opened: %w", err)
	}
	defer reader.Close()
	if len(reader.File) > maxGettyArchiveFiles {
		return nil, fmt.Errorf("Getty archive has too many ZIP members")
	}

	if err := checkRelationalArchive(reader, archivePath); err != nil {
		return nil, err
	}

	english, err := readEnglishTermIDs(reader, limits, budget)
	if err != nil {
		return nil, err
	}
	if len(english) == 0 {
		return nil, fmt.Errorf("the archive contained no English terms; its format may have changed")
	}
	return readTerms(reader, english, limits, budget)
}

// readEnglishTermIDs maps each English term to which English it is, because
// the dialects are not quite equal: where a concept has a preferred term in
// plain English and another in a dialect, plain English is the one to name as
// the term to use.
// checkRelationalArchive rejects an archive that is not the one this reads,
// naming what it looks like instead.
//
// Getty publishes several archives on the same page, and the XML one has a
// name one character different from the relational one — aat_xml_0126.zip
// against aat_rel_0126.zip. Picking the wrong one is the easiest mistake here,
// and "the archive did not contain TERM.out" does not tell anyone which file
// to go back for.
func checkRelationalArchive(reader *zip.ReadCloser, archivePath string) error {
	for _, file := range reader.File {
		if strings.EqualFold(filepath.Base(file.Name), "TERM.out") {
			return nil
		}
	}

	name := filepath.Base(archivePath)
	for _, file := range reader.File {
		if strings.EqualFold(filepath.Ext(file.Name), ".xml") {
			return fmt.Errorf("%s holds XML files, so it is Getty's XML archive. This reads the relational one — the file named aat_rel_<mmyy>.zip on the same download page", name)
		}
	}
	return fmt.Errorf("%s is not a Getty vocabulary archive: it holds no TERM.out table. The file to use is named aat_rel_<mmyy>.zip", name)
}

func readEnglishTermIDs(reader *zip.ReadCloser, limits gettyArchiveLimits, budget *gettyRetainedBudget) (map[string]string, error) {
	file, err := openArchiveFile(reader, "LANGUAGE_RELS.out", limits.maxTableBytes)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	english := map[string]string{}
	scanner, tableReader := newTableScanner(file, limits.maxTableBytes)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 4 {
			continue
		}
		switch fields[0] {
		case aatEnglishLanguageID, aatAmericanEnglishLanguageID, aatBritishEnglishLanguageID:
			if _, exists := english[fields[3]]; !exists {
				if len(english) >= limits.maxEntries {
					return nil, fmt.Errorf("Getty archive has too many English term IDs")
				}
				if err := budget.reserve(int64(len(fields[3]) + len(fields[0]) + 128)); err != nil {
					return nil, err
				}
			}
			// Clone substrings so a short ID does not retain its whole table line.
			english[strings.Clone(fields[3])] = strings.Clone(fields[0])
		}
	}
	return english, tableScanErr(scanner, tableReader, "LANGUAGE_RELS.out", limits.maxTableBytes)
}

// readTerms walks TERM twice: once to learn each concept's English preferred
// label, once to write the list. Two passes rather than one because the table
// is not ordered by concept, so a variant can be read long before the
// preferred term it should point at.
func readTerms(reader *zip.ReadCloser, english map[string]string, limits gettyArchiveLimits, budget *gettyRetainedBudget) ([]vocabularyTerm, error) {
	preferred, err := readPreferredLabels(reader, english, limits, budget)
	if err != nil {
		return nil, err
	}

	file, err := openArchiveFile(reader, "TERM.out", limits.maxTableBytes)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Terms are deduplicated without regard to case, so one spelling can arrive
	// twice: the same word is the preferred term of one concept and a variant
	// of another. Being preferred anywhere is what matters — the tag is then a
	// term to use as written — so a preferred row replaces a variant already
	// kept, and the index is what makes that reachable.
	at := map[string]int{}
	var terms []vocabularyTerm
	scanner, tableReader := newTableScanner(file, limits.maxTableBytes)
	for scanner.Scan() {
		row, ok := parseTermRow(scanner.Text(), english)
		if !ok {
			continue
		}

		entry := vocabularyTerm{Term: row.term}
		if !row.preferred {
			// A variant carries the term to use instead. When the concept has
			// no English preferred label — a few do not — the variant is all
			// the list can offer, so it stands on its own rather than being
			// dropped.
			if label := preferred[row.subject]; label != "" && label != row.term {
				entry.Preferred = label
			}
		}

		key := strings.ToLower(row.term)
		if index, taken := at[key]; taken {
			if entry.Preferred == "" {
				terms[index] = entry
			}
			continue
		}
		if len(terms) >= limits.maxEntries {
			return nil, fmt.Errorf("Getty archive has too many distinct terms")
		}
		if err := budget.reserve(int64(len(key) + len(entry.Term) + len(entry.Preferred) + 128)); err != nil {
			return nil, err
		}
		at[key] = len(terms)
		terms = append(terms, entry)
	}
	return terms, tableScanErr(scanner, tableReader, "TERM.out", limits.maxTableBytes)
}

// readPreferredLabels maps each concept to its English preferred term.
//
// A concept can have one per dialect — "moulds" in British English beside
// "molds" in plain English — and the list can only name one as the term to
// use. Plain English wins, since that is the label the live endpoint returns.
func readPreferredLabels(reader *zip.ReadCloser, english map[string]string, limits gettyArchiveLimits, budget *gettyRetainedBudget) (map[string]string, error) {
	file, err := openArchiveFile(reader, "TERM.out", limits.maxTableBytes)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	labels := map[string]string{}
	plain := map[string]bool{}
	scanner, tableReader := newTableScanner(file, limits.maxTableBytes)
	for scanner.Scan() {
		row, ok := parseTermRow(scanner.Text(), english)
		if !ok || !row.preferred {
			continue
		}
		if plain[row.subject] {
			// Already have the plain-English label; nothing beats it.
			continue
		}
		isPlain := row.language == aatEnglishLanguageID
		if _, taken := labels[row.subject]; taken && !isPlain {
			// A dialect label only fills a gap, and the gap is filled.
			continue
		}
		if _, exists := labels[row.subject]; !exists {
			if len(labels) >= limits.maxEntries {
				return nil, fmt.Errorf("Getty archive has too many preferred labels")
			}
			if err := budget.reserve(int64(len(row.subject) + len(row.term) + 128)); err != nil {
				return nil, err
			}
		} else if err := budget.reserve(int64(len(row.term))); err != nil {
			return nil, err
		}
		labels[row.subject] = row.term
		plain[row.subject] = isPlain
	}
	return labels, tableScanErr(scanner, tableReader, "TERM.out", limits.maxTableBytes)
}

// termRow is the part of a TERM line this export uses.
type termRow struct {
	term      string
	subject   string
	language  string
	preferred bool
}

// parseTermRow reads one TERM line, keeping English terms only. Column 7 is
// the P/V flag, 9 the concept, 10 the text, and 11 the term id.
func parseTermRow(line string, english map[string]string) (termRow, bool) {
	fields := strings.Split(line, "\t")
	if len(fields) < 12 {
		return termRow{}, false
	}
	language, isEnglish := english[fields[11]]
	if !isEnglish {
		return termRow{}, false
	}
	term := strings.Clone(normalizeTagText(fields[10]).Cleaned)
	if term == "" {
		return termRow{}, false
	}
	return termRow{
		term:      term,
		subject:   strings.Clone(fields[9]),
		language:  language,
		preferred: fields[7] == termPreferred,
	}, true
}

func openArchiveFile(reader *zip.ReadCloser, name string, maxBytes int64) (io.ReadCloser, error) {
	for _, file := range reader.File {
		if strings.EqualFold(filepath.Base(file.Name), name) {
			if file.UncompressedSize64 > uint64(maxBytes) {
				return nil, fmt.Errorf("%s exceeds the Getty archive table size limit", name)
			}
			return file.Open()
		}
	}
	return nil, fmt.Errorf("the archive did not contain %s; its format may have changed", name)
}

// newTableScanner also bounds actual decompressed bytes, independent of ZIP
// metadata. The extra byte lets tableScanErr distinguish an exact-limit table.
func newTableScanner(r io.Reader, maxBytes int64) (*bufio.Scanner, *io.LimitedReader) {
	limited := &io.LimitedReader{R: r, N: maxBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return scanner, limited
}

func tableScanErr(scanner *bufio.Scanner, limited *io.LimitedReader, name string, maxBytes int64) error {
	if err := scanner.Err(); err != nil {
		return err
	}
	if limited.N == 0 {
		return fmt.Errorf("%s exceeds the Getty archive table size limit of %d bytes", name, maxBytes)
	}
	return nil
}

// writeTermList writes the list the offline check reads: the term, then the
// preferred term to use in its place when the term is a variant.
//
// A preferred term is written alone, exactly as the earlier one-column lists
// were written, so a list made by this version and one made before it are the
// same file for every term that needs no correction, and an older list still
// loads.
func writeTermList(path string, terms []vocabularyTerm) error {
	for _, term := range terms {
		if spreadsheetFormulaRisk(term.Term) || spreadsheetFormulaRisk(term.Preferred) {
			return fmt.Errorf("archive contains a term that could run as a spreadsheet formula")
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not write the term list: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	for _, term := range terms {
		record := []string{term.Term}
		if term.Preferred != "" {
			record = append(record, term.Preferred)
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("could not write the term list: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("could not write the term list: %w", err)
	}
	return nil
}
