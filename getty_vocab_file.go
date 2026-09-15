package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// A vocabulary list exported from the working AAT Tags sheet is the offline
// source. It is the list the cataloguers actually pick from, which makes it a
// better answer to "is this a term we use" than the full thesaurus would be,
// and it needs no network at all.
//
// The file is one term per line. It is read as CSV rather than split on
// newlines because a handful of AAT terms contain commas, and a spreadsheet
// export quotes those.
//
// Getty Vocabulary data is published under ODC-By 1.0. A list derived from it
// carries the same attribution requirement: credit the Getty Research
// Institute and name the Art & Architecture Thesaurus.

// fileVocabulary answers term lookups from a list held in memory.
type fileVocabulary struct {
	name string
	// terms maps a lowercased term to every spelling the list holds for it.
	// AAT distinguishes concepts by case — "acacia" the wood against "Acacia"
	// the genus — so the spellings cannot be collapsed into one.
	terms map[string][]string
	count int
}

// loadVocabularyFile reads a term list from disk.
func loadVocabularyFile(path string) (*fileVocabulary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open vocabulary %s: %w", filepath.Base(path), err)
	}
	defer file.Close()

	vocabulary, err := readVocabulary(file, filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("read vocabulary %s: %w", filepath.Base(path), err)
	}
	return vocabulary, nil
}

// readVocabulary builds the index from any reader, so tests need no fixture
// file on disk.
func readVocabulary(r io.Reader, name string) (*fileVocabulary, error) {
	reader := csv.NewReader(newBOMStrippingReader(r))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	vocabulary := &fileVocabulary{name: name, terms: map[string][]string{}}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) == 0 {
			continue
		}

		// The export carries the term in the first column. Later columns, if a
		// future export grows any, are notes rather than terms.
		term := normalizeTagText(record[0]).Cleaned
		if term == "" {
			continue
		}

		key := strings.ToLower(term)
		if containsString(vocabulary.terms[key], term) {
			// The working list has at least one term entered twice. A repeated
			// spelling is the same fact stated twice, not a second concept.
			continue
		}
		vocabulary.terms[key] = append(vocabulary.terms[key], term)
		vocabulary.count++
	}

	if vocabulary.count == 0 {
		return nil, fmt.Errorf("no terms found; expected one term per line")
	}
	return vocabulary, nil
}

func containsString(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if candidate == needle {
			return true
		}
	}
	return false
}

// SourceName identifies the list in reports, so a result can be traced to the
// vocabulary version that produced it.
func (v *fileVocabulary) SourceName() string {
	return fmt.Sprintf("%s (%s)", v.name, pluralize(v.count, "term", "terms"))
}

// Count reports how many distinct spellings the list holds.
func (v *fileVocabulary) Count() int { return v.count }

// Lookup answers from memory, so it never fails and never blocks.
//
// Matching is case-insensitive, because a cataloguer typing "Landscapes" means
// the term that the list spells "landscapes". Where the list holds several
// spellings that differ only by case, the exact spelling is preferred and the
// first listed is reported otherwise, which is what lets the report say the
// sheet disagrees with the vocabulary about case.
func (v *fileVocabulary) Lookup(_ context.Context, term string) (gettyTermMatch, error) {
	cleaned := normalizeTagText(term).Cleaned
	spellings, ok := v.terms[strings.ToLower(cleaned)]
	if !ok {
		return gettyTermMatch{Term: term, Found: false}, nil
	}

	preferred := spellings[0]
	if containsString(spellings, cleaned) {
		preferred = cleaned
	}
	return gettyTermMatch{
		Term:           term,
		Found:          true,
		PreferredLabel: preferred,
	}, nil
}
