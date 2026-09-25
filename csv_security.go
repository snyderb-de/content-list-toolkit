package main

import (
	"strconv"
	"strings"
	"unicode"
)

// spreadsheetSafeCell keeps untrusted text from being evaluated when a CSV or
// TSV is opened in a spreadsheet. The leading tab is part of the CSV value;
// callers that consume their own exports as data must remove it explicitly.
func spreadsheetSafeCell(value string) string {
	if spreadsheetFormulaRisk(value) {
		return "\t" + value
	}
	return value
}

func spreadsheetSafeRow(values []string) []string {
	safe := make([]string, len(values))
	for i, value := range values {
		safe[i] = spreadsheetSafeCell(value)
	}
	return safe
}

func spreadsheetOriginalCell(value string) string {
	if strings.HasPrefix(value, "\t") && spreadsheetFormulaRisk(value[1:]) {
		return value[1:]
	}
	return value
}

func spreadsheetFormulaRisk(value string) bool {
	if value == "" {
		return false
	}
	// A signed numeric literal is data in spreadsheet readers. In particular,
	// negative numbers in imported metadata must keep their exact value.
	if value[0] == '-' || value[0] == '+' {
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			return false
		}
	}
	for _, r := range value {
		switch r {
		case '=', '+', '-', '@', '＝', '＋', '－', '＠':
			return true
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return true
		}
		if unicode.IsSpace(r) {
			continue
		}
		return false
	}
	return true
}
