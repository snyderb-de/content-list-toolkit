package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Captured verbatim from vocab.getty.edu for the term "aerial photographs".
// Note that the same concept comes back three times, once per language.
//
// Data from the Getty Art & Architecture Thesaurus (AAT), J. Paul Getty Trust,
// used under the Open Data Commons Attribution License (ODC-By) 1.0.
const aatAerialPhotographsResponse = `{
  "head" : {
    "vars" : [ "s", "label" ]
  },
  "results" : {
    "bindings" : [ {
      "s" : {
        "type" : "uri",
        "value" : "http://vocab.getty.edu/aat/300128222"
      },
      "label" : {
        "xml:lang" : "es",
        "type" : "literal",
        "value" : "fotografías aéreas (photographs)"
      }
    }, {
      "s" : {
        "type" : "uri",
        "value" : "http://vocab.getty.edu/aat/300128222"
      },
      "label" : {
        "xml:lang" : "en",
        "type" : "literal",
        "value" : "aerial photographs"
      }
    }, {
      "s" : {
        "type" : "uri",
        "value" : "http://vocab.getty.edu/aat/300128222"
      },
      "label" : {
        "xml:lang" : "nl",
        "type" : "literal",
        "value" : "luchtfoto's"
      }
    } ]
  }
}`

const aatEmptyResponse = `{
  "head" : { "vars" : [ "s", "label" ] },
  "results" : { "bindings" : [ ] }
}`

func TestParseAATResponseMatchesTheEnglishLabel(t *testing.T) {
	match, err := parseAATLookupResponse([]byte(aatAerialPhotographsResponse), "aerial photographs", "aerial photographs")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !match.Found {
		t.Fatal("expected the term to be found")
	}
	if match.SubjectID != "300128222" {
		t.Fatalf("SubjectID = %q, want 300128222", match.SubjectID)
	}
	if match.PreferredLabel != "aerial photographs" {
		t.Fatalf("PreferredLabel = %q, want the English label", match.PreferredLabel)
	}
}

// The same response carries Spanish and Dutch labels for the same concept.
// Matching one of those would accept a term the sheet never contained.
func TestParseAATResponseIgnoresOtherLanguages(t *testing.T) {
	for _, term := range []string{"luchtfoto's", "fotografías aéreas (photographs)"} {
		match, err := parseAATLookupResponse([]byte(aatAerialPhotographsResponse), term, term)
		if err != nil {
			t.Fatalf("%q: %v", term, err)
		}
		if match.Found {
			t.Fatalf("%q is a non-English label and must not count as a match", term)
		}
	}
}

// luc:term is a full-text search, so the endpoint returns candidates. A near
// miss must not be accepted just because Getty offered it.
func TestParseAATResponseRejectsANearMiss(t *testing.T) {
	match, err := parseAATLookupResponse([]byte(aatAerialPhotographsResponse), "aerial photograph", "aerial photograph")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if match.Found {
		t.Fatal("a singular near miss is not the AAT term")
	}
}

func TestParseAATResponseMatchesCaseInsensitively(t *testing.T) {
	match, err := parseAATLookupResponse([]byte(aatAerialPhotographsResponse), "Aerial Photographs", "Aerial Photographs")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !match.Found {
		t.Fatal("expected a case-insensitive match")
	}
	if match.ExactLabel() {
		t.Fatal("the sheet and Getty disagree about case, so this is not an exact label match")
	}
}

func TestParseAATResponseHandlesNoResults(t *testing.T) {
	match, err := parseAATLookupResponse([]byte(aatEmptyResponse), "not a real term", "not a real term")
	if err != nil {
		t.Fatalf("an absent term is an answer, not an error: %v", err)
	}
	if match.Found {
		t.Fatal("expected the term to be absent")
	}
}

func TestParseAATResponseRejectsUnreadableJSON(t *testing.T) {
	if _, err := parseAATLookupResponse([]byte("<html>gateway timeout</html>"), "landscapes", "landscapes"); err == nil {
		t.Fatal("expected an error for a non-JSON response")
	}
}

// A term arrives from a spreadsheet cell, so it is arbitrary text. It must not
// be able to close the literal and rewrite the query.
func TestSPARQLStringLiteralEscapesTheTerm(t *testing.T) {
	for _, tc := range []struct {
		name string
		term string
		want string
	}{
		{name: "quote", term: `artists" books`, want: `"artists\" books"`},
		{name: "backslash", term: `a\b`, want: `"a\\b"`},
		{name: "newline", term: "a\nb", want: `"a\nb"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sparqlStringLiteral(tc.term); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

// Apostrophes are common in AAT terms and must survive into the query intact.
func TestBuildAATLookupQueryKeepsApostrophes(t *testing.T) {
	query := buildAATLookupQuery("artists' books")
	if !strings.Contains(query, `"artists' books"`) {
		t.Fatalf("query should carry the term verbatim, got:\n%s", query)
	}
	if !strings.Contains(query, aatScheme) {
		t.Fatal("query should restrict results to the AAT scheme")
	}
}

func TestAATSubjectIDReducesTheURI(t *testing.T) {
	for _, tc := range []struct{ uri, want string }{
		{uri: "http://vocab.getty.edu/aat/300128222", want: "300128222"},
		{uri: "http://vocab.getty.edu/aat/300128222/", want: "300128222"},
		{uri: "", want: ""},
	} {
		if got := aatSubjectID(tc.uri); got != tc.want {
			t.Fatalf("aatSubjectID(%q) = %q, want %q", tc.uri, got, tc.want)
		}
	}
}

func TestSPARQLVocabularyLookupEndToEnd(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/sparql-results+json")
		_, _ = w.Write([]byte(aatAerialPhotographsResponse))
	}))
	t.Cleanup(server.Close)

	vocabulary := newSPARQLVocabulary(server.Client(), server.URL)
	match, err := vocabulary.Lookup(context.Background(), "aerial photographs")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !match.Found || match.SubjectID != "300128222" {
		t.Fatalf("unexpected match: %+v", match)
	}
	if !strings.Contains(gotQuery, "luc:term") {
		t.Fatalf("query did not reach the endpoint intact: %q", gotQuery)
	}
}

// "We could not ask" must never be recorded as "the term is wrong".
func TestSPARQLVocabularyReportsServiceFailuresAsErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>proxy error</body></html>\nsecond line"))
	}))
	t.Cleanup(server.Close)

	vocabulary := newSPARQLVocabulary(server.Client(), server.URL)
	match, err := vocabulary.Lookup(context.Background(), "landscapes")
	if err == nil {
		t.Fatal("expected an error for HTTP 502")
	}
	if match.Found {
		t.Fatal("a failed request must not report the term as found")
	}
	if strings.Contains(err.Error(), "second line") {
		t.Fatalf("error should be kept to one line, got %q", err.Error())
	}
}

func TestSPARQLVocabularyIgnoresAnEmptyTerm(t *testing.T) {
	vocabulary := newSPARQLVocabulary(http.DefaultClient, "http://127.0.0.1:1/unused")
	match, err := vocabulary.Lookup(context.Background(), "   ")
	if err != nil {
		t.Fatalf("an empty cell should not reach the network: %v", err)
	}
	if match.Found {
		t.Fatal("an empty term is not found")
	}
}

func TestSPARQLVocabularyComposesWithTheCache(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(aatAerialPhotographsResponse))
	}))
	t.Cleanup(server.Close)

	cache := newCachedVocabulary(newSPARQLVocabulary(server.Client(), server.URL))
	for i := 0; i < 5; i++ {
		if _, err := cache.Lookup(context.Background(), "aerial photographs"); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	if requests != 1 {
		t.Fatalf("endpoint received %d requests, want 1", requests)
	}
}
