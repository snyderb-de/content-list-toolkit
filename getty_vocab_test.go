package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProbeGettyReachabilityAcceptsAHealthyService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept header = %q, want application/json", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	result := probeGettyReachability(context.Background(), server.Client(), server.URL)
	if !result.Reachable {
		t.Fatalf("expected reachable, got %+v", result)
	}
	if result.CheckedAt.IsZero() {
		t.Fatal("expected CheckedAt to be set")
	}
}

// A blocked network often answers with something other than the service, and
// a captive proxy page is not a working vocabulary lookup.
func TestProbeGettyReachabilityRejectsNon200(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusBadGateway, 499} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		result := probeGettyReachability(context.Background(), server.Client(), server.URL)
		server.Close()

		if result.Reachable {
			t.Fatalf("HTTP %d should not count as reachable", status)
		}
		if !strings.Contains(result.Detail, "proxy") {
			t.Fatalf("HTTP %d detail should mention a proxy, got %q", status, result.Detail)
		}
	}
}

func TestProbeGettyReachabilityReportsAnUnreachableHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close() // nothing is listening now

	result := probeGettyReachability(context.Background(), server.Client(), url)
	if result.Reachable {
		t.Fatal("expected unreachable")
	}
	if result.Detail == "" {
		t.Fatal("expected a human-readable detail")
	}
}

func TestDescribeProbeFailureExplainsCommonCauses(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: errors.New("context deadline exceeded"), want: "blocking"},
		{name: "dns", err: errors.New("dial tcp: lookup vocab.getty.edu: no such host"), want: "DNS"},
		{name: "tls", err: errors.New("x509: certificate signed by unknown authority"), want: "proxy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeProbeFailure(tc.err); !strings.Contains(got, tc.want) {
				t.Fatalf("describeProbeFailure(%v) = %q, want it to mention %q", tc.err, got, tc.want)
			}
		})
	}
}

// fakeVocabulary counts how many questions actually reach the source.
type fakeVocabulary struct {
	mu    sync.Mutex
	calls map[string]int
	terms map[string]gettyTermMatch
	err   error
}

func newFakeVocabulary(terms map[string]gettyTermMatch) *fakeVocabulary {
	return &fakeVocabulary{calls: map[string]int{}, terms: terms}
}

func (f *fakeVocabulary) SourceName() string { return "fake" }

func (f *fakeVocabulary) Lookup(_ context.Context, term string) (gettyTermMatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[strings.ToLower(term)]++
	if f.err != nil {
		return gettyTermMatch{}, f.err
	}
	if match, ok := f.terms[strings.ToLower(term)]; ok {
		return match, nil
	}
	return gettyTermMatch{Term: term, Found: false}, nil
}

func (f *fakeVocabulary) callCount(term string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[strings.ToLower(term)]
}

func TestCachedVocabularyAsksTheSourceOncePerTerm(t *testing.T) {
	fake := newFakeVocabulary(map[string]gettyTermMatch{
		"aerial photographs": {Term: "aerial photographs", Found: true, SubjectID: "300046300"},
	})
	cache := newCachedVocabulary(fake)

	for i := 0; i < 5; i++ {
		match, err := cache.Lookup(context.Background(), "aerial photographs")
		if err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
		if !match.Found {
			t.Fatalf("lookup %d: expected the term to be found", i)
		}
	}

	if got := fake.callCount("aerial photographs"); got != 1 {
		t.Fatalf("source consulted %d times, want 1", got)
	}
	if got := cache.Lookups(); got != 1 {
		t.Fatalf("Lookups() = %d, want 1", got)
	}
}

// A sheet repeats the same term in different casing constantly; that must not
// double the request count.
func TestCachedVocabularyIgnoresCaseAndSurroundingSpace(t *testing.T) {
	fake := newFakeVocabulary(map[string]gettyTermMatch{
		"landscapes": {Term: "landscapes", Found: true, SubjectID: "300008626"},
	})
	cache := newCachedVocabulary(fake)

	for _, spelling := range []string{"landscapes", "Landscapes", "LANDSCAPES", " landscapes "} {
		if _, err := cache.Lookup(context.Background(), spelling); err != nil {
			t.Fatalf("%q: %v", spelling, err)
		}
	}
	if got := cache.Lookups(); got != 1 {
		t.Fatalf("Lookups() = %d, want 1", got)
	}
}

// The cached answer must come back labeled with the caller's spelling, so a
// report line quotes what is actually in the sheet.
func TestCachedVocabularyReportsTheCallersSpelling(t *testing.T) {
	fake := newFakeVocabulary(map[string]gettyTermMatch{
		"landscapes": {Term: "landscapes", Found: true, PreferredLabel: "landscapes"},
	})
	cache := newCachedVocabulary(fake)

	if _, err := cache.Lookup(context.Background(), "landscapes"); err != nil {
		t.Fatal(err)
	}
	match, err := cache.Lookup(context.Background(), "Landscapes")
	if err != nil {
		t.Fatal(err)
	}
	if match.Term != "Landscapes" {
		t.Fatalf("Term = %q, want the caller's spelling %q", match.Term, "Landscapes")
	}
	if match.ExactLabel() {
		t.Fatal("a casing difference from Getty's preferred label is not an exact match")
	}
}

// A term genuinely absent from the vocabulary is an answer worth caching.
func TestCachedVocabularyCachesMisses(t *testing.T) {
	fake := newFakeVocabulary(map[string]gettyTermMatch{})
	cache := newCachedVocabulary(fake)

	for i := 0; i < 3; i++ {
		match, err := cache.Lookup(context.Background(), "not a real term")
		if err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
		if match.Found {
			t.Fatalf("lookup %d: expected the term to be absent", i)
		}
	}
	if got := fake.callCount("not a real term"); got != 1 {
		t.Fatalf("source consulted %d times for a miss, want 1", got)
	}
}

// A transport failure on one row must not poison the same term on later rows.
func TestCachedVocabularyDoesNotCacheErrors(t *testing.T) {
	fake := newFakeVocabulary(map[string]gettyTermMatch{
		"landscapes": {Term: "landscapes", Found: true},
	})
	fake.err = errors.New("network unreachable")
	cache := newCachedVocabulary(fake)

	if _, err := cache.Lookup(context.Background(), "landscapes"); err == nil {
		t.Fatal("expected the first lookup to fail")
	}

	fake.mu.Lock()
	fake.err = nil
	fake.mu.Unlock()

	match, err := cache.Lookup(context.Background(), "landscapes")
	if err != nil {
		t.Fatalf("expected recovery after the error cleared: %v", err)
	}
	if !match.Found {
		t.Fatal("expected the term to be found on retry")
	}
}

func TestCachedVocabularyIsSafeUnderConcurrency(t *testing.T) {
	fake := newFakeVocabulary(map[string]gettyTermMatch{
		"landscapes": {Term: "landscapes", Found: true},
	})
	cache := newCachedVocabulary(fake)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := cache.Lookup(ctx, "landscapes"); err != nil {
				t.Errorf("concurrent lookup: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := cache.Lookups(); got < 1 {
		t.Fatalf("Lookups() = %d, want at least 1", got)
	}
}
