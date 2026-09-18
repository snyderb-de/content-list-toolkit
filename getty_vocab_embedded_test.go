package main

import (
	"context"
	"strings"
	"testing"
)

// The bundled list has to load from the binary with no file and no network.
func TestBuiltinVocabularyLoads(t *testing.T) {
	vocabulary, err := builtinVocabulary()
	if err != nil {
		t.Fatalf("builtinVocabulary: %v", err)
	}
	if vocabulary.Count() < 150000 {
		t.Fatalf("Count() = %d, expected the full extracted list", vocabulary.Count())
	}
	if !strings.Contains(vocabulary.SourceName(), embeddedVocabularyDate) {
		t.Fatalf("SourceName should carry the archive date, got %q", vocabulary.SourceName())
	}
}

// Building the index costs real work, so it happens once.
func TestBuiltinVocabularyIsBuiltOnce(t *testing.T) {
	first, err := builtinVocabulary()
	if err != nil {
		t.Fatal(err)
	}
	second, err := builtinVocabulary()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("expected the same index to be reused")
	}
}

func TestBuiltinVocabularyAnswersLookups(t *testing.T) {
	vocabulary, err := builtinVocabulary()
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"aerial photographs", "black-and-white photographs", "counter stools"} {
		match, lerr := vocabulary.Lookup(context.Background(), term)
		if lerr != nil {
			t.Fatalf("%q: %v", term, lerr)
		}
		if !match.Found {
			t.Fatalf("%q should be in the bundled list", term)
		}
	}
	match, err := vocabulary.Lookup(context.Background(), "definitely not an aat term")
	if err != nil {
		t.Fatal(err)
	}
	if match.Found {
		t.Fatal("expected an invented term to be absent")
	}
}

// It is a frozen archive, so it must declare itself a snapshot like any other
// dated source.
func TestBuiltinVocabularyDeclaresItselfASnapshot(t *testing.T) {
	vocabulary, err := builtinVocabulary()
	if err != nil {
		t.Fatal(err)
	}
	note := vocabulary.SnapshotNote()
	if !strings.Contains(note, embeddedVocabularyDate) {
		t.Fatalf("the caution should name the archive date, got %q", note)
	}
}

func TestBuiltinSourceResolvesThroughTheBinding(t *testing.T) {
	app := newApp("")
	vocabulary, err := app.buildVocabulary(GettyCheckOptions{Source: gettySourceBuiltin})
	if err != nil {
		t.Fatalf("buildVocabulary: %v", err)
	}
	if vocabulary == nil {
		t.Fatal("expected a vocabulary")
	}
	if !strings.Contains(vocabulary.SourceName(), "built-in") {
		t.Fatalf("SourceName = %q", vocabulary.SourceName())
	}
	// A dated source must not lose its caution behind the cache.
	if snapshotNoteFor(vocabulary) == "" {
		t.Fatal("the bundled list must still warn that it is a snapshot")
	}
}

// The bundled list is the whole English thesaurus, and it has to say which of
// its terms are variants — otherwise an offline check accepts a term the
// catalogue does not.
func TestBuiltinVocabularyAnswersAVariantWithItsPreferredTerm(t *testing.T) {
	vocabulary, err := builtinVocabulary()
	if err != nil {
		t.Fatal(err)
	}

	match, err := vocabulary.Lookup(context.Background(), "photos")
	if err != nil {
		t.Fatal(err)
	}
	if !match.Found || !match.Variant {
		t.Fatalf("%q is an AAT variant, got %+v", "photos", match)
	}
	if match.PreferredLabel != "photographs" {
		t.Fatalf("PreferredLabel = %q, want %q", match.PreferredLabel, "photographs")
	}

	// The preferred term of the same concept passes untouched.
	preferred, err := vocabulary.Lookup(context.Background(), "photographs")
	if err != nil {
		t.Fatal(err)
	}
	if !preferred.Found || preferred.Variant || !preferred.ExactLabel() {
		t.Fatalf("the preferred term should be accepted as written, got %+v", preferred)
	}
}

// AAT tags terms with one of three Englishes, and the bundled list has to hold
// all of them. "place settings" is American English: when the extract kept
// plain English only, the check called that tag — taken from a real sheet and
// confirmed valid at vocab.getty.edu — invalid.
func TestBuiltinVocabularyHoldsTheEnglishDialects(t *testing.T) {
	vocabulary, err := builtinVocabulary()
	if err != nil {
		t.Fatal(err)
	}

	match, err := vocabulary.Lookup(context.Background(), "place settings")
	if err != nil {
		t.Fatal(err)
	}
	if !match.Found || match.Variant {
		t.Fatalf("%q is an American English AAT term, got %+v", "place settings", match)
	}
}
