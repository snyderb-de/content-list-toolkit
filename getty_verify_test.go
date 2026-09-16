package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func verifyVocabulary(t *testing.T) *fileVocabulary {
	t.Helper()
	vocabulary, err := readVocabulary(strings.NewReader(
		"aerial photographs\nlandscapes\ncity plans\n"), "test list")
	if err != nil {
		t.Fatalf("readVocabulary: %v", err)
	}
	return vocabulary
}

func TestVerifyTagsAcceptsKnownTerms(t *testing.T) {
	result := checkTagCell("aerial photographs; landscapes; city plans")
	verifyTags(context.Background(), verifyVocabulary(t), &result)

	if !result.OK() {
		t.Fatalf("expected no issues, got %+v", result.Issues)
	}
	if len(result.Terms) != 3 {
		t.Fatalf("expected 3 verdicts, got %d", len(result.Terms))
	}
	for _, verdict := range result.Terms {
		if !verdict.Checked || !verdict.Found {
			t.Fatalf("term %q: %+v", verdict.Term, verdict)
		}
	}
}

// An unknown term makes an otherwise clean row worth reporting.
func TestVerifyTagsFlagsUnknownTerms(t *testing.T) {
	result := checkTagCell("aerial photographs; invented term; city plans")
	verifyTags(context.Background(), verifyVocabulary(t), &result)

	if !hasIssue(result, tagIssueUnknownTerm) {
		t.Fatalf("expected an unknown-term issue, got %+v", result.Issues)
	}
	for _, issue := range result.Issues {
		if issue.Kind != tagIssueUnknownTerm {
			continue
		}
		if !strings.Contains(issue.Detail, `"invented term"`) {
			t.Fatalf("detail should name the term, got %q", issue.Detail)
		}
		// The report has to be able to say a miss came from a list rather
		// than from Getty itself.
		if !strings.Contains(issue.Detail, "test list") {
			t.Fatalf("detail should name the source, got %q", issue.Detail)
		}
		if issue.Severity != severityReview {
			t.Fatalf("an unknown term does not break the upload, got %q", issue.Severity)
		}
		if issue.Repaired {
			t.Fatal("an unknown term cannot be repaired by cleaning")
		}
	}
}

func TestVerifyTagsReportsCaseDisagreement(t *testing.T) {
	result := checkTagCell("Aerial Photographs; landscapes; city plans")
	verifyTags(context.Background(), verifyVocabulary(t), &result)

	if !hasIssue(result, tagIssueTermCase) {
		t.Fatalf("expected a term-case issue, got %+v", result.Issues)
	}
	if hasIssue(result, tagIssueUnknownTerm) {
		t.Fatal("a case difference is not an unknown term")
	}
}

// The distinction the whole design rests on: a lookup that failed is not a
// term that is wrong.
func TestVerifyTagsSeparatesFailedLookupsFromAbsentTerms(t *testing.T) {
	fake := newFakeVocabulary(map[string]gettyTermMatch{})
	fake.err = errors.New("network unreachable")

	result := checkTagCell("aerial photographs; landscapes; city plans")
	verifyTags(context.Background(), fake, &result)

	if hasIssue(result, tagIssueUnknownTerm) {
		t.Fatalf("a failed lookup must not be reported as an unknown term: %+v", result.Issues)
	}
	if !hasIssue(result, tagIssueNotChecked) {
		t.Fatalf("expected a not-checked issue, got %+v", result.Issues)
	}
	for _, verdict := range result.Terms {
		if verdict.Checked {
			t.Fatalf("term %q should be marked unchecked", verdict.Term)
		}
		if verdict.Error == "" {
			t.Fatalf("term %q should carry the reason it could not be checked", verdict.Term)
		}
	}
}

// One unreachable term must not abandon the rest of the cell.
func TestVerifyTagsContinuesAfterAFailure(t *testing.T) {
	result := checkTagCell("aerial photographs; landscapes; city plans")
	verifyTags(context.Background(), failingOnceVocabulary{inner: verifyVocabulary(t)}, &result)

	if len(result.Terms) != 3 {
		t.Fatalf("expected all 3 terms to be attempted, got %d", len(result.Terms))
	}
	checked := 0
	for _, verdict := range result.Terms {
		if verdict.Checked {
			checked++
		}
	}
	if checked != 2 {
		t.Fatalf("expected 2 terms to be checked after one failure, got %d", checked)
	}
}

type failingOnceVocabulary struct {
	inner  gettyVocabulary
	called bool
}

func (f failingOnceVocabulary) SourceName() string { return f.inner.SourceName() }

func (f failingOnceVocabulary) Lookup(ctx context.Context, term string) (gettyTermMatch, error) {
	if term == "aerial photographs" {
		return gettyTermMatch{}, errors.New("timed out")
	}
	return f.inner.Lookup(ctx, term)
}

func TestVerifyTagsWithoutAVocabularyDoesNothing(t *testing.T) {
	result := checkTagCell("aerial photographs; invented term; city plans")
	before := len(result.Issues)
	verifyTags(context.Background(), nil, &result)

	if len(result.Issues) != before {
		t.Fatalf("a structure-only run must raise no vocabulary issues, got %+v", result.Issues)
	}
	if len(result.Terms) != 0 {
		t.Fatal("no vocabulary means no verdicts")
	}
}

// End to end: an otherwise clean sheet still reports a row whose term is not
// in the vocabulary.
func TestCheckTagSheetWithVocabularyReportsUnknownTerms(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; landscapes; city plans"),
		rowWithTags("aerial photographs; invented term; city plans"),
	})

	report, err := checkTagSheetWithVocabulary(context.Background(), path, verifyVocabulary(t))
	if err != nil {
		t.Fatalf("checkTagSheetWithVocabulary: %v", err)
	}
	if report.VocabularySource == "" {
		t.Fatal("the report must record which vocabulary answered")
	}
	if len(report.Rows) != 1 {
		t.Fatalf("expected 1 row with findings, got %d", len(report.Rows))
	}
	if report.Rows[0].Number != 3 {
		t.Fatalf("finding is on row %d, want 3", report.Rows[0].Number)
	}
	if report.BlockingRows() != 0 {
		t.Fatal("an unknown term does not break the upload")
	}
	if report.ReviewRows() != 1 {
		t.Fatalf("ReviewRows = %d, want 1", report.ReviewRows())
	}
}

// A structure-only run must not claim a vocabulary was consulted.
func TestCheckTagSheetWithoutVocabularyRecordsNoSource(t *testing.T) {
	path := writeXLSX(t, [][]string{
		mainTableHeader,
		rowWithTags("aerial photographs; invented term; city plans"),
	})

	report, err := checkTagSheet(path)
	if err != nil {
		t.Fatalf("checkTagSheet: %v", err)
	}
	if report.VocabularySource != "" {
		t.Fatalf("VocabularySource = %q, want empty", report.VocabularySource)
	}
	if len(report.Rows) != 0 {
		t.Fatalf("structure is clean, so nothing should be reported: %+v", report.Rows)
	}
}
