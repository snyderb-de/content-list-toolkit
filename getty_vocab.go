package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Tag terms are checked against the Getty Art & Architecture Thesaurus. Two
// sources can answer that question: a live lookup against vocab.getty.edu, or
// a snapshot extracted from Getty's monthly release and bundled with the app.
// Archive workstations vary in whether outbound access is permitted, so the
// source is a choice rather than an assumption, and reachability is surfaced
// in the interface instead of discovered through a failed scan.
//
// Getty Vocabulary data is published under the Open Data Commons Attribution
// License (ODC-By) 1.0. Anything built from it has to credit the Getty
// Research Institute and name the vocabulary.

const (
	// gettyProbeURL is a single stable AAT concept used only to decide whether
	// the service is reachable. It is deliberately not a search: a probe should
	// not depend on query syntax staying still.
	gettyProbeURL = "https://vocab.getty.edu/aat/300046300.json"

	// A reachability probe blocks the interface, so it fails fast. A real
	// lookup is allowed to take longer.
	gettyProbeTimeout = 6 * time.Second
)

// GettyReachability is what the indicator in the interface renders. It answers
// "can this machine reach Getty right now", nothing about a specific term.
type GettyReachability struct {
	Reachable bool      `json:"reachable"`
	CheckedAt time.Time `json:"checkedAt"`
	LatencyMS int64     `json:"latencyMs"`
	Detail    string    `json:"detail"`
}

// probeGettyReachability performs one HEAD-like check against a known concept.
// A non-200 response counts as unreachable: a proxy that answers with its own
// block page is not a working vocabulary service, however healthy it looks.
func probeGettyReachability(ctx context.Context, client *http.Client, url string) GettyReachability {
	if client == nil {
		client = &http.Client{Timeout: gettyProbeTimeout}
	}
	if url == "" {
		url = gettyProbeURL
	}

	ctx, cancel := context.WithTimeout(ctx, gettyProbeTimeout)
	defer cancel()

	started := time.Now()
	result := GettyReachability{CheckedAt: started}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		result.Detail = fmt.Sprintf("could not build the request: %v", err)
		return result
	}
	request.Header.Set("Accept", "application/json")

	response, err := client.Do(request)
	result.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Detail = describeProbeFailure(err)
		return result
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		result.Detail = fmt.Sprintf("Getty answered HTTP %d; a network proxy may be intercepting the request", response.StatusCode)
		return result
	}

	result.Reachable = true
	result.Detail = fmt.Sprintf("Getty responded in %d ms", result.LatencyMS)
	return result
}

// describeProbeFailure turns a transport error into something an archivist can
// act on, rather than a Go error string.
func describeProbeFailure(err error) string {
	text := err.Error()
	switch {
	case strings.Contains(text, "context deadline exceeded"), strings.Contains(text, "Client.Timeout"):
		return fmt.Sprintf("no response within %s; the network may be blocking vocab.getty.edu", gettyProbeTimeout)
	case strings.Contains(text, "no such host"):
		return "vocab.getty.edu could not be resolved; check DNS or the network connection"
	case strings.Contains(text, "certificate"):
		return "the TLS certificate was rejected, which usually means an intercepting proxy is in the way"
	default:
		return fmt.Sprintf("could not reach Getty: %v", err)
	}
}

// gettyTermMatch is the answer for one term.
type gettyTermMatch struct {
	Term string `json:"term"`
	// Found is false for a term the vocabulary does not contain. That is an
	// answer, not an error.
	Found bool `json:"found"`
	// SubjectID is the AAT identifier, e.g. "300046300", when found.
	SubjectID string `json:"subjectId,omitempty"`
	// PreferredLabel is Getty's own spelling. When it differs from Term only
	// by case or punctuation, the sheet is worth correcting toward Getty.
	PreferredLabel string `json:"preferredLabel,omitempty"`
}

// ExactLabel reports whether the sheet spelling matches Getty's preferred
// label exactly. A term can be found through a variant spelling and still be
// worth normalizing.
func (m gettyTermMatch) ExactLabel() bool {
	return m.Found && m.PreferredLabel != "" && m.Term == m.PreferredLabel
}

// gettyVocabulary is the seam between the checker and where terms are verified,
// so the live service and a bundled snapshot are interchangeable and a test can
// substitute a fake without touching the network.
type gettyVocabulary interface {
	// Lookup returns whether the term exists. A term that is absent returns a
	// match with Found false and a nil error; an error means the question could
	// not be answered at all.
	Lookup(ctx context.Context, term string) (gettyTermMatch, error)
	// SourceName identifies the backing source in reports, so a result is
	// traceable to the vocabulary version that produced it.
	SourceName() string
}

// gettySuggester is an optional capability. A source that can propose near
// matches for a term it does not hold implements it; one that cannot is still
// a perfectly good vocabulary, so this is separate from gettyVocabulary rather
// than part of it.
//
// Suggestions exist because "not an AAT term" on its own sends someone to the
// Getty website to search by hand. The candidates are already in the response
// the lookup makes, so proposing them costs almost nothing and turns a dead end
// into a choice.
type gettySuggester interface {
	Suggest(ctx context.Context, term string) ([]string, error)
}

// gettySnapshotSource is an optional capability for vocabularies that answer
// from a fixed copy rather than from Getty itself.
//
// This exists because of a case found in practice. Getty's January 2026
// archive lists "landscapes" as the preferred label of two AAT subjects; the
// live endpoint holds no such label today, because Getty disambiguated the
// term in the months between. A list built from the archive therefore reports
// that tag as valid when Getty no longer agrees.
//
// That is a false positive, and it is the harder failure to notice: a wrong
// "not found" gets investigated, while a wrong "found" is silently believed.
// Any source that cannot be current has to say so wherever its answers are
// shown.
type gettySnapshotSource interface {
	SnapshotNote() string
}

// snapshotNoteFor returns a source's caution, or nothing for a live source.
func snapshotNoteFor(vocabulary gettyVocabulary) string {
	if note, ok := vocabulary.(gettySnapshotSource); ok {
		return note.SnapshotNote()
	}
	return ""
}

// maxSuggestions keeps the list to something a person will actually read.
const maxSuggestions = 6

// suggestFor asks a vocabulary for near matches when it supports them, and
// returns nothing when it does not. A failure to suggest is never worth
// surfacing: the finding stands on its own without them.
func suggestFor(ctx context.Context, vocabulary gettyVocabulary, term string) []string {
	suggester, ok := vocabulary.(gettySuggester)
	if !ok {
		return nil
	}
	suggestions, err := suggester.Suggest(ctx, term)
	if err != nil {
		return nil
	}
	return suggestions
}

// cachedVocabulary memoizes lookups for the lifetime of one scan.
//
// A collection sheet repeats the same handful of AAT terms across hundreds of
// rows, so this is the difference between hundreds of requests and a few dozen.
// Misses are cached alongside hits, because a term absent from the vocabulary
// stays absent. Errors are never cached: a timeout on row 12 must not condemn
// the same term on row 400.
type cachedVocabulary struct {
	inner gettyVocabulary

	mu          sync.Mutex
	entries     map[string]gettyTermMatch
	suggestions map[string][]string
	lookups     int
}

func newCachedVocabulary(inner gettyVocabulary) *cachedVocabulary {
	return &cachedVocabulary{
		inner:       inner,
		entries:     map[string]gettyTermMatch{},
		suggestions: map[string][]string{},
	}
}

// Suggest passes through to the wrapped source when it can suggest, caching
// the answer. The same unknown term usually appears on many rows, and asking
// Getty about it once is the whole point of the cache.
func (c *cachedVocabulary) Suggest(ctx context.Context, term string) ([]string, error) {
	suggester, ok := c.inner.(gettySuggester)
	if !ok {
		return nil, nil
	}

	key := cacheKey(term)
	c.mu.Lock()
	cached, hit := c.suggestions[key]
	c.mu.Unlock()
	if hit {
		return cached, nil
	}

	found, err := suggester.Suggest(ctx, term)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.suggestions[key] = found
	c.mu.Unlock()
	return found, nil
}

func (c *cachedVocabulary) SourceName() string { return c.inner.SourceName() }

// SnapshotNote passes the wrapped source's caution through, so wrapping a
// dated list in a cache does not quietly hide that it is dated.
func (c *cachedVocabulary) SnapshotNote() string { return snapshotNoteFor(c.inner) }

func (c *cachedVocabulary) Lookup(ctx context.Context, term string) (gettyTermMatch, error) {
	key := cacheKey(term)

	c.mu.Lock()
	cached, ok := c.entries[key]
	c.mu.Unlock()
	if ok {
		// The cached answer was found under a possibly different spelling, so
		// report the term as this caller spelled it.
		cached.Term = term
		return cached, nil
	}

	match, err := c.inner.Lookup(ctx, term)
	if err != nil {
		return gettyTermMatch{Term: term}, err
	}

	c.mu.Lock()
	c.entries[key] = match
	c.lookups++
	c.mu.Unlock()
	return match, nil
}

// Lookups reports how many questions actually reached the underlying source,
// which the report uses to show the cache earning its keep.
func (c *cachedVocabulary) Lookups() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lookups
}

// cacheKey folds the differences that never change the answer. Getty matching
// is case-insensitive, and surrounding whitespace is gone by this point anyway.
func cacheKey(term string) string {
	return strings.ToLower(strings.TrimSpace(term))
}
