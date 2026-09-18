package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleVocabulary = `aerial photographs
landscapes
city plans
acacia
Acacia
"balustrades, railings and their components"
press cameras
press cameras
`

func loadSampleVocabulary(t *testing.T) *fileVocabulary {
	t.Helper()
	vocabulary, err := readVocabulary(strings.NewReader(sampleVocabulary), "sample")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}
	return vocabulary
}

func TestFileVocabularyFindsTerms(t *testing.T) {
	vocabulary := loadSampleVocabulary(t)
	match, err := vocabulary.Lookup(context.Background(), "aerial photographs")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !match.Found {
		t.Fatal("expected the term to be found")
	}
	if !match.ExactLabel() {
		t.Fatalf("expected an exact label match, got %q", match.PreferredLabel)
	}
}

func TestFileVocabularyReportsAbsentTermsWithoutError(t *testing.T) {
	vocabulary := loadSampleVocabulary(t)
	match, err := vocabulary.Lookup(context.Background(), "not a real term")
	if err != nil {
		t.Fatalf("an absent term is an answer, not an error: %v", err)
	}
	if match.Found {
		t.Fatal("expected the term to be absent")
	}
}

// A cataloguer typing "Landscapes" means the term the list spells
// "landscapes", but the report should still be able to say they disagree.
func TestFileVocabularyMatchesCaseInsensitivelyAndKeepsTheListSpelling(t *testing.T) {
	vocabulary := loadSampleVocabulary(t)
	match, err := vocabulary.Lookup(context.Background(), "Landscapes")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !match.Found {
		t.Fatal("expected a case-insensitive match")
	}
	if match.PreferredLabel != "landscapes" {
		t.Fatalf("PreferredLabel = %q, want the list spelling %q", match.PreferredLabel, "landscapes")
	}
	if match.ExactLabel() {
		t.Fatal("the sheet and the list disagree about case, so this is not an exact match")
	}
}

// AAT distinguishes concepts by case: acacia the wood against Acacia the
// genus. Both spellings have to survive loading.
func TestFileVocabularyKeepsCaseVariantsApart(t *testing.T) {
	vocabulary := loadSampleVocabulary(t)
	for _, spelling := range []string{"acacia", "Acacia"} {
		match, err := vocabulary.Lookup(context.Background(), spelling)
		if err != nil {
			t.Fatalf("%q: %v", spelling, err)
		}
		if !match.ExactLabel() {
			t.Fatalf("%q should match its own spelling exactly, got %q", spelling, match.PreferredLabel)
		}
	}
}

// Some AAT terms contain commas, and a spreadsheet export quotes them. Reading
// the file as CSV rather than splitting on newlines is what keeps them whole.
func TestFileVocabularyKeepsQuotedCommasIntact(t *testing.T) {
	vocabulary := loadSampleVocabulary(t)
	match, err := vocabulary.Lookup(context.Background(), "balustrades, railings and their components")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !match.Found {
		t.Fatal("a term containing a comma should survive loading intact")
	}
}

// The working list has at least one term entered twice.
func TestFileVocabularyCountsARepeatedSpellingOnce(t *testing.T) {
	vocabulary := loadSampleVocabulary(t)
	// aerial photographs, landscapes, city plans, acacia, Acacia,
	// balustrades..., press cameras
	if vocabulary.Count() != 7 {
		t.Fatalf("Count() = %d, want 7 distinct spellings", vocabulary.Count())
	}
}

func TestFileVocabularyRejectsAnEmptyList(t *testing.T) {
	if _, err := readVocabulary(strings.NewReader("\n\n"), "empty"); err == nil {
		t.Fatal("expected an error for a list with no terms")
	}
}

func TestLoadVocabularyFileReadsFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AAT Tags.csv")
	if err := os.WriteFile(path, []byte(sampleVocabulary), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	vocabulary, err := loadVocabularyFile(path)
	if err != nil {
		t.Fatalf("loadVocabularyFile: %v", err)
	}
	if !strings.Contains(vocabulary.SourceName(), "AAT Tags.csv") {
		t.Fatalf("SourceName = %q, want it to name the file", vocabulary.SourceName())
	}
}

func TestLoadVocabularyFileReportsAMissingFile(t *testing.T) {
	if _, err := loadVocabularyFile(filepath.Join(t.TempDir(), "absent.csv")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

// The cache and the file source have to compose, since the same handful of
// terms repeats across a whole sheet.
func TestFileVocabularyWorksBehindTheCache(t *testing.T) {
	cache := newCachedVocabulary(loadSampleVocabulary(t))
	for i := 0; i < 4; i++ {
		match, err := cache.Lookup(context.Background(), "city plans")
		if err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
		if !match.Found {
			t.Fatalf("lookup %d: expected the term to be found", i)
		}
	}
	if cache.Lookups() != 1 {
		t.Fatalf("Lookups() = %d, want 1", cache.Lookups())
	}
}

// Getty's January 2026 archive lists "landscapes" as the preferred label of
// two AAT subjects. The live endpoint holds no such label today, because Getty
// disambiguated the term in between. A list built from that archive therefore
// reports the tag as valid when Getty no longer agrees — a false positive,
// which is the harder failure to notice, since a wrong "not found" gets
// investigated and a wrong "found" is believed.
func TestFileVocabularyWarnsThatItIsASnapshot(t *testing.T) {
	vocabulary := loadSampleVocabulary(t)
	note := vocabulary.SnapshotNote()

	if note == "" {
		t.Fatal("a source that cannot be current has to say so")
	}
	if !strings.Contains(note, "renamed") || !strings.Contains(note, "live") {
		t.Fatalf("the caution should explain the risk and name the authority, got %q", note)
	}
	if !strings.Contains(note, "sample") {
		t.Fatalf("the caution should name the list it is about, got %q", note)
	}
}

// Wrapping a dated list in a cache must not hide that it is dated.
func TestCachePassesTheSnapshotCautionThrough(t *testing.T) {
	cached := newCachedVocabulary(loadSampleVocabulary(t))
	if snapshotNoteFor(cached) == "" {
		t.Fatal("the caution was lost behind the cache")
	}
}

// A live source is current by definition and must not carry the caution.
func TestLiveSourceCarriesNoSnapshotCaution(t *testing.T) {
	if note := snapshotNoteFor(newSPARQLVocabulary(nil, "http://example.invalid")); note != "" {
		t.Fatalf("the live source should not warn about staleness, got %q", note)
	}
	if note := snapshotNoteFor(newCachedVocabulary(newSPARQLVocabulary(nil, "http://example.invalid"))); note != "" {
		t.Fatalf("a cached live source should not warn either, got %q", note)
	}
}

// Getty writes qualified terms as "counters (furniture)". The relational
// archive these lists come from stores the term as bare "counters" and keeps
// the qualifier in data the export does not carry, so comparing exactly
// rejects a tag that is perfectly correct — a false negative on real data,
// found by checking a real export against the live endpoint.
func TestFileVocabularyMatchesAQualifiedTermAgainstABareEntry(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader("counters\naerial photographs\n"), "archive list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	match, err := vocabulary.Lookup(context.Background(), "counters (furniture)")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !match.Found {
		t.Fatal("a qualified term should match the bare entry the archive holds")
	}
	if !match.QualifierIgnored {
		t.Fatal("the match must record that the bracketed part went unchecked")
	}
}

// The fallback confirms the term, not the qualifier. It must not be presented
// as a full match, or a nonsense qualifier would pass silently.
func TestQualifierFallbackDoesNotClaimAnExactMatch(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader("counters\n"), "archive list")
	if err != nil {
		t.Fatal(err)
	}
	match, err := vocabulary.Lookup(context.Background(), "counters (not a real qualifier)")
	if err != nil {
		t.Fatal(err)
	}
	if match.ExactLabel() {
		t.Fatal("a qualifier that was never checked cannot be an exact match")
	}
	if !match.QualifierIgnored {
		t.Fatal("expected the qualifier to be marked unchecked")
	}
}

// A term with no bracketed part must not go near the fallback.
func TestQualifierFallbackLeavesPlainTermsAlone(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader("counters\n"), "archive list")
	if err != nil {
		t.Fatal(err)
	}
	match, err := vocabulary.Lookup(context.Background(), "invented term")
	if err != nil {
		t.Fatal(err)
	}
	if match.Found || match.QualifierIgnored {
		t.Fatalf("an absent plain term must stay absent: %+v", match)
	}
}

func TestSplitQualifier(t *testing.T) {
	for _, tc := range []struct{ in, base, qualifier string }{
		{in: "counters (furniture)", base: "counters", qualifier: "furniture"},
		{in: "cafeterias (eating and drinking spaces)", base: "cafeterias", qualifier: "eating and drinking spaces"},
		{in: "counters", base: "counters", qualifier: ""},
		{in: "counters (furniture", base: "counters (furniture", qualifier: ""},
		{in: "(furniture)", base: "(furniture)", qualifier: ""},
		{in: "counters ()", base: "counters ()", qualifier: ""},
	} {
		base, qualifier := splitQualifier(tc.in)
		if base != tc.base || qualifier != tc.qualifier {
			t.Fatalf("splitQualifier(%q) = (%q, %q), want (%q, %q)", tc.in, base, qualifier, tc.base, tc.qualifier)
		}
	}
}

// The bundled list is built from that archive, so the real case has to work.
func TestBuiltinListMatchesQualifiedTermsFromRealData(t *testing.T) {
	vocabulary, err := builtinVocabulary()
	if err != nil {
		t.Fatal(err)
	}
	match, err := vocabulary.Lookup(context.Background(), "counters (furniture)")
	if err != nil {
		t.Fatal(err)
	}
	if !match.Found {
		t.Fatal("a real tag from a real export must not be reported as absent")
	}
	if !match.QualifierIgnored {
		t.Fatal("the bundled list holds the bare term, so the qualifier went unchecked")
	}
}

// A variant in the list is found, but answered with the term that replaces it.
func TestFileVocabularyMarksVariants(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader(
		"photographs\nphotos,photographs\ncity plans\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	match, err := vocabulary.Lookup(context.Background(), "Photos")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !match.Found || !match.Variant {
		t.Fatalf("expected a found variant, got %+v", match)
	}
	if match.PreferredLabel != "photographs" {
		t.Fatalf("PreferredLabel = %q, want the replacement term", match.PreferredLabel)
	}
	if match.ExactLabel() {
		t.Fatal("a variant is never the preferred label")
	}
}

// A term repeating its own spelling in the second column is preferred, not a
// variant of itself.
func TestFileVocabularyTreatsASelfReferenceAsPreferred(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader(
		"photographs,photographs\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	match, err := vocabulary.Lookup(context.Background(), "photographs")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !match.Found || match.Variant {
		t.Fatalf("expected a preferred term, got %+v", match)
	}
}

// Suggesting a variant would replace one unusable term with another, so the
// preferred term is offered in its place — once, however many variants of it
// the list holds.
func TestFileVocabularySuggestsPreferredTermsRatherThanVariants(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader(
		"photographs\nlarge photographs,photographs\naerial photographs,photographs\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	suggestions, err := vocabulary.Suggest(context.Background(), "small photographs")
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(suggestions) != 1 || suggestions[0] != "photographs" {
		t.Fatalf("expected only the preferred term, got %v", suggestions)
	}
}

// A tag that is the right term with a letter missing shares no whole word with
// it, so a suggester that only matches whole words offers everything except
// the term meant. This is the case reported from a real sheet: "portait
// photography" for "portrait photography".
func TestFileVocabularySuggestsTheTermMeantAcrossATypo(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader(
		"portrait photography\nphotography\nJPEG photography\ndigital photography\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	suggestions, err := vocabulary.Suggest(context.Background(), "portait photography")
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(suggestions) == 0 || suggestions[0] != "portrait photography" {
		t.Fatalf("expected the misspelled term first, got %v", suggestions)
	}
}

// Scoring has to beat length: the shortest term sharing a word is not the
// closest one, which is how "JPEG" came to be offered for a portrait tag.
func TestFileVocabularyRanksClosenessOverBrevity(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader(
		"JPEG\nphoto zines\nportrait photography\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	suggestions, err := vocabulary.Suggest(context.Background(), "portrait photografy")
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(suggestions) == 0 || suggestions[0] != "portrait photography" {
		t.Fatalf("expected the closest term first, got %v", suggestions)
	}
}

// The commonest typing slip of all is a swapped pair, so it counts as one.
func TestFileVocabularySuggestsAcrossASwappedPair(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader(
		"portraits\ncity plans\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	suggestions, err := vocabulary.Suggest(context.Background(), "portriats")
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(suggestions) == 0 || suggestions[0] != "portraits" {
		t.Fatalf("expected the term meant, got %v", suggestions)
	}
}

// A suggestion that is not the term meant is worse than none: it invites a
// wrong tag to be accepted with one click. Two letters apart is a different
// word.
func TestFileVocabularyDoesNotSuggestADistantWord(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader(
		"landmarks\ncity plans\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	suggestions, err := vocabulary.Suggest(context.Background(), "landscapes")
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("expected nothing close enough to offer, got %v", suggestions)
	}
}

func TestEditDistanceWithin(t *testing.T) {
	for _, c := range []struct {
		a, b string
		max  int
		want int
	}{
		{"portrait", "portrait", 2, 0}, // the same word is no slips
		{"portrait", "portait", 1, 1},  // a letter dropped
		{"portrait", "portrsit", 1, 1}, // a letter mistyped
		{"portrait", "portraits", 1, 1},
		{"portraits", "portriats", 1, 1}, // neighbours swapped, counted once
		{"portraits", "protraits", 1, 1}, // swapped nearer the front
		{"portraits", "ortriats", 2, 2},  // a dropped letter and a swap
		{"portraits", "ortriats", 1, 0},  // …which is out of reach at one
		{"portrait", "porait", 1, 0},     // two letters dropped
		{"portrait", "traitpor", 2, 0},   // same letters, different word
		{"landscapes", "landmarks", 2, 0},
	} {
		if got := editDistanceWithin(c.a, c.b, c.max); got != c.want {
			t.Errorf("editDistanceWithin(%q, %q, %d) = %d, want %d", c.a, c.b, c.max, got, c.want)
		}
	}
}

// The allowance scales, or a nine-letter word gets the same latitude as a
// five-letter one.
func TestAllowedSlipsScalesWithLength(t *testing.T) {
	for _, c := range []struct {
		word, candidate string
		want            int
	}{
		{"bows", "bowl", 0},
		{"plans", "plants", 1},
		{"portraits", "portriats", 2},
		{"portraits", "bows", 0}, // the shorter length decides
	} {
		if got := allowedSlips(len(c.word), len(c.candidate)); got != c.want {
			t.Errorf("allowedSlips(%d, %d) = %d, want %d", len(c.word), len(c.candidate), got, c.want)
		}
	}
}

// Two quite different files land on this code, and they enforce different
// things. A list that cannot record variants has to say so where it is named,
// or "nothing was flagged" reads as "nothing is wrong".
func TestFileVocabularyNamesAListWithNoVariantInformation(t *testing.T) {
	plain, err := readVocabulary(strings.NewReader("photographs\nlandscapes\n"), "AAT Tags export.csv")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}
	if !strings.Contains(plain.SourceName(), "no variant information") {
		t.Fatalf("SourceName should say the list carries none, got %q", plain.SourceName())
	}
	if !strings.Contains(plain.SnapshotNote(), "no variants") {
		t.Fatalf("the caution should explain the consequence, got %q", plain.SnapshotNote())
	}

	// A list built from a Getty archive knows, so it says nothing extra.
	fromArchive, err := readVocabulary(strings.NewReader("photographs\nphotos,photographs\n"), "AAT terms.csv")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}
	if strings.Contains(fromArchive.SourceName(), "no variant information") {
		t.Fatalf("a list with variants should not be labelled as lacking them, got %q", fromArchive.SourceName())
	}
	if strings.Contains(fromArchive.SnapshotNote(), "no variants") {
		t.Fatalf("unexpected caution: %q", fromArchive.SnapshotNote())
	}
}

// The full description belongs at the top of a report, not repeated on every
// unknown term.
func TestListNameIsTheShortNameThroughACache(t *testing.T) {
	list, err := readVocabulary(strings.NewReader("photographs\n"), "AAT Tags export.csv")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}
	if got := listNameFor(newCachedVocabulary(list)); got != "AAT Tags export.csv" {
		t.Fatalf("listNameFor = %q, want the bare file name", got)
	}
}

func TestGroupDigits(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{176629, "176,629"},
		{1000000, "1,000,000"},
	} {
		if got := groupDigits(c.n); got != c.want {
			t.Errorf("groupDigits(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// The index is what makes suggesting affordable, and it must not change the
// answers: same list, same query, same order, however many times it is asked.
func TestFileVocabularySuggestionsAreStable(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader(
		"portraits\nself-portraits\nportrait photography\ncity plans\nphotos,photographs\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	first, err := vocabulary.Suggest(context.Background(), "portriats")
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected suggestions")
	}
	for i := 0; i < 5; i++ {
		again, err := vocabulary.Suggest(context.Background(), "portriats")
		if err != nil {
			t.Fatalf("Suggest: %v", err)
		}
		if strings.Join(again, "|") != strings.Join(first, "|") {
			t.Fatalf("suggestions changed between calls: %v then %v", first, again)
		}
	}
}

// Building it costs real work, so it happens on the first suggestion and not
// at all for a run with nothing to suggest about.
func TestFileVocabularyBuildsTheIndexOnceAndOnlyWhenNeeded(t *testing.T) {
	vocabulary, err := readVocabulary(strings.NewReader("portraits\ncity plans\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}

	if _, err := vocabulary.Lookup(context.Background(), "portraits"); err != nil {
		t.Fatal(err)
	}
	if vocabulary.index != nil {
		t.Fatal("a lookup should not build the suggestion index")
	}

	if _, err := vocabulary.Suggest(context.Background(), "portriats"); err != nil {
		t.Fatal(err)
	}
	built := vocabulary.index
	if built == nil {
		t.Fatal("the first suggestion should build the index")
	}
	if _, err := vocabulary.Suggest(context.Background(), "city plns"); err != nil {
		t.Fatal(err)
	}
	if vocabulary.index != built {
		t.Fatal("the index should be built once and reused")
	}
}

// The bundled list's name ends in its date, so bracketing the count as well
// put two bracketed clauses side by side on screen.
func TestSourceNameDoesNotDoubleUpBrackets(t *testing.T) {
	dated, err := readVocabulary(strings.NewReader("photographs\nphotos,photographs\n"),
		"built-in Getty AAT list (January 2026)")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}
	if got := dated.SourceName(); got != "built-in Getty AAT list (January 2026) · 2 terms" {
		t.Fatalf("SourceName = %q", got)
	}

	plain, err := readVocabulary(strings.NewReader("photographs\n"), "AAT Tags export.csv")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}
	if got := plain.SourceName(); got != "AAT Tags export.csv (1 term, no variant information)" {
		t.Fatalf("SourceName = %q", got)
	}
}
