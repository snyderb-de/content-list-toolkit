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
// Two properties of the endpoint shape this code, both observed rather than
// assumed. luc:term is a full-text search that saturates the row limit on
// ordinary terms, so the equality test belongs in the query where LIMIT cannot
// discard the true match. And the label pattern returns the preferred label in
// every language Getty holds — a search for "aerial photographs" comes back
// with the Spanish and Dutch labels for the same concept alongside the English
// one — so the language test belongs here, where a silent mismatch cannot be
// confused with a term that genuinely does not exist.
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

// buildAATLookupQuery finds concepts whose preferred or alternate label
// equals the term, and asks for the preferred label alongside.
//
// Alternate labels count as found. A cataloguer who writes a legitimate AAT
// variant has not made a mistake, and reporting it as absent from the
// vocabulary would send them to correct something that is already correct.
// Because the preferred spelling comes back in the same answer, a variant can
// be reported as "this is the term, Getty spells it this way" instead.
//
// luc:term is a full-text search and saturates the row limit on ordinary
// terms, so the equality test belongs in the query where LIMIT cannot discard
// the true match. The language test does not: a SPARQL langMatches that
// excluded untagged literals would fail silently, and its absences would look
// exactly like real ones, so labels come back in every language and Go decides
// which count.
func buildAATLookupQuery(term string) string {
	return fmt.Sprintf(`PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
PREFIX xl: <http://www.w3.org/2008/05/skos-xl#>
PREFIX luc: <http://www.ontotext.com/owlim/lucene#>
SELECT ?s ?matched ?label WHERE {
  ?s luc:term %s ;
     skos:inScheme <%s> ;
     xl:prefLabel/xl:literalForm ?label .
  { ?s xl:prefLabel/xl:literalForm ?matched }
  UNION
  { ?s xl:altLabel/xl:literalForm ?matched }
  FILTER(lcase(str(?matched)) = %s)
} LIMIT 60`, sparqlStringLiteral(term), aatScheme, sparqlStringLiteral(strings.ToLower(term)))
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
// A concept counts when the label that matched is English or untagged. The
// preferred label is reported separately, so a term matched through an
// alternate spelling comes back with Getty's preferred form attached and the
// screen can say which it is rather than calling it wrong.
//
// Labels in other languages ride along in the same response and are ignored:
// the Spanish label for aerial photographs is not evidence that the English
// term was spelled correctly.
func parseAATLookupResponse(body []byte, originalTerm, cleanedTerm string) (gettyTermMatch, error) {
	var results sparqlResults
	if err := json.Unmarshal(body, &results); err != nil {
		return gettyTermMatch{Term: originalTerm}, fmt.Errorf("Getty returned an unreadable response for %q: %w", originalTerm, err)
	}

	want := strings.ToLower(cleanedTerm)
	match := gettyTermMatch{Term: originalTerm}

	for _, binding := range results.Results.Bindings {
		matched, ok := binding["matched"]
		if !ok || !isEnglishLabel(matched.Language) {
			continue
		}
		if strings.ToLower(normalizeTagText(matched.Value).Cleaned) != want {
			continue
		}

		match.Found = true
		match.SubjectID = aatSubjectID(binding["s"].Value)

		// Several rows carry the same concept, one per language of its
		// preferred label. Keep the English one.
		if label, ok := binding["label"]; ok && isEnglishLabel(label.Language) {
			match.PreferredLabel = label.Value
			return match, nil
		}
	}

	return match, nil
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

// Suggest proposes AAT terms near one the vocabulary does not hold.
//
// This is the search luc:term was already doing: its candidates are concepts
// whose labels share words with the term. For a lookup they are noise, which is
// why the equality filter exists. For a suggestion they are exactly the answer,
// so the same search runs without the filter.
func (v *sparqlVocabulary) Suggest(ctx context.Context, term string) ([]string, error) {
	cleaned := normalizeTagText(term).Cleaned
	if cleaned == "" {
		return nil, nil
	}

	endpoint, err := url.Parse(v.endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid Getty endpoint %q: %w", v.endpoint, err)
	}
	query := endpoint.Query()
	query.Set("query", buildAATSuggestQuery(cleaned))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/sparql-results+json")

	response, err := v.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Getty answered HTTP %d", response.StatusCode)
	}

	return parseAATSuggestions(body, cleaned)
}

func buildAATSuggestQuery(term string) string {
	return fmt.Sprintf(`PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
PREFIX xl: <http://www.w3.org/2008/05/skos-xl#>
PREFIX luc: <http://www.ontotext.com/owlim/lucene#>
SELECT ?label WHERE {
  ?s luc:term %s ;
     skos:inScheme <%s> ;
     xl:prefLabel/xl:literalForm ?label .
} LIMIT 60`, sparqlStringLiteral(term), aatScheme)
}

// parseAATSuggestions keeps the English labels, drops duplicates, and drops the
// term itself. A suggestion identical to what was typed would imply the tool
// had contradicted itself.
func parseAATSuggestions(body []byte, term string) ([]string, error) {
	var results sparqlResults
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, err
	}

	seen := map[string]bool{strings.ToLower(term): true}
	var suggestions []string
	for _, binding := range results.Results.Bindings {
		label, ok := binding["label"]
		if !ok || !isEnglishLabel(label.Language) {
			continue
		}
		value := normalizeTagText(label.Value).Cleaned
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		suggestions = append(suggestions, value)
		if len(suggestions) == maxSuggestions {
			break
		}
	}
	return suggestions, nil
}
