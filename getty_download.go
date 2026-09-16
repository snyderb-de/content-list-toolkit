package main

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Getty serves its bulk vocabulary archives from aatdownloads.getty.edu, which
// is a different host from the vocab.getty.edu SPARQL endpoint the live check
// uses. A network that blocks one does not necessarily block the other, so
// downloading a term list is a genuine second route to verifying terms rather
// than a convenience.
//
// Two things about this route are worth stating plainly wherever it is offered.
//
// The archives are the datasets Getty froze in January 2026 and has said it
// will not refresh; the live endpoint is the only current source. A downloaded
// list is therefore a dated snapshot, and the date is written into the
// filename so it cannot be mistaken for current.
//
// The host answers on plain HTTP and refuses HTTPS, so the download is not
// protected in transit. For a public vocabulary that is a modest risk, but it
// is a real one and not something this code can fix.

const (
	gettyDownloadIndexURL = "http://aatdownloads.getty.edu/"

	// aatEnglishLanguageID is the language identifier English terms carry in
	// LANGUAGE_RELS. Confirmed against a known concept: "aerial photographs",
	// term 1000128222 of subject 300128222, the same subject the live endpoint
	// returns for that term.
	aatEnglishLanguageID = "70051"

	// termPreferred marks a concept's preferred label in TERM; "V" marks a
	// variant. Both are real AAT terms and both are kept.
	termPreferred = "P"

	gettyDownloadTimeout = 10 * time.Minute
)

// relationalArchivePattern finds the relational archive on the index page.
// The filename carries a month, so it is discovered rather than hardcoded:
// should Getty publish a newer archive, this follows it.
var relationalArchivePattern = regexp.MustCompile(`VocabData/aat_rel_[0-9]+\.zip`)

// GettyDownloadResult tells the screen what was written and how old it is.
type GettyDownloadResult struct {
	Path string `json:"path"`
	// Terms is how many distinct English terms were written.
	Terms int `json:"terms"`
	// Archive is the file it came from, and Published when Getty last changed
	// it, so a stale list can be recognised as stale.
	Archive   string `json:"archive"`
	Published string `json:"published"`
	Elapsed   string `json:"elapsed"`
}

// DownloadGettyVocabulary fetches Getty's relational archive and writes an
// English term list that the offline check can read.
func (a *App) DownloadGettyVocabulary(destinationDir string) (GettyDownloadResult, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, gettyDownloadTimeout)
	defer cancel()

	started := time.Now()
	client := &http.Client{Timeout: gettyDownloadTimeout}

	archiveURL, err := findRelationalArchive(ctx, client)
	if err != nil {
		return GettyDownloadResult{}, err
	}

	// The archive is read from a temporary file rather than memory: it is tens
	// of megabytes, and archive/zip needs to seek.
	temp, err := os.CreateTemp("", "aat-archive-*.zip")
	if err != nil {
		return GettyDownloadResult{}, fmt.Errorf("could not create a temporary file: %w", err)
	}
	defer os.Remove(temp.Name())
	defer temp.Close()

	published, err := downloadTo(ctx, client, archiveURL, temp)
	if err != nil {
		return GettyDownloadResult{}, err
	}

	terms, err := extractEnglishTerms(temp.Name())
	if err != nil {
		return GettyDownloadResult{}, err
	}

	// The date goes in the filename because these archives are frozen. A list
	// called "AAT terms.csv" tells nobody how old it is.
	name := fmt.Sprintf("AAT terms %s (English).csv", published.Format("2006-01-02"))
	destination := filepath.Join(destinationDir, name)
	if err := writeTermList(destination, terms); err != nil {
		return GettyDownloadResult{}, err
	}

	return GettyDownloadResult{
		Path:      destination,
		Terms:     len(terms),
		Archive:   filepath.Base(archiveURL),
		Published: published.Format("2 January 2006"),
		Elapsed:   time.Since(started).Round(time.Second).String(),
	}, nil
}

func findRelationalArchive(ctx context.Context, client *http.Client) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, gettyDownloadIndexURL, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("could not reach the Getty download centre: %s", describeProbeFailure(err))
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the Getty download centre answered HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}

	match := relationalArchivePattern.Find(body)
	if match == nil {
		return "", fmt.Errorf("could not find a vocabulary archive on the Getty download centre; the page layout may have changed")
	}
	return gettyDownloadIndexURL + string(match), nil
}

// downloadTo streams the archive to disk and reports when Getty last changed
// it, which is the only honest age for a frozen dataset.
func downloadTo(ctx context.Context, client *http.Client, url string, file *os.File) (time.Time, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return time.Time{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return time.Time{}, fmt.Errorf("could not download %s: %s", filepath.Base(url), describeProbeFailure(err))
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("Getty answered HTTP %d for %s", response.StatusCode, filepath.Base(url))
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		return time.Time{}, fmt.Errorf("could not save the archive: %w", err)
	}

	published := time.Now()
	if header := response.Header.Get("Last-Modified"); header != "" {
		if parsed, perr := http.ParseTime(header); perr == nil {
			published = parsed
		}
	}
	return published, nil
}

// extractEnglishTerms reads the two tables that together say which terms are
// English. TERM holds the text and whether it is preferred; LANGUAGE_RELS says
// what language each term is in. Without the second, a list built from the
// first would carry every language Getty holds.
func extractEnglishTerms(archivePath string) ([]string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("the downloaded archive could not be opened: %w", err)
	}
	defer reader.Close()

	english, err := readEnglishTermIDs(reader)
	if err != nil {
		return nil, err
	}
	if len(english) == 0 {
		return nil, fmt.Errorf("the archive contained no English terms; its format may have changed")
	}
	return readTerms(reader, english)
}

func readEnglishTermIDs(reader *zip.ReadCloser) (map[string]bool, error) {
	file, err := openArchiveFile(reader, "LANGUAGE_RELS.out")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	english := map[string]bool{}
	scanner := newTableScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 4 {
			continue
		}
		if fields[0] == aatEnglishLanguageID {
			english[fields[3]] = true
		}
	}
	return english, scanner.Err()
}

func readTerms(reader *zip.ReadCloser, english map[string]bool) ([]string, error) {
	file, err := openArchiveFile(reader, "TERM.out")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	seen := map[string]bool{}
	var terms []string
	scanner := newTableScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 12 {
			continue
		}
		termID := fields[11]
		if !english[termID] {
			continue
		}
		term := normalizeTagText(fields[10]).Cleaned
		if term == "" || seen[strings.ToLower(term)] {
			continue
		}
		seen[strings.ToLower(term)] = true
		terms = append(terms, term)
	}
	return terms, scanner.Err()
}

func openArchiveFile(reader *zip.ReadCloser, name string) (io.ReadCloser, error) {
	for _, file := range reader.File {
		if strings.EqualFold(filepath.Base(file.Name), name) {
			return file.Open()
		}
	}
	return nil, fmt.Errorf("the archive did not contain %s; its format may have changed", name)
}

// newTableScanner reads these tables line by line with a buffer large enough
// for their longest rows, which exceed bufio's default.
func newTableScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return scanner
}

// writeTermList writes one term per line, the format the offline check reads.
// Terms containing a comma are quoted so the reader keeps them whole.
func writeTermList(path string, terms []string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not write the term list: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, term := range terms {
		if strings.ContainsAny(term, `",`) {
			fmt.Fprintf(writer, "%q\n", term)
			continue
		}
		fmt.Fprintln(writer, term)
	}
	return writer.Flush()
}
