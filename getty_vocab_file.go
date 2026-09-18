package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// A vocabulary list exported from the working AAT Tags sheet is the offline
// source. It is the list the cataloguers actually pick from, which makes it a
// better answer to "is this a term we use" than the full thesaurus would be,
// and it needs no network at all.
//
// The file is one term per line, optionally followed by the preferred term to
// use in its place. It is read as CSV rather than split on newlines because a
// handful of AAT terms contain commas, and a spreadsheet export quotes those.
//
// A line with a second column names a variant: a real AAT term that this
// catalogue does not accept, and the term that replaces it. A line without one
// is a term to use as written. Lists made before the second column existed
// therefore load unchanged, and every term in them reads as acceptable — which
// is what they meant when they were written.
//
// Getty Vocabulary data is published under ODC-By 1.0. A list derived from it
// carries the same attribution requirement: credit the Getty Research
// Institute and name the Art & Architecture Thesaurus.

// fileVocabulary answers term lookups from a list held in memory.
type fileVocabulary struct {
	name string
	// terms maps a lowercased term to every entry the list holds for it. AAT
	// distinguishes concepts by case — "acacia" the wood against "Acacia" the
	// genus — so the spellings cannot be collapsed into one.
	terms map[string][]vocabularyTerm
	count int
	// variants is how many of those terms name a preferred term to use in
	// their place. A list with none says nothing about variants — it is not
	// that the list has none, it is that the format cannot record them — and a
	// check against it can only answer "this term is in the list".
	variants int

	// indexOnce guards the word index, which is built on the first suggestion
	// and never at all for a run with nothing to suggest about.
	indexOnce sync.Once
	index     *termWordIndex
}

// spellingsOf lists the entries' spellings, for the places that only care what
// the list says a term looks like.
func spellingsOf(entries []vocabularyTerm) []string {
	spellings := make([]string, len(entries))
	for i, entry := range entries {
		spellings[i] = entry.Term
	}
	return spellings
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

	vocabulary := &fileVocabulary{name: name, terms: map[string][]vocabularyTerm{}}
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

		// The export carries the term in the first column and, for a variant,
		// the preferred term in the second. Later columns, if a future export
		// grows any, are notes rather than terms.
		term := normalizeTagText(record[0]).Cleaned
		if term == "" {
			continue
		}
		preferred := ""
		if len(record) > 1 {
			preferred = normalizeTagText(record[1]).Cleaned
		}
		if preferred == term {
			// A term that is its own replacement is a preferred term written
			// the long way.
			preferred = ""
		}

		key := strings.ToLower(term)
		if containsString(spellingsOf(vocabulary.terms[key]), term) {
			// The working list has at least one term entered twice. A repeated
			// spelling is the same fact stated twice, not a second concept.
			continue
		}
		vocabulary.terms[key] = append(vocabulary.terms[key], vocabularyTerm{Term: term, Preferred: preferred})
		vocabulary.count++
		if preferred != "" {
			vocabulary.variants++
		}
	}

	if vocabulary.count == 0 {
		return nil, fmt.Errorf("no terms found; expected one term per line")
	}
	return vocabulary, nil
}

// uniqueStrings drops repeats while keeping the first of each, which matters
// once variants are answered with their preferred term: several variants of
// one concept collapse to the same suggestion.
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	unique := values[:0]
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
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
//
// A list with no variants in it says so. Two quite different files land on
// this code — a list built from Getty's archive, which knows which terms are
// variants, and a one-column list exported from the cataloguers' own working
// sheet, which cannot — and they enforce different things. Naming that where
// the source is named keeps it from being something a release note has to
// explain.
func (v *fileVocabulary) SourceName() string {
	// Grouped digits, because these lists run to six figures and "176629
	// terms" is a number nobody reads at a glance.
	terms := fmt.Sprintf("%s terms", groupDigits(v.count))
	if v.count == 1 {
		terms = "1 term"
	}
	if v.variants == 0 {
		terms += ", no variant information"
	}
	// The bundled list carries its date in its own name, so bracketing the
	// count as well produced "…(January 2026) (176,629 terms)" on screen.
	if strings.HasSuffix(v.name, ")") {
		return fmt.Sprintf("%s · %s", v.name, terms)
	}
	return fmt.Sprintf("%s (%s)", v.name, terms)
}

// groupDigits writes a count with thousands separators.
func groupDigits(n int) string {
	digits := strconv.Itoa(n)
	if n < 0 {
		digits = digits[1:]
	}

	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	if n < 0 {
		return "-" + grouped.String()
	}
	return grouped.String()
}

// ListName is the file's own name, for lines that repeat per term and would
// drown in the full description.
func (v *fileVocabulary) ListName() string { return v.name }

// SnapshotNote warns that a term list answers for the day it was made.
//
// Getty revises the thesaurus continually. A term this list holds may since
// have been renamed, which makes a "found" answer from here weaker evidence
// than the same answer from the live service.
func (v *fileVocabulary) SnapshotNote() string {
	note := fmt.Sprintf("%s is a fixed copy. Getty revises the thesaurus, so a term found here may since have been renamed; the live check is the current authority.", v.name)
	if v.variants == 0 {
		note += " It also records no variants, so no tag will be reported as one — build a list from a Getty archive, or use the live check, to have variants caught."
	}
	return note
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
	entries, ok := v.terms[strings.ToLower(cleaned)]
	if !ok {
		// Getty writes qualified terms as "counters (furniture)". The
		// relational archive these lists come from stores the term as bare
		// "counters" and keeps the qualifier in data the export does not
		// carry, so an exact comparison rejects a tag that is perfectly
		// correct. Falling back to the base term avoids that false negative;
		// the match records that the qualifier went unchecked, because it did.
		if base, qualifier := splitQualifier(cleaned); qualifier != "" {
			if baseEntries, baseOK := v.terms[strings.ToLower(base)]; baseOK {
				entry := pickEntry(baseEntries, base)
				return gettyTermMatch{
					Term:             term,
					Found:            true,
					PreferredLabel:   entry.replacement(),
					Variant:          entry.Preferred != "",
					QualifierIgnored: true,
				}, nil
			}
		}
		return gettyTermMatch{Term: term, Found: false}, nil
	}

	entry := pickEntry(entries, cleaned)
	return gettyTermMatch{
		Term:           term,
		Found:          true,
		PreferredLabel: entry.replacement(),
		Variant:        entry.Preferred != "",
	}, nil
}

// pickEntry chooses which of the spellings held under one lowercased key the
// lookup is about: the exact spelling when the list holds it, and the first
// listed otherwise, which is what lets the report say the sheet disagrees with
// the vocabulary about case.
func pickEntry(entries []vocabularyTerm, cleaned string) vocabularyTerm {
	for _, entry := range entries {
		if entry.Term == cleaned {
			return entry
		}
	}
	return entries[0]
}

// replacement is what the sheet should say: the preferred term for a variant,
// and the list's own spelling otherwise.
func (t vocabularyTerm) replacement() string {
	if t.Preferred != "" {
		return t.Preferred
	}
	return t.Term
}

// Suggest proposes the terms closest to the one that was not found.
//
// Closeness is scored rather than filtered, because the two ways a tag goes
// wrong need different answers. A tag that is simply the wrong term shares a
// whole word with the right one — "coastal landscapes" for "landscapes" — and
// a tag that is the right term misspelled shares no word at all: "portait
// photography" has to reach "portrait photography" across a missing letter,
// and "ortriats" has to reach "portraits" across two.
//
// So a candidate scores 3 for each query word it holds outright, 2 for each it
// holds one slip away, 1 for each it holds two slips away, and the
// best-scoring few are offered. Ranking on the score rather than on length is
// what stops a short unrelated term outranking the term someone actually
// meant.
//
// The comparison is made word by word against an index rather than term by
// term against the list, because the same words repeat endlessly across a
// thesaurus: 176,629 terms are built from a fraction of that many distinct
// words, and "photographs" is worth comparing once rather than once per term
// containing it.
func (v *fileVocabulary) Suggest(_ context.Context, term string) ([]string, error) {
	cleaned := strings.ToLower(normalizeTagText(term).Cleaned)
	if cleaned == "" {
		return nil, nil
	}

	words := significantWords(cleaned)
	if len(words) == 0 {
		return nil, nil
	}

	index := v.wordIndex()
	scores := map[int32]int{}
	best := map[int32]int{}
	for _, word := range words {
		clear(best)
		index.scoreWord(word, best)
		for entry, score := range best {
			scores[entry] += score
		}
	}

	type scored struct {
		term  string
		score int
	}
	var candidates []scored
	seen := map[string]int{}
	for entry, score := range scores {
		// A variant is never suggested: proposing it would replace one unusable
		// term with another. Its preferred term is offered in its place, which
		// is why several entries can collapse to one suggestion — and why the
		// best score among them is the one that counts.
		spelling := index.entries[entry].replacement()
		if strings.ToLower(spelling) == cleaned {
			continue
		}
		if at, taken := seen[spelling]; taken {
			if score > candidates[at].score {
				candidates[at].score = score
			}
			continue
		}
		seen[spelling] = len(candidates)
		candidates = append(candidates, scored{term: spelling, score: score})
	}

	// Map iteration is unordered, so the order is settled here: closest first,
	// then the term nearest in length to what was typed — a term of about the
	// same size is usually the one meant — and alphabetically to break ties.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		iGap, jGap := lengthGap(candidates[i].term, cleaned), lengthGap(candidates[j].term, cleaned)
		if iGap != jGap {
			return iGap < jGap
		}
		return candidates[i].term < candidates[j].term
	})
	if len(candidates) > maxSuggestions {
		candidates = candidates[:maxSuggestions]
	}

	suggestions := make([]string, len(candidates))
	for i, candidate := range candidates {
		suggestions[i] = candidate.term
	}
	return suggestions, nil
}

// termWordIndex is the list turned inside out: every distinct word, and which
// terms hold it.
//
// It is built once, on the first suggestion asked for, and never for a run
// that has no unknown terms to suggest about. Building it over the bundled
// list costs a fraction of a second; not having it costs that much on every
// unknown term instead.
type termWordIndex struct {
	entries []vocabularyTerm
	words   []indexedWord
	byWord  map[string]int32
}

// indexedWord carries what the cheap tests need alongside the word itself, so
// most candidates are rejected without touching the edit distance at all.
type indexedWord struct {
	text string
	// letters has a bit set per distinct ASCII letter or digit in the word.
	// Each distinct character one word has and the other lacks costs at least
	// one slip, so comparing the two masks bounds the distance from below —
	// cheaply, and before any table is built.
	letters uint32
	terms   []int32
}

func (v *fileVocabulary) wordIndex() *termWordIndex {
	v.indexOnce.Do(func() {
		index := &termWordIndex{byWord: map[string]int32{}}

		// A stable order over the entries, because map iteration is not one and
		// suggestions have to come out the same way twice.
		keys := make([]string, 0, len(v.terms))
		for key := range v.terms {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			for _, entry := range v.terms[key] {
				at := int32(len(index.entries))
				index.entries = append(index.entries, entry)

				// Both spellings are indexed: the word that matches may be the
				// variant's, while the term offered is its preferred form.
				index.add(entry.Term, at)
				if entry.Preferred != "" {
					index.add(entry.Preferred, at)
				}
			}
		}
		v.index = index
	})
	return v.index
}

func (i *termWordIndex) add(term string, entry int32) {
	for _, word := range wordsOf(strings.ToLower(term)) {
		at, known := i.byWord[word]
		if !known {
			at = int32(len(i.words))
			i.byWord[word] = at
			i.words = append(i.words, indexedWord{text: word, letters: letterMask(word)})
		}
		if held := i.words[at].terms; len(held) > 0 && held[len(held)-1] == entry {
			// The same word twice in one term says nothing new.
			continue
		}
		i.words[at].terms = append(i.words[at].terms, entry)
	}
}

// scoreWord records, for one word of the term that was not found, the best
// score every term can claim against it.
func (i *termWordIndex) scoreWord(word string, best map[int32]int) {
	if at, known := i.byWord[word]; known {
		for _, entry := range i.words[at].terms {
			best[entry] = 3
		}
	}

	allowed := allowedSlips(len(word), len(word))
	if allowed == 0 {
		return
	}
	mask := letterMask(word)

	for index := range i.words {
		candidate := &i.words[index]
		if candidate.text == word {
			continue
		}
		slack := allowedSlips(len(word), len(candidate.text))
		if slack == 0 || abs(len(candidate.text)-len(word)) > slack {
			continue
		}
		// Each distinct character one word holds and the other does not costs a
		// slip, so this rejects nearly everything before the distance is built.
		if bits.OnesCount32(mask&^candidate.letters) > slack ||
			bits.OnesCount32(candidate.letters&^mask) > slack {
			continue
		}

		slips := editDistanceWithin(candidate.text, word, slack)
		if slips == 0 {
			continue
		}
		score := 3 - slips
		for _, entry := range candidate.terms {
			if score > best[entry] {
				best[entry] = score
			}
		}
	}
}

// letterMask sets one bit per distinct letter or digit. Anything else — the
// hyphen in "black-and-white", an accent — falls into a single shared bit,
// which weakens the test for those words without making it wrong.
func letterMask(word string) uint32 {
	var mask uint32
	for _, r := range word {
		switch {
		case r >= 'a' && r <= 'z':
			mask |= 1 << uint(r-'a')
		case r >= '0' && r <= '9':
			mask |= 1 << uint(26+(r-'0')%5)
		default:
			mask |= 1 << 31
		}
	}
	return mask
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// scoreTerm rates one candidate term against the words of the term that was
// not found, on the same 3/2/1 scale the index uses: 3 for a word held as
// written, 2 for one slip away, 1 for two.
func scoreTerm(candidate string, words []string) int {
	fields := wordsOf(strings.ToLower(candidate))
	score := 0
	for _, word := range words {
		best := 0
		for _, field := range fields {
			if field == word {
				best = 3
				break
			}
			if slips := nearWord(field, word); slips > 0 && 3-slips > best {
				best = 3 - slips
			}
		}
		score += best
	}
	return score
}

// nearWord reports how badly two words differ when they are the same word
// typed badly, and 0 when they are not.
func nearWord(candidate, word string) int {
	allowed := allowedSlips(len(word), len(candidate))
	if allowed == 0 {
		return 0
	}
	return editDistanceWithin(candidate, word, allowed)
}

// perfectScore is what a candidate scores when it holds every word of what was
// typed, exactly as typed — the mark of the term someone meant rather than a
// term that merely shares a word with it.
func perfectScore(typed string) int {
	return 3 * len(significantWords(strings.ToLower(normalizeTagText(typed).Cleaned)))
}

// rankSuggestions orders candidates by closeness to what was typed and keeps
// the best few. It also reports the best score, so a caller can tell "these are
// the term meant" from "these merely share a word".
func rankSuggestions(candidates []string, typed string) ([]string, int) {
	cleaned := strings.ToLower(normalizeTagText(typed).Cleaned)
	words := significantWords(cleaned)

	type scored struct {
		term  string
		score int
	}
	ranked := make([]scored, 0, len(candidates))
	seen := map[string]bool{cleaned: true}
	for _, candidate := range candidates {
		key := strings.ToLower(candidate)
		if candidate == "" || seen[key] {
			continue
		}
		seen[key] = true
		ranked = append(ranked, scored{term: candidate, score: scoreTerm(candidate, words)})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		iGap, jGap := lengthGap(strings.ToLower(ranked[i].term), cleaned), lengthGap(strings.ToLower(ranked[j].term), cleaned)
		if iGap != jGap {
			return iGap < jGap
		}
		return ranked[i].term < ranked[j].term
	})

	best := 0
	if len(ranked) > 0 {
		best = ranked[0].score
	}
	if len(ranked) > maxSuggestions {
		ranked = ranked[:maxSuggestions]
	}
	out := make([]string, len(ranked))
	for i, candidate := range ranked {
		out[i] = candidate.term
	}
	return out, best
}

// allowedSlips is how wrong a word of this length may be and still be taken as
// the same word. The shorter of the two lengths decides, so a long candidate
// cannot be matched generously against a short query.
func allowedSlips(wordLen, candidateLen int) int {
	shortest := wordLen
	if candidateLen < shortest {
		shortest = candidateLen
	}
	switch {
	case shortest < 5:
		return 0
	case shortest < 8:
		return 1
	default:
		return 2
	}
}

// editDistanceWithin returns the number of slips between two words when that
// is at most max, and 0 when it is more or when they are identical. A slip is
// a letter inserted, dropped, mistyped, or swapped with its neighbour — the
// swap counted once rather than as the two substitutions it literally is,
// because it is the most common typing error there is.
//
// Only the cells within max of the diagonal are computed. Anything further out
// is already too far apart to matter, which keeps this cheap enough to run
// against every term in a list of 176,629.
func editDistanceWithin(a, b string, max int) int {
	if a == b {
		return 0
	}
	if len(a)-len(b) > max || len(b)-len(a) > max {
		return 0
	}

	const unreachable = 1 << 20
	// Three rows: the two before this one, and the one being filled. The row
	// before last is what lets a swap cost one.
	twoBack := make([]int, len(b)+1)
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		low, high := i-max, i+max
		if low < 1 {
			low = 1
		}
		if high > len(b) {
			high = len(b)
		}
		// Cells outside the band are never computed, so they must not be read
		// as though they held a real distance.
		for j := 0; j <= len(b); j++ {
			current[j] = unreachable
		}
		current[0] = i

		best := unreachable
		for j := low; j <= high; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			distance := min3(
				previous[j]+1,      // a letter dropped
				current[j-1]+1,     // a letter inserted
				previous[j-1]+cost, // the same letter, or one mistyped
			)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				if swapped := twoBack[j-2] + 1; swapped < distance {
					distance = swapped
				}
			}
			current[j] = distance
			if distance < best {
				best = distance
			}
		}
		if best > max {
			// Every cell in this row is already too far apart, and no later row
			// brings it back.
			return 0
		}

		twoBack, previous, current = previous, current, twoBack
	}

	if previous[len(b)] > max {
		return 0
	}
	return previous[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// lengthGap is how far a candidate is from the typed term in length, used to
// separate candidates that score the same.
func lengthGap(candidate, typed string) int {
	gap := len(candidate) - len(typed)
	if gap < 0 {
		return -gap
	}
	return gap
}

// significantWords drops the short connecting words that would match almost
// everything in a vocabulary of thirty thousand terms.
func significantWords(term string) []string {
	var words []string
	for _, word := range wordsOf(term) {
		if len(word) >= 4 {
			words = append(words, word)
		}
	}
	return words
}

// wordsOf splits a term into the words a comparison is made on, dropping the
// punctuation Getty writes around them.
func wordsOf(term string) []string {
	return strings.FieldsFunc(term, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
