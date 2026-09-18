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
		client = secureHTTPClient(gettyProbeTimeout)
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
	// Variant marks a term matched through a non-preferred label: a real AAT
	// term, but not the one this catalogue accepts. The sheet takes Getty's
	// preferred term only, so a variant is a correction to make rather than a
	// spelling curiosity, and PreferredLabel carries what to put in its place.
	Variant bool `json:"variant,omitempty"`
	// QualifierIgnored marks a match made only after dropping a parenthetical
	// qualifier, because the source could not check it. Getty writes
	// "counters (furniture)"; the relational archive stores the term as bare
	// "counters" and keeps the qualifier somewhere the export does not carry.
	// Such a match confirms the term but says nothing about the qualifier.
	QualifierIgnored bool `json:"qualifierIgnored,omitempty"`
}

// ExactLabel reports whether the sheet spelling matches Getty's preferred
// label exactly. A term found through a variant spelling is not exact, and the
// catalogue requires the preferred label, so it has to be replaced.
func (m gettyTermMatch) ExactLabel() bool {
	return m.Found && m.PreferredLabel != "" && m.Term == m.PreferredLabel
}

// splitQualifier separates "counters (furniture)" into its term and qualifier.
// A term without a trailing parenthetical comes back unchanged.
func splitQualifier(term string) (base, qualifier string) {
	trimmed := strings.TrimSpace(term)
	if !strings.HasSuffix(trimmed, ")") {
		return trimmed, ""
	}
	open := strings.LastIndex(trimmed, "(")
	if open <= 0 {
		return trimmed, ""
	}
	base = strings.TrimSpace(trimmed[:open])
	qualifier = strings.TrimSpace(trimmed[open+1 : len(trimmed)-1])
	if base == "" || qualifier == "" {
		return trimmed, ""
	}
	return base, qualifier
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

// gettyListNamed is an optional capability for sources with a short name worth
// using where the full SourceName would repeat: one unknown-term line per term
// carrying "(176,629 terms, no variant information)" is noise by the third row.
type gettyListNamed interface {
	ListName() string
}

// listNameFor returns the short name when a source has one, and the full
// source name otherwise.
func listNameFor(vocabulary gettyVocabulary) string {
	if named, ok := vocabulary.(gettyListNamed); ok {
		return named.ListName()
	}
	return vocabulary.SourceName()
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

// backedSuggester answers "did you mean" from a second source when the first
// cannot.
//
// Getty's endpoint searches full words: ask it about "portait photography" and
// it matches on "photography" and returns a scattering of terms containing that
// word, never "portrait photography" — the term actually meant is not in what
// it sends back, so no amount of ranking finds it. The bundled list can match
// across a typo, because it compares letter by letter.
//
// So the live source stays the authority on whether a term exists, and the
// bundled list is consulted only for the suggestion — and only when the live
// answer holds nothing close. A suggestion is a proposal a person accepts or
// ignores, not a verdict, which is what makes borrowing one from a fixed copy
// reasonable where borrowing a verdict would not be.
type backedSuggester struct {
	gettyVocabulary
	backup gettySuggester
}

func (s backedSuggester) Suggest(ctx context.Context, term string) ([]string, error) {
	var live []string
	if suggester, ok := s.gettyVocabulary.(gettySuggester); ok {
		suggestions, err := suggester.Suggest(ctx, term)
		if err == nil {
			live = suggestions
		}
	}

	// The endpoint is enough when something it returned holds every word of
	// what was typed. Holding one word of two is how "portait photography"
	// came back as a list of unrelated photography terms.
	ranked, best := rankSuggestions(live, term)
	if best >= perfectScore(term) || s.backup == nil {
		return ranked, nil
	}

	backup, err := s.backup.Suggest(ctx, term)
	if err != nil || len(backup) == 0 {
		return ranked, nil
	}
	fromBackup, backupBest := rankSuggestions(backup, term)
	if backupBest <= best {
		return ranked, nil
	}
	// The closer set leads; whatever the endpoint offered follows, so nothing
	// it found is thrown away.
	return trimSuggestions(append(fromBackup, ranked...)), nil
}

// trimSuggestions drops repeats and keeps the list readable.
func trimSuggestions(all []string) []string {
	seen := map[string]bool{}
	var kept []string
	for _, suggestion := range all {
		key := strings.ToLower(suggestion)
		if seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, suggestion)
		if len(kept) == maxSuggestions {
			break
		}
	}
	return kept
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

// ListName passes the wrapped source's short name through for the same reason
// SnapshotNote does: wrapping a list in a cache must not change what a report
// says about it.
func (c *cachedVocabulary) ListName() string { return listNameFor(c.inner) }

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
