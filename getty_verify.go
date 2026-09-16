package main

import (
	"context"
	"fmt"
	"strings"
)

// Term verification is the second half of the check. Cleaning decides whether
// the file will load; verification decides whether the terms in it are terms
// the vocabulary actually holds.
//
// It is kept separate from cleaning because it can fail. Cleaning is pure text
// work that always produces an answer, while a lookup can time out or be
// blocked, and a report must never present "we could not ask" as "this term is
// wrong".

// TagTermVerdict is the outcome for a single term within a cell.
type TagTermVerdict struct {
	Term string `json:"term"`
	// Checked is false when the lookup itself failed, which is different from
	// the term being absent.
	Checked        bool   `json:"checked"`
	Found          bool   `json:"found"`
	SubjectID      string `json:"subjectId,omitempty"`
	PreferredLabel string `json:"preferredLabel,omitempty"`
	// Error carries why the term could not be checked, when Checked is false.
	Error string `json:"error,omitempty"`
}

// CaseDiffers reports a term that exists but is spelled with different case
// than the vocabulary uses.
func (v TagTermVerdict) CaseDiffers() bool {
	return v.Found && v.PreferredLabel != "" && v.Term != v.PreferredLabel
}

// verifyTags looks up every term in a checked cell and records what came back.
//
// Lookup failures are collected rather than returned, because one unreachable
// term must not abandon the rest of the sheet. The count of unchecked terms is
// what the report uses to say how much of the verification actually happened.
func verifyTags(ctx context.Context, vocabulary gettyVocabulary, result *TagCheckResult) {
	if vocabulary == nil || len(result.Tags) == 0 {
		return
	}

	var unknown, caseDiffers []string
	unchecked := 0

	for _, tag := range result.Tags {
		match, err := vocabulary.Lookup(ctx, tag)
		verdict := TagTermVerdict{Term: tag}
		switch {
		case err != nil:
			verdict.Error = err.Error()
			unchecked++
		default:
			verdict.Checked = true
			verdict.Found = match.Found
			verdict.SubjectID = match.SubjectID
			verdict.PreferredLabel = match.PreferredLabel
			if !match.Found {
				unknown = append(unknown, tag)
			} else if verdict.CaseDiffers() {
				caseDiffers = append(caseDiffers,
					fmt.Sprintf("%q is spelled %q", tag, match.PreferredLabel))
			}
		}
		result.Terms = append(result.Terms, verdict)
	}

	if len(unknown) > 0 {
		result.Issues = append(result.Issues, newTagIssue(tagIssueUnknownTerm,
			fmt.Sprintf("not found in %s: %s", vocabulary.SourceName(), strings.Join(quoteAll(unknown), ", "))))
	}
	if len(caseDiffers) > 0 {
		result.Issues = append(result.Issues, newTagIssue(tagIssueTermCase,
			strings.Join(caseDiffers, "; ")))
	}
	if unchecked > 0 {
		result.Issues = append(result.Issues, newTagIssue(tagIssueNotChecked,
			fmt.Sprintf("%s could not be checked against the vocabulary",
				pluralize(unchecked, "term", "terms"))))
	}
}

func quoteAll(values []string) []string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return quoted
}
