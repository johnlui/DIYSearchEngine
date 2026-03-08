package controllers

import (
	"regexp"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"
)

var asciiRegexp = regexp.MustCompile("[[:ascii:]]")

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func sortDocKeysByScore(docsScores map[string]float64) []string {
	keys := make([]string, 0, len(docsScores))
	for key := range docsScores {
		keys = append(keys, key)
	}

	sort.SliceStable(keys, func(i, j int) bool {
		return docsScores[keys[i]] > docsScores[keys[j]]
	})

	return keys
}

func briefForSearchResult(text string) string {
	brief := asciiRegexp.ReplaceAllLiteralString(text, "")

	length := 100
	briefLen := utf8.RuneCountInString(brief)
	if briefLen < 100 {
		length = briefLen
	}
	if length > 0 {
		brief = string([]rune(brief)[:length-1])
	}

	return brief
}

func sumStringInts(values []string) int {
	total := 0
	for _, value := range values {
		count, _ := strconv.Atoi(value)
		total += count
	}
	return total
}

func redisMinuteKey(prefix string, now time.Time, minuteOffset int64) string {
	return prefix + strconv.FormatInt((now.Unix()-minuteOffset)/60, 10)
}

func rollingMinuteSum(getValue func(string) int, prefix string, now time.Time, minutes int) int {
	total := 0
	for i := 0; i < minutes; i++ {
		total += getValue(prefix + strconv.FormatInt(now.Unix()/60-int64(i), 10))
	}
	return total
}
