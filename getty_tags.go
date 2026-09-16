package main

import (
	"fmt"
	"sort"
	"strings"
)

// Getty AAT terms reach the Access "Tags" field by copy and paste out of the
// Getty web interface, which drags invisible characters along with the text.
// Those characters survive into Excel and then break the tab-delimited export
// that CONTENTdm ingests, so every line currently gets checked by hand.
//
// normalizeTagText reproduces the spreadsheet formula that work relies on:
//
//	=TRIM(CLEAN(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(
//	    K3,CHAR(160)," "),UNICHAR(8203),""),UNICHAR(65279),""),
//	    UNICHAR(8239)," "),CHAR(9)," ")))
//
// and extends it to the other separators and invisible marks that show up in
// pasted vocabulary text. Anything beyond the formula is marked in the table
// below so the difference stays visible.

const (
	tagSeparator  = "; "
	minTagsPerRow = 3
	maxTagsPerRow = 5

	// replacementChar means the text was already damaged by an earlier
	// encoding round trip. The original character is unrecoverable, so it is
	// reported and left alone rather than guessed at.
	replacementChar = '\uFFFD'
)

// ghostRune is one invisible or layout character and what to do with it.
// A replacement of " " keeps word boundaries intact; "" deletes outright.
type ghostRune struct {
	value       rune
	name        string
	replacement string
	inFormula   bool // present in the spreadsheet formula above
}

var ghostRunes = []ghostRune{
	{value: '\u00A0', name: "no-break space", replacement: " ", inFormula: true},
	{value: '\u200B', name: "zero width space", replacement: "", inFormula: true},
	{value: '\uFEFF', name: "zero width no-break space", replacement: "", inFormula: true},
	{value: '\u202F', name: "narrow no-break space", replacement: " ", inFormula: true},
	{value: '\t', name: "tab", replacement: " ", inFormula: true},

	// Beyond the formula. Every one of these is a real paste artifact that
	// either splits a word invisibly or reads as whitespace to Excel but not
	// to a tab-delimited parser.
	{value: '\u00AD', name: "soft hyphen", replacement: ""},
	{value: '\u2060', name: "word joiner", replacement: ""},
	{value: '\u200C', name: "zero width non-joiner", replacement: ""},
	{value: '\u200D', name: "zero width joiner", replacement: ""},
	{value: '\u2000', name: "en quad", replacement: " "},
	{value: '\u2001', name: "em quad", replacement: " "},
	{value: '\u2002', name: "en space", replacement: " "},
	{value: '\u2003', name: "em space", replacement: " "},
	{value: '\u2004', name: "three-per-em space", replacement: " "},
	{value: '\u2005', name: "four-per-em space", replacement: " "},
	{value: '\u2006', name: "six-per-em space", replacement: " "},
	{value: '\u2007', name: "figure space", replacement: " "},
	{value: '\u2008', name: "punctuation space", replacement: " "},
	{value: '\u2009', name: "thin space", replacement: " "},
	{value: '\u200A', name: "hair space", replacement: " "},
	{value: '\u205F', name: "medium mathematical space", replacement: " "},
	{value: '\u3000', name: "ideographic space", replacement: " "},
	{value: '\u2028', name: "line separator", replacement: " "},
	{value: '\u2029', name: "paragraph separator", replacement: " "},
}

var ghostRuneIndex = func() map[rune]ghostRune {
	index := make(map[rune]ghostRune, len(ghostRunes))
	for _, g := range ghostRunes {
		index[g.value] = g
	}
	return index
}()

// tagTextNormalization records what changed so the report can show the work
// rather than silently rewriting archival metadata.
type tagTextNormalization struct {
	Original string
	Cleaned  string
	Changes  map[string]int // character name -> times it was found
	// SpacingAdjusted records ordinary ASCII spacing that Excel's TRIM would
	// also have tidied: leading, trailing, or repeated spaces. It is tracked
	// apart from Changes because a plain space is visible, does not break a
	// tab-delimited upload, and saying "ghost characters" about one would be
	// both alarming and wrong.
	SpacingAdjusted bool
}

// Changed reports whether normalization altered the text at all.
func (n tagTextNormalization) Changed() bool {
	return n.Original != n.Cleaned
}

// ChangeSummary renders the recorded changes in a stable order, so report
// output does not reshuffle between runs over the same sheet.
func (n tagTextNormalization) ChangeSummary() string {
	if len(n.Changes) == 0 {
		return ""
	}
	names := make([]string, 0, len(n.Changes))
	for name := range n.Changes {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		if count := n.Changes[name]; count == 1 {
			parts = append(parts, name)
		} else {
			parts = append(parts, fmt.Sprintf("%s ×%d", name, count))
		}
	}
	return strings.Join(parts, ", ")
}

// normalizeTagText strips invisible characters, then applies the same leading,
// trailing, and repeated-space squeeze that Excel's TRIM performs.
func normalizeTagText(s string) tagTextNormalization {
	result := tagTextNormalization{Original: s, Changes: map[string]int{}}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if ghost, ok := ghostRuneIndex[r]; ok {
			result.Changes[ghost.name]++
			b.WriteString(ghost.replacement)
			continue
		}
		// Excel's CLEAN drops characters 0 through 31. DEL is added here
		// because it survives a paste just as easily and is just as invisible.
		if r < 0x20 || r == 0x7F {
			result.Changes[fmt.Sprintf("control character U+%04X", r)]++
			continue
		}
		b.WriteRune(r)
	}

	converted := b.String()
	result.Cleaned = squeezeSpaces(converted)
	result.SpacingAdjusted = result.Cleaned != converted
	return result
}

// squeezeSpaces collapses runs of ASCII spaces into one and trims the ends,
// matching Excel TRIM. Only the ASCII space is considered, because every other
// space-like rune has already been converted by normalizeTagText.
func squeezeSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pendingSpace := false
	for _, r := range s {
		if r == ' ' {
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

type tagIssueKind string

const (
	tagIssueGhostCharacters tagIssueKind = "ghost-characters"
	tagIssueSeparator       tagIssueKind = "separator"
	tagIssueTagCount        tagIssueKind = "tag-count"
	tagIssueEmptyTag        tagIssueKind = "empty-tag"
	tagIssueDuplicateTag    tagIssueKind = "duplicate-tag"
	tagIssueDamagedText     tagIssueKind = "damaged-text"
	tagIssueFieldLimit      tagIssueKind = "field-limit"
	tagIssueWhitespace      tagIssueKind = "whitespace"
)

// accessTagsFieldLimit is the width of [Tags] in the CONTENTdm Access
// template: Text(255). Access truncates silently at entry, and it truncates by
// character count rather than at a separator, so an over-long cell arrives with
// its final term cut mid-word.
const accessTagsFieldLimit = 255

// tagIssueSeverity separates the two questions this module answers. A blocking
// issue is one that stops the tab-delimited file from loading into CONTENTdm
// at all. A review issue uploads perfectly well but is worth a human look.
//
// Severity describes the cell as it arrived. Whether the cleaned output still
// carries the problem is a separate axis, reported by Repaired, because most
// blocking issues are exactly the ones this module fixes automatically.
type tagIssueSeverity string

const (
	severityBlocking tagIssueSeverity = "blocks-upload"
	severityReview   tagIssueSeverity = "review"
)

// issueSeverity classifies a problem by whether it breaks the upload.
//
// Ghost characters are blocking: a literal tab splits a column, a newline
// splits a row, and the invisible spaces are what currently force a hand check
// of every line. Everything else loads fine and is a metadata question.
func issueSeverity(kind tagIssueKind) tagIssueSeverity {
	switch kind {
	case tagIssueGhostCharacters:
		return severityBlocking
	default:
		return severityReview
	}
}

// repairedByCleaning reports whether writing the cleaned cell resolves the
// problem. A count that is out of range, a duplicate term, or text already
// damaged by a bad encoding round trip all need a person.
func repairedByCleaning(kind tagIssueKind) bool {
	switch kind {
	case tagIssueGhostCharacters, tagIssueSeparator, tagIssueEmptyTag, tagIssueWhitespace:
		return true
	default:
		return false
	}
}

type TagIssue struct {
	Kind     tagIssueKind     `json:"kind"`
	Severity tagIssueSeverity `json:"severity"`
	// Repaired is true when the cleaned cell no longer has this problem.
	Repaired bool   `json:"repaired"`
	Detail   string `json:"detail"`
}

// TagCheckResult is the outcome for a single Tags cell.
type TagCheckResult struct {
	Original string     `json:"original"`
	Cleaned  string     `json:"cleaned"`
	Tags     []string   `json:"tags"`
	Issues   []TagIssue `json:"issues"`
}

// Changed reports whether the cleaned cell differs from what was read.
func (r TagCheckResult) Changed() bool {
	return r.Original != r.Cleaned
}

// OK reports whether the cell passed every structural check.
func (r TagCheckResult) OK() bool {
	return len(r.Issues) == 0
}

// checkTagCell normalizes one Tags cell and reports what was wrong with it.
// The returned Cleaned value always uses the canonical "; " separator, so a
// cell that only had separator trouble comes back ready to upload.
//
// This checks structure only. Whether each term actually exists in the Getty
// AAT is a separate question, answered against the vocabulary itself.
func newTagIssue(kind tagIssueKind, detail string) TagIssue {
	return TagIssue{
		Kind:     kind,
		Severity: issueSeverity(kind),
		Repaired: repairedByCleaning(kind),
		Detail:   detail,
	}
}

func checkTagCell(s string) TagCheckResult {
	normalized := normalizeTagText(s)
	result := TagCheckResult{Original: s}

	if len(normalized.Changes) > 0 {
		result.Issues = append(result.Issues, newTagIssue(tagIssueGhostCharacters, normalized.ChangeSummary()))
	}
	if normalized.SpacingAdjusted {
		result.Issues = append(result.Issues, newTagIssue(tagIssueWhitespace, "leading, trailing, or repeated spaces tidied"))
	}
	if strings.ContainsRune(normalized.Cleaned, replacementChar) {
		result.Issues = append(result.Issues, newTagIssue(tagIssueDamagedText,
			"contains U+FFFD replacement character from an earlier encoding error; the original character cannot be recovered automatically"))
	}

	// The separator check runs against the text as it arrived, because
	// splitting and rejoining below repairs the spacing either way and would
	// hide the fact that it was ever wrong.
	if detail := describeSeparatorProblems(normalized.Cleaned); detail != "" {
		result.Issues = append(result.Issues, newTagIssue(tagIssueSeparator, detail))
	}

	var tags []string
	emptyTags := 0
	for _, part := range strings.Split(normalized.Cleaned, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			emptyTags++
			continue
		}
		tags = append(tags, part)
	}
	result.Tags = tags
	result.Cleaned = strings.Join(tags, tagSeparator)

	if emptyTags > 0 {
		result.Issues = append(result.Issues, newTagIssue(tagIssueEmptyTag,
			fmt.Sprintf("%s dropped", pluralize(emptyTags, "empty tag", "empty tags"))))
	}
	if duplicates := duplicateTags(tags); len(duplicates) > 0 {
		result.Issues = append(result.Issues, newTagIssue(tagIssueDuplicateTag,
			fmt.Sprintf("repeated: %s", strings.Join(duplicates, ", "))))
	}
	if detail := describeFieldLimit(s, tags); detail != "" {
		result.Issues = append(result.Issues, newTagIssue(tagIssueFieldLimit, detail))
	}
	if n := len(tags); n < minTagsPerRow || n > maxTagsPerRow {
		result.Issues = append(result.Issues, newTagIssue(tagIssueTagCount,
			fmt.Sprintf("%s, expected %d to %d",
				pluralize(n, "tag", "tags"), minTagsPerRow, maxTagsPerRow)))
	}

	return result
}

// describeSeparatorProblems reports separators that are not exactly "; ".
// It runs on normalized text, so a report of "no space" means the semicolon
// genuinely had nothing after it rather than an invisible character.
func describeSeparatorProblems(s string) string {
	runes := []rune(s)
	missingSpace := 0
	spaceBefore := 0
	for i, r := range runes {
		if r != ';' {
			continue
		}
		if i > 0 && runes[i-1] == ' ' {
			spaceBefore++
		}
		if i+1 < len(runes) && runes[i+1] != ' ' {
			missingSpace++
		}
	}

	var parts []string
	if missingSpace > 0 {
		parts = append(parts, fmt.Sprintf("%s missing the following space",
			pluralize(missingSpace, "separator", "separators")))
	}
	if spaceBefore > 0 {
		parts = append(parts, fmt.Sprintf("%s with a space before the semicolon",
			pluralize(spaceBefore, "separator", "separators")))
	}
	return strings.Join(parts, "; ")
}

// duplicateTags returns the tags that appear more than once, compared without
// regard to case, in the order they were first seen.
func duplicateTags(tags []string) []string {
	seen := make(map[string]int, len(tags))
	var repeated []string
	for _, tag := range tags {
		key := strings.ToLower(tag)
		seen[key]++
		if seen[key] == 2 {
			repeated = append(repeated, tag)
		}
	}
	return repeated
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// uploadUnsafeRunes returns the characters in s that would break a
// tab-delimited export: a tab splits a column, CR or LF splits a row, and any
// other control character is a parser's choice rather than a defined outcome.
//
// This is the post-condition on cleaning. Everything checkTagCell produces is
// expected to come back empty here, and a test enforces that over the whole
// ghost-character table.
func uploadUnsafeRunes(s string) []rune {
	var unsafe []rune
	seen := map[rune]bool{}
	for _, r := range s {
		if r != '\t' && r != '\r' && r != '\n' && r >= 0x20 && r != 0x7F {
			continue
		}
		if !seen[r] {
			seen[r] = true
			unsafe = append(unsafe, r)
		}
	}
	return unsafe
}

// CleanedUploadSafe reports whether the cleaned cell can be written into a
// tab-delimited file without breaking its structure.
func (r TagCheckResult) CleanedUploadSafe() bool {
	return len(uploadUnsafeRunes(r.Cleaned)) == 0
}

// OriginalBlocksUpload reports whether the cell as it arrived would have
// broken the tab-delimited load. This is what the module exists to catch.
func (r TagCheckResult) OriginalBlocksUpload() bool {
	for _, issue := range r.Issues {
		if issue.Severity == severityBlocking {
			return true
		}
	}
	return false
}

// NeedsReview reports whether anything survives the cleaning and still wants a
// person to look at it.
func (r TagCheckResult) NeedsReview() bool {
	for _, issue := range r.Issues {
		if !issue.Repaired {
			return true
		}
	}
	return false
}

// describeFieldLimit reports cells at or beyond the Access field width.
//
// A cell measuring exactly the limit is the interesting case: Access cut it at
// entry, so the text on screen looks complete while the last term is missing
// characters. Counting runes rather than bytes matters here, because Access
// counts characters and a diacritic in a German or French term would otherwise
// shift the count.
func describeFieldLimit(original string, tags []string) string {
	length := len([]rune(original))
	switch {
	case length > accessTagsFieldLimit:
		return fmt.Sprintf("%d characters, longer than the %d the Access Tags field holds",
			length, accessTagsFieldLimit)
	case length == accessTagsFieldLimit:
		detail := fmt.Sprintf("exactly %d characters, the Access Tags field width, so it was probably truncated at entry",
			accessTagsFieldLimit)
		if len(tags) > 0 {
			if last := tags[len(tags)-1]; last != "" {
				detail += fmt.Sprintf("; check the final term %q", last)
			}
		}
		return detail
	default:
		return ""
	}
}
