package main

import (
	"compress/gzip"
	"embed"
	"fmt"
	"sync"
)

// The application ships a copy of the Getty AAT so that offline checking works
// out of the box, with no download and no network at all.
//
// It is stored gzipped and inflated on first use: 4.3 MB of terms compress to
// 1.1 MB, and a check that never consults a vocabulary should not pay to build
// an index of 176,629 terms it will not read.
//
// It is the whole English thesaurus, preferred terms and variants alike, and
// each variant carries the preferred term that replaces it — which is what
// lets an offline check tell a tag to correct from a tag that is already
// right.
//
// This copy is Getty's January 2026 archive, which Getty has frozen. It can
// therefore accept a term Getty has since renamed, so it reports itself as a
// snapshot wherever its answers are shown. See assets/ATTRIBUTION.md.

//go:embed assets/aat-terms-2026-01.csv.gz
var embeddedVocabularyData embed.FS

const (
	embeddedVocabularyPath = "assets/aat-terms-2026-01.csv.gz"
	// embeddedVocabularyDate is the publication date of the archive this was
	// extracted from, and is shown to the user rather than kept in a comment.
	embeddedVocabularyDate = "January 2026"
)

var (
	embeddedVocabularyOnce sync.Once
	embeddedVocabularyList *fileVocabulary
	embeddedVocabularyErr  error
)

// builtinVocabulary returns the bundled term list, building the index the
// first time it is asked for and reusing it afterwards.
func builtinVocabulary() (*fileVocabulary, error) {
	embeddedVocabularyOnce.Do(func() {
		file, err := embeddedVocabularyData.Open(embeddedVocabularyPath)
		if err != nil {
			embeddedVocabularyErr = fmt.Errorf("the bundled term list is missing from this build: %w", err)
			return
		}
		defer file.Close()

		reader, err := gzip.NewReader(file)
		if err != nil {
			embeddedVocabularyErr = fmt.Errorf("the bundled term list could not be read: %w", err)
			return
		}
		defer reader.Close()

		list, err := readVocabulary(reader, fmt.Sprintf("built-in Getty AAT list (%s)", embeddedVocabularyDate))
		if err != nil {
			embeddedVocabularyErr = fmt.Errorf("the bundled term list could not be read: %w", err)
			return
		}
		embeddedVocabularyList = list
	})
	return embeddedVocabularyList, embeddedVocabularyErr
}
