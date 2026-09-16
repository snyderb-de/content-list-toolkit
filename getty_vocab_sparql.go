package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The live source queries Getty's SPARQL endpoint. It is authoritative in a
// way an exported term list cannot be, at the cost of needing the network.
//
// Two properties of the endpoint shape this code. luc:term is a full-text
// search, so it returns candidates rather than exact matches, and the label
// pattern returns the preferred label in every language Getty holds — a search
// for "aerial photographs" comes back with the Spanish and Dutch labels for
// the same concept alongside the English one. Deciding whether the sheet's
// term is really in the vocabulary therefore happens here, not in the query.
//
// Getty Vocabulary data is published under ODC-By 1.0: results displayed to a
// user must credit the Getty Research Institute and name the vocabulary.

const (
	gettySPARQLEndpoint = "https://vocab.getty.edu/sparql.json"
	aatScheme           = "http://vocab.getty.edu/aat/"
	gettyLookupTimeout  = 15 * time.Second

	// englishLanguage is the label language the CONTENTdm records are written
	// in. Getty tags labels with a language, and an untagged label is accepted
	// as well rather than discarded.
	englishLanguage = "en"
)

type sparqlVocabulary struct {
	endpoint string
	client   *http.Client
}

func newSPARQLVocabulary(client *http.Client, endpoint string) *sparqlVocabulary {
	if client == nil {
		client = &http.Client{Timeout: gettyLookupTimeout}
	}
	if endpoint == "" {
		endpoint = gettySPARQLEndpoint
	}
	return &sparqlVocabulary{endpoint: endpoint, client: client}
}

func (v *sparqlVocabulary) SourceName() string {
	return "Getty Art & Architecture Thesaurus (live, vocab.getty.edu)"
}

// Lookup asks Getty whether the term exists in the AAT.
//
// A term Getty does not hold returns Found false with a nil error. A network
// or service failure returns an error, because "we could not ask" must not be
// recorded as "the term is wrong".
func (v *sparqlVocabulary) Lookup(ctx context.Context, term string) (gettyTermMatch, error) {
	cleaned := normalizeTagText(term).Cleaned
	if cleaned == "" {
		return gettyTermMatch{Term: term}, nil
	}

	endpoint, err := url.Parse(v.endpoint)
	if err != nil {
		return gettyTermMatch{Term: term}, fmt.Errorf("invalid Getty endpoint %q: %w", v.endpoint, err)
	}
	query := endpoint.Query()
	query.Set("query", buildAATLookupQuery(cleaned))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return gettyTermMatch{Term: term}, fmt.Errorf("build Getty request: %w", err)
	}
	request.Header.Set("Accept", "application/sparql-results+json")

	response, err := v.client.Do(request)
	if err != nil {
		return gettyTermMatch{Term: term}, fmt.Errorf("ask Getty about %q: %s", term, describeProbeFailure(err))
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return gettyTermMatch{Term: term}, fmt.Errorf("read Getty response for %q: %w", term, err)
	}
	if response.StatusCode != http.StatusOK {
		return gettyTermMatch{Term: term}, fmt.Errorf("Getty answered HTTP %d for %q: %s",
			response.StatusCode, term, firstLine(string(body)))
	}

	return parseAATLookupResponse(body, term, cleaned)
}

// buildAATLookupQuery asks for candidate concepts and their preferred labels.
//
// The search deliberately stays broad and the exact comparison happens in Go.
// Pushing the comparison into SPARQL would mean relying on the endpoint's
// collation for case folding, and a stricter query that returned nothing would
// be indistinguishable from a term that genuinely does not exist.
func buildAATLookupQuery(term string) string {
	return fmt.Sprintf(`PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
PREFIX xl: <http://www.w3.org/2008/05/skos-xl#>
PREFIX luc: <http://www.ontotext.com/owlim/lucene#>
SELECT ?s ?label WHERE {
  ?s luc:term %s ;
     skos:inScheme <%s> ;
     xl:prefLabel/xl:literalForm ?label .
} LIMIT 50`, sparqlStringLiteral(term), aatScheme)
}

// sparqlStringLiteral quotes a term for inclusion in a query.
//
// Terms come from a spreadsheet cell, so they reach here as arbitrary text.
// Backslashes and quotes are escaped so the literal cannot be closed early and
// the rest of the query rewritten.
func sparqlStringLiteral(term string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return `"` + replacer.Replace(term) + `"`
}

// sparqlResults is the SPARQL 1.1 JSON results shape Getty returns.
type sparqlResults struct {
	Results struct {
		Bindings []map[string]sparqlBinding `json:"bindings"`
	} `json:"results"`
}

type sparqlBinding struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	Language string `json:"xml:lang"`
}

// parseAATLookupResponse decides whether the term is really in the vocabulary.
//
// A concept is a match only when one of its English preferred labels equals the
// sheet's term, compared without regard to case. Labels in other languages are
// carried in the same response and are ignored: the Spanish label for aerial
// photographs is not evidence that the English term was spelled correctly.
func parseAATLookupResponse(body []byte, originalTerm, cleanedTerm string) (gettyTermMatch, error) {
	var results sparqlResults
	if err := json.Unmarshal(body, &results); err != nil {
		return gettyTermMatch{Term: originalTerm}, fmt.Errorf("Getty returned an unreadable response for %q: %w", originalTerm, err)
	}

	want := strings.ToLower(cleanedTerm)
	for _, binding := range results.Results.Bindings {
		label, ok := binding["label"]
		if !ok || !isEnglishLabel(label.Language) {
			continue
		}
		if strings.ToLower(normalizeTagText(label.Value).Cleaned) != want {
			continue
		}
		return gettyTermMatch{
			Term:           originalTerm,
			Found:          true,
			SubjectID:      aatSubjectID(binding["s"].Value),
			PreferredLabel: label.Value,
		}, nil
	}

	return gettyTermMatch{Term: originalTerm, Found: false}, nil
}

// isEnglishLabel accepts English labels and untagged ones. Getty tags labels
// with a language, but an untagged literal is not a reason to reject a match.
func isEnglishLabel(language string) bool {
	if language == "" {
		return true
	}
	// Accept regional variants such as en-GB.
	return strings.EqualFold(language, englishLanguage) ||
		strings.HasPrefix(strings.ToLower(language), englishLanguage+"-")
}

// aatSubjectID reduces a concept URI to the AAT identifier, so a report can
// print 300128222 rather than the full URI.
func aatSubjectID(uri string) string {
	if uri == "" {
		return ""
	}
	trimmed := strings.TrimSuffix(uri, "/")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

// firstLine keeps an error message to one line, since a failing endpoint may
// answer with an entire HTML page.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if index := strings.IndexAny(s, "\r\n"); index >= 0 {
		s = s[:index]
	}
	const limit = 200
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}
