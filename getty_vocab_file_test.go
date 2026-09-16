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
