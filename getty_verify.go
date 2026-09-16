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
	// Suggestions are near matches offered when the term was not found, so the
	// screen can propose a replacement instead of leaving a dead end.
	Suggestions []string `json:"suggestions,omitempty"`
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

	var caseDiffers []string
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
				// One issue per unknown term rather than one listing them all,
				// so each carries its own suggestions and the screen can offer
				// a replacement for the specific term.
				verdict.Suggestions = suggestFor(ctx, vocabulary, tag)
				result.Issues = append(result.Issues, newTagIssue(tagIssueUnknownTerm,
					describeUnknownTerm(tag, vocabulary.SourceName(), verdict.Suggestions)))
			} else if verdict.CaseDiffers() {
				caseDiffers = append(caseDiffers,
					fmt.Sprintf("%q is spelled %q", tag, match.PreferredLabel))
			}
		}
		result.Terms = append(result.Terms, verdict)
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

// describeUnknownTerm names the term, the source that was asked, and what to
// use instead when the vocabulary can propose something. "Not an AAT term" on
// its own sends someone to the Getty website to search by hand.
func describeUnknownTerm(term, source string, suggestions []string) string {
	detail := fmt.Sprintf("%q is not in %s", term, source)
	if len(suggestions) == 0 {
		return detail
	}
	return fmt.Sprintf("%s — did you mean %s?", detail, strings.Join(quoteAll(suggestions), ", "))
}

func quoteAll(values []string) []string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return quoted
}
