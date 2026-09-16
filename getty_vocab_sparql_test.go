package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Shaped like a response to the current query, which returns ?s, ?matched and
// ?label. The concept, its identifier, and its three language labels are taken
// from a real captured response for "aerial photographs"; the ?matched column
// is added to fit the query as it now stands. Replace this with a verbatim
// capture the next time one is taken.
//
// Data from the Getty Art & Architecture Thesaurus (AAT), J. Paul Getty Trust,
// used under the Open Data Commons Attribution License (ODC-By) 1.0.
const aatAerialPhotographsResponse = `{
  "head" : { "vars" : [ "s", "matched", "label" ] },
  "results" : {
    "bindings" : [ {
      "s" : { "type" : "uri", "value" : "http://vocab.getty.edu/aat/300128222" },
      "matched" : { "xml:lang" : "en", "type" : "literal", "value" : "aerial photographs" },
      "label" : { "xml:lang" : "es", "type" : "literal", "value" : "fotograf\u00edas a\u00e9reas (photographs)" }
    }, {
      "s" : { "type" : "uri", "value" : "http://vocab.getty.edu/aat/300128222" },
      "matched" : { "xml:lang" : "en", "type" : "literal", "value" : "aerial photographs" },
      "label" : { "xml:lang" : "en", "type" : "literal", "value" : "aerial photographs" }
    }, {
      "s" : { "type" : "uri", "value" : "http://vocab.getty.edu/aat/300128222" },
      "matched" : { "xml:lang" : "en", "type" : "literal", "value" : "aerial photographs" },
      "label" : { "xml:lang" : "nl", "type" : "literal", "value" : "luchtfoto\u0027s" }
    } ]
  }
}`

// A term matched through an alternate spelling: what the cataloguer typed is
// not what Getty prefers, but it is still a real AAT term.
const aatAlternateLabelResponse = `{
  "head" : { "vars" : [ "s", "matched", "label" ] },
  "results" : {
    "bindings" : [ {
      "s" : { "type" : "uri", "value" : "http://vocab.getty.edu/aat/300046300" },
      "matched" : { "xml:lang" : "en", "type" : "literal", "value" : "photos" },
      "label" : { "xml:lang" : "en", "type" : "literal", "value" : "photographs" }
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

// luc:term saturates the row limit on ordinary terms — "black-and-white
// photographs", "balustrades, railings and their components", and "artists'
// books" each returned a full page of candidates from the live endpoint. If
// the equality test lived only in Go, the true match would be dropped whenever
// it fell outside that page, and a valid term would be reported as absent.
func TestAATQueryFiltersServerSideSoTheLimitCannotHideAMatch(t *testing.T) {
	query := buildAATLookupQuery("black-and-white photographs")

	if !strings.Contains(query, "FILTER(lcase(str(?matched))") {
		t.Fatalf("query must compare labels server-side, got:\n%s", query)
	}
	if !strings.Contains(query, `"black-and-white photographs"`) {
		t.Fatalf("query must carry the lowercased term to compare against, got:\n%s", query)
	}
	if !strings.Contains(query, "luc:term") {
		t.Fatalf("query should still use the full-text index to find candidates, got:\n%s", query)
	}
}

// Language is deliberately not filtered in SPARQL: a langMatches that excluded
// untagged literals would produce absences indistinguishable from real ones.
func TestAATQueryDoesNotFilterLanguageServerSide(t *testing.T) {
	query := buildAATLookupQuery("landscapes")
	if strings.Contains(query, "langMatches") {
		t.Fatalf("language belongs in Go, not the query, got:\n%s", query)
	}
}

// The term is lowercased for the server-side comparison, but a mixed-case
// sheet entry must still match.
func TestAATQueryLowercasesTheComparisonTerm(t *testing.T) {
	query := buildAATLookupQuery("Aerial Photographs")
	if !strings.Contains(query, `"aerial photographs"`) {
		t.Fatalf("comparison term should be lowercased, got:\n%s", query)
	}
	if !strings.Contains(query, `luc:term "Aerial Photographs"`) {
		t.Fatalf("the search term should keep its original spelling, got:\n%s", query)
	}
}

// Escaping still has to hold now that the term appears in the query twice.
func TestAATQueryEscapesTheTermInBothPositions(t *testing.T) {
	query := buildAATLookupQuery(`a"b`)
	if strings.Count(query, `\"`) != 2 {
		t.Fatalf("both occurrences of the term must be escaped, got:\n%s", query)
	}
}

// A legitimate AAT variant is not a mistake. Reporting it as absent would send
// a cataloguer to correct something already correct, so it counts as found and
// carries Getty's preferred spelling for the report to suggest.
func TestParseAATResponseAcceptsAnAlternateLabel(t *testing.T) {
	match, err := parseAATLookupResponse([]byte(aatAlternateLabelResponse), "photos", "photos")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !match.Found {
		t.Fatal("an alternate label is still an AAT term")
	}
	if match.SubjectID != "300046300" {
		t.Fatalf("SubjectID = %q", match.SubjectID)
	}
	if match.PreferredLabel != "photographs" {
		t.Fatalf("PreferredLabel = %q, want Getty's preferred spelling", match.PreferredLabel)
	}
	if match.ExactLabel() {
		t.Fatal("the term and the preferred spelling differ, so this is not an exact match")
	}
}

// Both label properties have to be searched, or a valid variant is reported as
// absent from the vocabulary — the worst answer this module can give.
func TestAATQuerySearchesAlternateLabelsToo(t *testing.T) {
	query := buildAATLookupQuery("photos")
	for _, want := range []string{"xl:prefLabel/xl:literalForm ?matched", "xl:altLabel/xl:literalForm ?matched", "UNION"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q:\n%s", want, query)
		}
	}
}
