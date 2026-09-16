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
