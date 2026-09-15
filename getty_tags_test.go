package main

import (
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
