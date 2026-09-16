package main

import (
	"archive/zip"
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
	if len(terms) != 1 || terms[0] != "aerial photographs" {
		t.Fatalf("expected only the English term, got %v", terms)
	}
}

// Variants are real AAT terms. A cataloguer using one has not made a mistake.
func TestExtractEnglishTermsKeepsVariants(t *testing.T) {
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

	if _, err := extractEnglishTerms(path); err == nil {
		t.Fatal("expected an error when the expected tables are absent")
	} else if !strings.Contains(err.Error(), "format may have changed") {
		t.Fatalf("error should say the format may have changed, got %q", err)
	}
}

// The written list has to be readable by the offline check, including the
// terms that contain commas.
func TestWrittenTermListLoadsBackIntoTheVocabulary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AAT terms.csv")
	if err := writeTermList(path, []string{
		"aerial photographs",
		"balustrades, railings and their components",
		"counters (furniture)",
	}); err != nil {
		t.Fatalf("writeTermList: %v", err)
	}

	vocabulary, err := loadVocabularyFile(path)
	if err != nil {
		t.Fatalf("loadVocabularyFile: %v", err)
	}
	if vocabulary.Count() != 3 {
		t.Fatalf("expected 3 terms, got %d", vocabulary.Count())
	}
}

func TestFindRelationalArchiveMatchesTheIndexLink(t *testing.T) {
	page := []byte(`<a href="VocabData/aat_xml_0126.zip">XML</a> <a href="VocabData/aat_rel_0126.zip">Relational</a>`)
	match := relationalArchivePattern.Find(page)
	if string(match) != "VocabData/aat_rel_0126.zip" {
		t.Fatalf("matched %q", string(match))
	}
}
