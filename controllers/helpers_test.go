package controllers

import (
	"reflect"
	"testing"
	"time"
)

func TestMapValues(t *testing.T) {
	values := map[string]string{
		"a": "1",
		"b": "2",
	}
	got := mapValues(values)
	if len(got) != 2 {
		t.Fatalf("mapValues() len = %d", len(got))
	}
}

func TestSortDocKeysByScore(t *testing.T) {
	got := sortDocKeysByScore(map[string]float64{
		"a": 1,
		"b": 3,
		"c": 2,
	})
	want := []string{"b", "c", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortDocKeysByScore() = %v, want %v", got, want)
	}
}

func TestUniqueStrings(t *testing.T) {
	got := uniqueStrings([]string{"foo", "bar", "foo", "", "baz", "bar"})
	want := []string{"foo", "bar", "baz"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueStrings() = %v, want %v", got, want)
	}
}

func TestParseBestDocParts(t *testing.T) {
	got := parseBestDocParts("-1,2,3,10,0-1,2,5,10,4-2,8,1,20,7-bad-")
	if len(got) != 2 {
		t.Fatalf("parseBestDocParts() len = %d", len(got))
	}

	parts := map[string]docPart{}
	for _, part := range got {
		parts[part.docKey] = part
	}

	if part := parts["1-2"]; part.termFrequency != 5 || part.docLength != 10 {
		t.Fatalf("unexpected part for 1-2: %#v", part)
	}
	if part := parts["2-8"]; part.termFrequency != 1 || part.docLength != 20 {
		t.Fatalf("unexpected part for 2-8: %#v", part)
	}
}

func TestTopDocKeysByScore(t *testing.T) {
	got := topDocKeysByScore(map[string]float64{
		"a": 1,
		"b": 5,
		"c": 3,
		"d": 4,
	}, 3)
	want := []string{"b", "d", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("topDocKeysByScore() = %v, want %v", got, want)
	}
}

func TestParseDocKey(t *testing.T) {
	tableIndex, docID, ok := parseDocKey("12-345")
	if !ok || tableIndex != 12 || docID != 345 {
		t.Fatalf("parseDocKey() = (%d, %d, %v)", tableIndex, docID, ok)
	}

	if _, _, ok := parseDocKey("bad"); ok {
		t.Fatal("expected malformed doc key to fail")
	}
}

func TestBriefForSearchResult(t *testing.T) {
	if got := briefForSearchResult("abc中文123"); got != "中" {
		t.Fatalf("briefForSearchResult() = %q", got)
	}
}

func TestSumStringInts(t *testing.T) {
	if got := sumStringInts([]string{"1", "2", "bad", "3"}); got != 6 {
		t.Fatalf("sumStringInts() = %d", got)
	}
}

func TestRedisMinuteKey(t *testing.T) {
	now := time.Unix(1700000123, 0)
	if got := redisMinuteKey("prefix_", now, 60); got != "prefix_28333334" {
		t.Fatalf("redisMinuteKey() = %q", got)
	}
}

func TestRollingMinuteSum(t *testing.T) {
	now := time.Unix(1700000123, 0)
	values := map[string]int{
		"prefix_28333335": 2,
		"prefix_28333334": 3,
		"prefix_28333333": 5,
	}
	got := rollingMinuteSum(func(key string) int {
		return values[key]
	}, "prefix_", now, 3)
	if got != 10 {
		t.Fatalf("rollingMinuteSum() = %d", got)
	}
}
