package main

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A miniature of the real archive: the two tables, the same column positions,
// and the same language convention.
func writeTestArchive(t *testing.T, terms, languages string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aat_rel_test.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer file.Close()

	w := zip.NewWriter(file)
	for name, content := range map[string]string{"TERM.out": terms, "LANGUAGE_RELS.out": languages} {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatalf("create entry: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return path
}

func term(id, subject, text, kind string) string {
	// Column positions mirror the real table: 7 is P/V, 9 the subject, 10 the
	// text, 11 the term id.
	return strings.Join([]string{"NA", "", "NA", "1", "", "C", "NA", kind, "", subject, text, id, "U"}, "\t") + "\n"
}

func language(langID, termID string) string {
	return strings.Join([]string{langID, "N", "300000000", termID, "", "AD", "SN", "U"}, "\t") + "\n"
}

// Without the language table, a list built from TERM alone would carry every
// language Getty holds.
func TestExtractEnglishTermsFiltersByLanguage(t *testing.T) {
	archive := writeTestArchive(t,
		term("1000000001", "300128222", "aerial photographs", termPreferred)+
			term("1000000002", "300128222", "fotografías aéreas", "V")+
			term("1000000003", "300128222", "luchtfoto's", "V"),
		language(aatEnglishLanguageID, "1000000001")+
			language("70261", "1000000002")+
			language("70641", "1000000003"),
	)

	terms, err := extractEnglishTerms(archive)
	if err != nil {
		t.Fatalf("extractEnglishTerms: %v", err)
	}
	if len(terms) != 1 || terms[0].Term != "aerial photographs" {
		t.Fatalf("expected only the English term, got %v", terms)
	}
}

// English is three languages in AAT, and missing two of them is what made the
// check call "place settings" — an American English term, and a real one —
// invalid.
func TestExtractEnglishTermsKeepsTheEnglishDialects(t *testing.T) {
	archive := writeTestArchive(t,
		term("1000227297", "300227297", "place settings", termPreferred)+
			term("1000285718", "300227297", "place setting", "V")+
			term("1000000003", "300000003", "full-size moulds", termPreferred),
		language(aatAmericanEnglishLanguageID, "1000227297")+
			language(aatAmericanEnglishLanguageID, "1000285718")+
			language(aatBritishEnglishLanguageID, "1000000003"),
	)

	terms, err := extractEnglishTerms(archive)
	if err != nil {
		t.Fatalf("extractEnglishTerms: %v", err)
	}
	if len(terms) != 3 {
		t.Fatalf("expected the dialect terms to be kept, got %v", terms)
	}
	if terms[0].Term != "place settings" || terms[0].Preferred != "" {
		t.Fatalf("an American English preferred term is a term to use, got %+v", terms[0])
	}
	if terms[1].Preferred != "place settings" {
		t.Fatalf("the variant should point at its preferred term, got %+v", terms[1])
	}
}

// Where a concept is preferred in more than one English, the plain-English
// label is the one to tell a cataloguer to use: it is what the live endpoint
// returns.
func TestExtractEnglishTermsPrefersPlainEnglishOverADialect(t *testing.T) {
	archive := writeTestArchive(t,
		term("1000000001", "300000001", "moulds", termPreferred)+
			term("1000000002", "300000001", "molds", termPreferred)+
			term("1000000003", "300000001", "moulding forms", "V"),
		language(aatBritishEnglishLanguageID, "1000000001")+
			language(aatEnglishLanguageID, "1000000002")+
			language(aatEnglishLanguageID, "1000000003"),
	)

	terms, err := extractEnglishTerms(archive)
	if err != nil {
		t.Fatalf("extractEnglishTerms: %v", err)
	}
	if terms[2].Preferred != "molds" {
		t.Fatalf("the variant should point at the plain-English label, got %+v", terms[2])
	}
	// Both preferred labels are terms to use as written, dialect or not.
	for _, entry := range terms[:2] {
		if entry.Preferred != "" {
			t.Fatalf("%q is a preferred term, got %+v", entry.Term, entry)
		}
	}
}

// Variants are kept, but as variants: the list has to be able to say "that is
// an AAT term, and here is the one to use instead".
func TestExtractEnglishTermsKeepsVariantsWithTheirPreferredTerm(t *testing.T) {
	archive := writeTestArchive(t,
		term("1000000001", "300046300", "photographs", termPreferred)+
			term("1000000002", "300046300", "photos", "V"),
		language(aatEnglishLanguageID, "1000000001")+
			language(aatEnglishLanguageID, "1000000002"),
	)

	terms, err := extractEnglishTerms(archive)
	if err != nil {
		t.Fatalf("extractEnglishTerms: %v", err)
	}
	if len(terms) != 2 {
		t.Fatalf("expected the preferred term and its variant, got %v", terms)
	}

	byTerm := map[string]vocabularyTerm{}
	for _, entry := range terms {
		byTerm[entry.Term] = entry
	}
	if got := byTerm["photographs"].Preferred; got != "" {
		t.Fatalf("the preferred term should point at nothing, got %q", got)
	}
	if got := byTerm["photos"].Preferred; got != "photographs" {
		t.Fatalf("the variant should point at %q, got %q", "photographs", got)
	}
}

// TERM is not ordered by concept, so a variant is commonly read before the
// preferred term it has to point at.
func TestExtractEnglishTermsLinksAVariantReadBeforeItsPreferredTerm(t *testing.T) {
	archive := writeTestArchive(t,
		term("1000000002", "300046300", "photos", "V")+
			term("1000000001", "300046300", "photographs", termPreferred),
		language(aatEnglishLanguageID, "1000000001")+
			language(aatEnglishLanguageID, "1000000002"),
	)

	terms, err := extractEnglishTerms(archive)
	if err != nil {
		t.Fatalf("extractEnglishTerms: %v", err)
	}
	if terms[0].Term != "photos" || terms[0].Preferred != "photographs" {
		t.Fatalf("the variant should carry its preferred term, got %+v", terms[0])
	}
}

// A concept whose preferred label is in another language leaves its English
// variant with nothing to point at. The term is still the best the list has.
func TestExtractEnglishTermsKeepsAVariantWithNoEnglishPreferredTerm(t *testing.T) {
	archive := writeTestArchive(t,
		term("1000000001", "300046300", "fotografías", termPreferred)+
			term("1000000002", "300046300", "photos", "V"),
		language("70261", "1000000001")+
			language(aatEnglishLanguageID, "1000000002"),
	)

	terms, err := extractEnglishTerms(archive)
	if err != nil {
		t.Fatalf("extractEnglishTerms: %v", err)
	}
	if len(terms) != 1 || terms[0].Term != "photos" || terms[0].Preferred != "" {
		t.Fatalf("expected the variant alone with no replacement, got %+v", terms)
	}
}

func TestExtractEnglishTermsRejectsAnArchiveMissingATable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	entry, _ := w.Create("SOMETHING_ELSE.out")
	_, _ = entry.Write([]byte("x"))
	w.Close()
	file.Close()

	// The message names the file to go back for, rather than describing a
	// table nobody outside this code has heard of.
	if _, err := extractEnglishTerms(path); err == nil {
		t.Fatal("expected an error when the expected tables are absent")
	} else if !strings.Contains(err.Error(), "aat_rel_") {
		t.Fatalf("error should name the archive to use, got %q", err)
	}
}

// The written list has to be readable by the offline check, including the
// terms that contain commas.
func TestWrittenTermListLoadsBackIntoTheVocabulary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AAT terms.csv")
	if err := writeTermList(path, []vocabularyTerm{
		{Term: "aerial photographs"},
		{Term: "balustrades, railings and their components"},
		{Term: "counters (furniture)"},
		{Term: "photos", Preferred: "photographs"},
		{Term: "photographs"},
	}); err != nil {
		t.Fatalf("writeTermList: %v", err)
	}

	vocabulary, err := loadVocabularyFile(path)
	if err != nil {
		t.Fatalf("loadVocabularyFile: %v", err)
	}
	if vocabulary.Count() != 5 {
		t.Fatalf("expected 5 terms, got %d", vocabulary.Count())
	}

	// The second column has to survive the round trip, or the check would read
	// every variant as a term to accept.
	match, err := vocabulary.Lookup(context.Background(), "photos")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !match.Found || !match.Variant || match.PreferredLabel != "photographs" {
		t.Fatalf("expected a variant answered with its preferred term, got %+v", match)
	}
}

// Dating the list from inside the archive rather than from the file's own
// timestamp: a copied archive has a new modification time and the same data.
func TestArchivePublishedReadsTheDatesInsideTheArchive(t *testing.T) {
	archive := writeTestArchive(t,
		term("1000000001", "300046300", "photographs", termPreferred),
		language(aatEnglishLanguageID, "1000000001"),
	)

	published, err := archivePublished(archive)
	if err != nil {
		t.Fatalf("archivePublished: %v", err)
	}
	if published.IsZero() {
		t.Fatal("expected a date from the archive entries")
	}
}

func TestArchivePublishedRejectsSomethingThatIsNotAnArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-an-archive.zip")
	if err := os.WriteFile(path, []byte("this is not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := archivePublished(path); err == nil {
		t.Fatal("expected an error for a file that is not an archive")
	}
}

// The whole import, as the screen runs it: an archive on disk in, a term list
// beside it out, named for the data's own date.
func TestImportGettyVocabularyWritesTheListBesideTheArchive(t *testing.T) {
	archive := writeTestArchive(t,
		term("1000000001", "300046300", "photographs", termPreferred)+
			term("1000000002", "300046300", "photos", "V"),
		language(aatEnglishLanguageID, "1000000001")+
			language(aatEnglishLanguageID, "1000000002"),
	)

	app := &App{}
	result, err := app.ImportGettyVocabulary(archive)
	if err != nil {
		t.Fatalf("ImportGettyVocabulary: %v", err)
	}
	if got := filepath.Dir(result.Path); got != filepath.Dir(archive) {
		t.Fatalf("the list should be written beside the archive, got %q", got)
	}
	if result.Terms != 2 {
		t.Fatalf("Terms = %d, want 2", result.Terms)
	}
	if !strings.Contains(filepath.Base(result.Path), "AAT terms") || !strings.HasSuffix(result.Path, ".csv") {
		t.Fatalf("unexpected name %q", filepath.Base(result.Path))
	}

	// The written list has to be the one the check reads, variants included.
	vocabulary, err := loadVocabularyFile(result.Path)
	if err != nil {
		t.Fatalf("loadVocabularyFile: %v", err)
	}
	match, err := vocabulary.Lookup(context.Background(), "photos")
	if err != nil {
		t.Fatal(err)
	}
	if !match.Variant || match.PreferredLabel != "photographs" {
		t.Fatalf("expected the variant to survive the import, got %+v", match)
	}
}

func TestImportGettyVocabularyRejectsAFileThatIsNotAnArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sheet.xlsx")
	if err := os.WriteFile(path, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&App{}).ImportGettyVocabulary(path); err == nil {
		t.Fatal("expected an error rather than an empty list")
	}
}

// Picking the XML archive instead of the relational one is the easiest
// mistake on that download page, and it used to fail with a message about a
// missing table.
func TestImportGettyVocabularyNamesTheXMLArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aat_xml_0126.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	entry, _ := w.Create("AATOut_1Subjects.xml")
	_, _ = entry.Write([]byte("<Vocabulary/>"))
	_ = w.Close()
	_ = file.Close()

	_, err = (&App{}).ImportGettyVocabulary(path)
	if err == nil {
		t.Fatal("expected the XML archive to be refused")
	}
	for _, want := range []string{"XML archive", "aat_rel_", "aat_xml_0126.zip"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should mention %q", err, want)
		}
	}
}

// Any other zip is refused by name too, rather than silently producing
// nothing.
func TestImportGettyVocabularyRefusesAnUnrelatedZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photos.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	entry, _ := w.Create("holiday.jpg")
	_, _ = entry.Write([]byte("not a vocabulary"))
	_ = w.Close()
	_ = file.Close()

	_, err = (&App{}).ImportGettyVocabulary(path)
	if err == nil {
		t.Fatal("expected an unrelated zip to be refused")
	}
	if !strings.Contains(err.Error(), "photos.zip") || !strings.Contains(err.Error(), "aat_rel_") {
		t.Fatalf("error %q should name the file and the one to use", err)
	}
}
