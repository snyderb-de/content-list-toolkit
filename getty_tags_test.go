package main

import (
	"fmt"
	"strings"
	"testing"
)

// The spreadsheet formula these tests pin behavior against:
//
//	=TRIM(CLEAN(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(
//	    K3,CHAR(160)," "),UNICHAR(8203),""),UNICHAR(65279),""),
//	    UNICHAR(8239)," "),CHAR(9)," ")))
func TestNormalizeTagTextMatchesSpreadsheetFormula(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no-break space becomes a space",
			input: "aerial\u00A0photographs",
			want:  "aerial photographs",
		},
		{
			name:  "zero width space is deleted outright",
			input: "photo\u200Bgraphs",
			want:  "photographs",
		},
		{
			name:  "byte order mark is deleted outright",
			input: "\uFEFFphotographs",
			want:  "photographs",
		},
		{
			name:  "narrow no-break space becomes a space",
			input: "aerial\u202Fphotographs",
			want:  "aerial photographs",
		},
		{
			name:  "tab becomes a space",
			input: "aerial\tphotographs",
			want:  "aerial photographs",
		},
		{
			name:  "CLEAN drops control characters",
			input: "aerial\u0007photographs\u001F",
			want:  "aerialphotographs",
		},
		{
			name:  "CLEAN drops embedded newlines",
			input: "aerial\r\nphotographs",
			want:  "aerialphotographs",
		},
		{
			name:  "TRIM squeezes repeated spaces and trims the ends",
			input: "   aerial     photographs   ",
			want:  "aerial photographs",
		},
		{
			name:  "converted characters then collapse as one space",
			input: "aerial\u00A0\u00A0\u202Fphotographs",
			want:  "aerial photographs",
		},
		{
			name:  "clean text is left alone",
			input: "aerial photographs",
			want:  "aerial photographs",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeTagText(tc.input).Cleaned; got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeTagTextBeyondTheFormula(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "soft hyphen", input: "photo\u00ADgraphs", want: "photographs"},
		{name: "word joiner", input: "photo\u2060graphs", want: "photographs"},
		{name: "zero width non-joiner", input: "photo\u200Cgraphs", want: "photographs"},
		{name: "zero width joiner", input: "photo\u200Dgraphs", want: "photographs"},
		{name: "en space", input: "aerial\u2002photographs", want: "aerial photographs"},
		{name: "em space", input: "aerial\u2003photographs", want: "aerial photographs"},
		{name: "thin space", input: "aerial\u2009photographs", want: "aerial photographs"},
		{name: "hair space", input: "aerial\u200Aphotographs", want: "aerial photographs"},
		{name: "medium mathematical space", input: "aerial\u205Fphotographs", want: "aerial photographs"},
		{name: "ideographic space", input: "aerial\u3000photographs", want: "aerial photographs"},
		{name: "line separator", input: "aerial\u2028photographs", want: "aerial photographs"},
		{name: "paragraph separator", input: "aerial\u2029photographs", want: "aerial photographs"},
		{name: "delete character", input: "aerial\u007Fphotographs", want: "aerialphotographs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeTagText(tc.input).Cleaned; got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeTagTextPreservesRealContent(t *testing.T) {
	// Hyphens, apostrophes, parentheses, diacritics, and commas all appear in
	// genuine AAT terms. Normalization must not touch any of them.
	input := "black-and-white photographs; Bauzeichnungen (Brüssel), 1890s; artists' books"
	if got := normalizeTagText(input).Cleaned; got != input {
		t.Fatalf("normalization altered real content:\n got %q\nwant %q", got, input)
	}
}

func TestNormalizeTagTextReportsWhatChanged(t *testing.T) {
	result := normalizeTagText("aerial\u00A0photo\u200Bgraphs\u00A0\u00A0here")
	if !result.Changed() {
		t.Fatal("expected Changed to be true")
	}
	summary := result.ChangeSummary()
	if !strings.Contains(summary, "no-break space ×3") {
		t.Fatalf("summary missing the no-break space count: %q", summary)
	}
	if !strings.Contains(summary, "zero width space") {
		t.Fatalf("summary missing the zero width space: %q", summary)
	}
	// Singular entries carry no multiplier.
	if strings.Contains(summary, "zero width space ×") {
		t.Fatalf("single occurrence should not be counted: %q", summary)
	}
}

func TestNormalizeTagTextUnchangedTextReportsNothing(t *testing.T) {
	result := normalizeTagText("aerial photographs")
	if result.Changed() {
		t.Fatal("expected Changed to be false")
	}
	if got := result.ChangeSummary(); got != "" {
		t.Fatalf("expected an empty summary, got %q", got)
	}
}

func TestCheckTagCellAcceptsAWellFormedCell(t *testing.T) {
	input := "aerial photographs; landscapes; city plans"
	result := checkTagCell(input)
	if !result.OK() {
		t.Fatalf("expected no issues, got %+v", result.Issues)
	}
	if result.Changed() {
		t.Fatalf("expected the cell to be left alone, got %q", result.Cleaned)
	}
	if len(result.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %q", len(result.Tags), result.Tags)
	}
}

func TestCheckTagCellRepairsSeparators(t *testing.T) {
	// Missing space after the semicolon, and a stray space before one.
	result := checkTagCell("aerial photographs;landscapes ; city plans")
	if want := "aerial photographs; landscapes; city plans"; result.Cleaned != want {
		t.Fatalf("got %q want %q", result.Cleaned, want)
	}
	if !hasIssue(result, tagIssueSeparator) {
		t.Fatalf("expected a separator issue, got %+v", result.Issues)
	}
}

func TestCheckTagCellFlagsTagCount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		wantTags int
	}{
		{name: "too few", input: "aerial photographs; landscapes", wantTags: 2},
		{name: "too many", input: "a; b; c; d; e; f", wantTags: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := checkTagCell(tc.input)
			if len(result.Tags) != tc.wantTags {
				t.Fatalf("expected %d tags, got %d", tc.wantTags, len(result.Tags))
			}
			if !hasIssue(result, tagIssueTagCount) {
				t.Fatalf("expected a tag-count issue, got %+v", result.Issues)
			}
		})
	}
}

func TestCheckTagCellCountsBoundariesAsValid(t *testing.T) {
	for _, input := range []string{
		"a; b; c",
		"a; b; c; d; e",
	} {
		if result := checkTagCell(input); hasIssue(result, tagIssueTagCount) {
			t.Fatalf("%q should be an acceptable tag count, got %+v", input, result.Issues)
		}
	}
}

func TestCheckTagCellDropsEmptyTags(t *testing.T) {
	result := checkTagCell("aerial photographs;; landscapes; city plans;")
	if want := "aerial photographs; landscapes; city plans"; result.Cleaned != want {
		t.Fatalf("got %q want %q", result.Cleaned, want)
	}
	if !hasIssue(result, tagIssueEmptyTag) {
		t.Fatalf("expected an empty-tag issue, got %+v", result.Issues)
	}
}

func TestCheckTagCellFlagsDuplicatesIgnoringCase(t *testing.T) {
	result := checkTagCell("landscapes; Landscapes; city plans")
	if !hasIssue(result, tagIssueDuplicateTag) {
		t.Fatalf("expected a duplicate-tag issue, got %+v", result.Issues)
	}
}

func TestCheckTagCellFlagsDamagedTextWithoutGuessing(t *testing.T) {
	input := "aerial photographs; Bru\uFFFDssel; city plans"
	result := checkTagCell(input)
	if !hasIssue(result, tagIssueDamagedText) {
		t.Fatalf("expected a damaged-text issue, got %+v", result.Issues)
	}
	if !strings.ContainsRune(result.Cleaned, replacementChar) {
		t.Fatal("the replacement character should be reported, not silently removed")
	}
}

// The case this module exists for: a realistic paste out of the Getty web
// interface, carrying the invisible characters that break the tab-delimited
// export, arriving through the hand-cleaning workflow it replaces.
func TestCheckTagCellHandlesARealisticGettyPaste(t *testing.T) {
	input := "\uFEFFaerial\u00A0photographs;\u200Blandscapes\u202F; city\tplans  "
	result := checkTagCell(input)

	if want := "aerial photographs; landscapes; city plans"; result.Cleaned != want {
		t.Fatalf("got %q want %q", result.Cleaned, want)
	}
	if !hasIssue(result, tagIssueGhostCharacters) {
		t.Fatalf("expected a ghost-character issue, got %+v", result.Issues)
	}
	if len(result.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %q", len(result.Tags), result.Tags)
	}
	if strings.ContainsAny(result.Cleaned, "\u00A0\u200B\uFEFF\u202F\t") {
		t.Fatalf("cleaned text still carries an invisible character: %q", result.Cleaned)
	}
}

func hasIssue(result TagCheckResult, kind tagIssueKind) bool {
	for _, issue := range result.Issues {
		if issue.Kind == kind {
			return true
		}
	}
	return false
}

// The five characters the spreadsheet formula handles are the contract with
// the existing hand-cleaning workflow. Anything else in the table is an
// addition, and this test keeps that line from blurring.
func TestGhostRuneTableTracksTheFormula(t *testing.T) {
	want := map[rune]string{
		'\u00A0': " ", // CHAR(160)
		'\u200B': "",  // UNICHAR(8203)
		'\uFEFF': "",  // UNICHAR(65279)
		'\u202F': " ", // UNICHAR(8239)
		'\t':     " ", // CHAR(9)
	}

	got := map[rune]string{}
	for _, g := range ghostRunes {
		if g.inFormula {
			got[g.value] = g.replacement
		}
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d characters marked inFormula, got %d", len(want), len(got))
	}
	for r, replacement := range want {
		actual, ok := got[r]
		if !ok {
			t.Fatalf("U+%04X is in the formula but not marked inFormula", r)
		}
		if actual != replacement {
			t.Fatalf("U+%04X: formula replaces with %q, table uses %q", r, replacement, actual)
		}
	}
}

// The guarantee the module rests on: whatever comes in, the cleaned cell is
// safe to write into a tab-delimited file. Checked against every character in
// the ghost table rather than a handful of examples.
func TestCleanedOutputIsAlwaysUploadSafe(t *testing.T) {
	for _, g := range ghostRunes {
		for _, shape := range []string{
			"aerial%[1]cphotographs; landscapes; city plans",
			"%[1]caerial photographs; landscapes; city plans",
			"aerial photographs; landscapes; city plans%[1]c",
			"aerial photographs;%[1]clandscapes; city plans",
		} {
			input := fmt.Sprintf(shape, g.value)
			result := checkTagCell(input)
			if unsafe := uploadUnsafeRunes(result.Cleaned); len(unsafe) > 0 {
				t.Fatalf("U+%04X (%s) in %q left unsafe characters %U in %q",
					g.value, g.name, shape, unsafe, result.Cleaned)
			}
		}
	}
}

func TestUploadUnsafeRunesFindsStructureBreakers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{name: "tab splits a column", input: "aerial\tphotographs", want: true},
		{name: "newline splits a row", input: "aerial\nphotographs", want: true},
		{name: "carriage return splits a row", input: "aerial\rphotographs", want: true},
		{name: "control character", input: "aerial\u0007photographs", want: true},
		{name: "delete character", input: "aerial\u007Fphotographs", want: true},
		{name: "ordinary text is fine", input: "aerial photographs; landscapes", want: false},
		{name: "diacritics are fine", input: "Bauzeichnungen (Br\u00FCssel)", want: false},
		{name: "a no-break space does not break the parse", input: "aerial\u00A0photographs", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := len(uploadUnsafeRunes(tc.input)) > 0
			if got != tc.want {
				t.Fatalf("uploadUnsafeRunes(%q) unsafe = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestIssueSeveritySeparatesUploadFailuresFromMetadataQuestions(t *testing.T) {
	// Invisible characters are what actually stops the load.
	blocking := checkTagCell("aerial\u00A0photographs; landscapes; city plans")
	if !blocking.OriginalBlocksUpload() {
		t.Fatalf("ghost characters should block the upload, got %+v", blocking.Issues)
	}
	if blocking.NeedsReview() {
		t.Fatalf("cleaning resolves ghost characters, so nothing should need review: %+v", blocking.Issues)
	}

	// A wrong tag count uploads perfectly well and is a question for a person.
	review := checkTagCell("aerial photographs; landscapes")
	if review.OriginalBlocksUpload() {
		t.Fatalf("a short tag count does not break the upload, got %+v", review.Issues)
	}
	if !review.NeedsReview() {
		t.Fatalf("a short tag count needs a person, got %+v", review.Issues)
	}
}

func TestRepairedIssuesAreResolvedInTheCleanedCell(t *testing.T) {
	result := checkTagCell("\uFEFFaerial photographs;landscapes;; city plans")
	for _, issue := range result.Issues {
		if !issue.Repaired {
			continue
		}
		switch issue.Kind {
		case tagIssueGhostCharacters:
			if normalizeTagText(result.Cleaned).Changed() {
				t.Fatalf("cleaned cell still carries ghost characters: %q", result.Cleaned)
			}
		case tagIssueSeparator:
			if describeSeparatorProblems(result.Cleaned) != "" {
				t.Fatalf("cleaned cell still has separator problems: %q", result.Cleaned)
			}
		case tagIssueEmptyTag:
			for _, tag := range result.Tags {
				if strings.TrimSpace(tag) == "" {
					t.Fatalf("cleaned cell still has an empty tag: %q", result.Tags)
				}
			}
		}
	}
}

// [Tags] is Text(255) in the CONTENTdm Access template. Access truncates at
// entry by character count, not at a separator, so the final term arrives cut.
func TestCheckTagCellFlagsTheAccessFieldWidth(t *testing.T) {
	// Build a cell that lands exactly on the limit.
	filler := strings.Repeat("a", accessTagsFieldLimit-len("aerial photographs; landscapes; "))
	atLimit := "aerial photographs; landscapes; " + filler
	if got := len([]rune(atLimit)); got != accessTagsFieldLimit {
		t.Fatalf("test fixture is %d characters, want %d", got, accessTagsFieldLimit)
	}

	result := checkTagCell(atLimit)
	if !hasIssue(result, tagIssueFieldLimit) {
		t.Fatalf("expected a field-limit issue, got %+v", result.Issues)
	}
	for _, issue := range result.Issues {
		if issue.Kind != tagIssueFieldLimit {
			continue
		}
		if issue.Severity != severityReview {
			t.Fatalf("a truncated cell uploads fine and is a question for a person, got %q", issue.Severity)
		}
		if issue.Repaired {
			t.Fatal("cleaning cannot recover characters Access already discarded")
		}
		if !strings.Contains(issue.Detail, "truncated") {
			t.Fatalf("detail should explain the truncation, got %q", issue.Detail)
		}
	}
}

func TestCheckTagCellFlagsCellsOverTheFieldWidth(t *testing.T) {
	long := "aerial photographs; " + strings.Repeat("b", accessTagsFieldLimit)
	result := checkTagCell(long)
	if !hasIssue(result, tagIssueFieldLimit) {
		t.Fatalf("expected a field-limit issue, got %+v", result.Issues)
	}
}

// Characters, not bytes: a German or French AAT term must not be miscounted.
func TestFieldLimitCountsCharactersNotBytes(t *testing.T) {
	// 200 two-byte runes is 400 bytes but well inside the character limit.
	input := strings.Repeat("ü", 200)
	if detail := describeFieldLimit(input, []string{input}); detail != "" {
		t.Fatalf("200 characters should be within the limit, got %q", detail)
	}
}

func TestOrdinaryCellsHaveNoFieldLimitIssue(t *testing.T) {
	result := checkTagCell("aerial photographs; landscapes; city plans")
	if hasIssue(result, tagIssueFieldLimit) {
		t.Fatalf("a short cell should not be flagged, got %+v", result.Issues)
	}
}

// Ordinary spaces are visible, do not break a tab-delimited upload, and must
// not be reported as invisible characters. This was wrong once: a cell with a
// leading space produced a blocking "ghost characters" finding whose detail
// was an empty string.
func TestOrdinarySpacingIsNotReportedAsGhostCharacters(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "leading space", input: "  aerial photographs; landscapes; city plans"},
		{name: "trailing space", input: "aerial photographs; landscapes; city plans  "},
		{name: "repeated interior spaces", input: "aerial  photographs; landscapes; city plans"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := checkTagCell(tc.input)
			if hasIssue(result, tagIssueGhostCharacters) {
				t.Fatalf("plain spaces are not ghost characters, got %+v", result.Issues)
			}
			if !hasIssue(result, tagIssueWhitespace) {
				t.Fatalf("expected a whitespace issue, got %+v", result.Issues)
			}
			if result.OriginalBlocksUpload() {
				t.Fatalf("plain spaces do not break a tab-delimited upload, got %+v", result.Issues)
			}
			for _, issue := range result.Issues {
				if issue.Detail == "" {
					t.Fatalf("issue %q has an empty detail, which says nothing in a report", issue.Kind)
				}
			}
		})
	}
}

// Every issue this module can raise has to say something useful.
func TestNoIssueEverHasAnEmptyDetail(t *testing.T) {
	for _, input := range []string{
		"  aerial photographs; landscapes; city plans",
		"aerial\u00A0photographs;landscapes",
		"landscapes; Landscapes; city plans",
		"aerial photographs;; landscapes; city plans;",
		"a; b; c; d; e; f",
		"only one tag",
		"aerial photographs; Bru\uFFFDssel; city plans",
		strings.Repeat("a", accessTagsFieldLimit+10),
	} {
		for _, issue := range checkTagCell(input).Issues {
			if strings.TrimSpace(issue.Detail) == "" {
				t.Fatalf("input %q produced issue %q with no detail", input, issue.Kind)
			}
		}
	}
}

// An invisible character still reads as blocking even alongside plain spaces.
func TestGhostCharactersStillBlockWhenMixedWithOrdinarySpaces(t *testing.T) {
	result := checkTagCell("  aerial\u00A0photographs; landscapes; city plans  ")
	if !hasIssue(result, tagIssueGhostCharacters) {
		t.Fatalf("expected a ghost-character issue, got %+v", result.Issues)
	}
	if !result.OriginalBlocksUpload() {
		t.Fatal("a no-break space blocks the upload regardless of surrounding spaces")
	}
}
