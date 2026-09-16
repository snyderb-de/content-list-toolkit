package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
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

// SnapshotNote warns that a term list answers for the day it was made.
//
// Getty revises the thesaurus continually. A term this list holds may since
// have been renamed, which makes a "found" answer from here weaker evidence
// than the same answer from the live service.
func (v *fileVocabulary) SnapshotNote() string {
	return fmt.Sprintf("%s is a fixed copy. Getty revises the thesaurus, so a term found here may since have been renamed; the live check is the current authority.", v.name)
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
		// Getty writes qualified terms as "counters (furniture)". The
		// relational archive these lists come from stores the term as bare
		// "counters" and keeps the qualifier in data the export does not
		// carry, so an exact comparison rejects a tag that is perfectly
		// correct. Falling back to the base term avoids that false negative;
		// the match records that the qualifier went unchecked, because it did.
		if base, qualifier := splitQualifier(cleaned); qualifier != "" {
			if baseSpellings, baseOK := v.terms[strings.ToLower(base)]; baseOK {
				return gettyTermMatch{
					Term:             term,
					Found:            true,
					PreferredLabel:   baseSpellings[0],
					QualifierIgnored: true,
				}, nil
			}
		}
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

// Suggest proposes terms from the list that share a significant word with the
// one that was not found.
//
// Matching on whole words rather than raw substrings keeps the suggestions
// recognisable: searching "landscapes" should offer "coastal landscapes", not
// every term that happens to contain the letters.
func (v *fileVocabulary) Suggest(_ context.Context, term string) ([]string, error) {
	cleaned := strings.ToLower(normalizeTagText(term).Cleaned)
	if cleaned == "" {
		return nil, nil
	}

	words := significantWords(cleaned)
	if len(words) == 0 {
		return nil, nil
	}

	var suggestions []string
	for _, spellings := range v.terms {
		for _, spelling := range spellings {
			candidate := strings.ToLower(spelling)
			if candidate == cleaned {
				continue
			}
			if !containsAnyWord(candidate, words) {
				continue
			}
			suggestions = append(suggestions, spelling)
			break
		}
	}

	// Map iteration is unordered, so sort for a stable list, then take the
	// shortest few: a shorter term sharing the word is usually the closer one.
	sort.Slice(suggestions, func(i, j int) bool {
		if len(suggestions[i]) != len(suggestions[j]) {
			return len(suggestions[i]) < len(suggestions[j])
		}
		return suggestions[i] < suggestions[j]
	})
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}
	return suggestions, nil
}

// significantWords drops the short connecting words that would match almost
// everything in a vocabulary of thirty thousand terms.
func significantWords(term string) []string {
	var words []string
	for _, word := range strings.FieldsFunc(term, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(word) >= 4 {
			words = append(words, word)
		}
	}
	return words
}

func containsAnyWord(candidate string, words []string) bool {
	fields := strings.FieldsFunc(candidate, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, field := range fields {
		for _, word := range words {
			if field == word {
				return true
			}
		}
	}
	return false
}
