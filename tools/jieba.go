package tools

import (
	"regexp"
	"strings"
	"unicode"
)

var jiebaCut = fallbackCutForSearch
var fallbackSplitRegexp = regexp.MustCompile(`[\s\p{P}\p{S}]+`)

func GetFenciResultArray(s string) []string {
	return jiebaCut(s)
}

func fallbackCutForSearch(s string) []string {
	if s == "" {
		return nil
	}

	parts := fallbackSplitRegexp.Split(s, -1)
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}

		if isASCIIWord(part) {
			tokens = append(tokens, strings.ToLower(part))
			continue
		}

		runes := []rune(part)
		for _, r := range runes {
			if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
				continue
			}
			tokens = append(tokens, string(r))
		}
		for i := 0; i+1 < len(runes); i++ {
			tokens = append(tokens, string(runes[i:i+2]))
		}
	}

	return tokens
}

func isASCIIWord(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
