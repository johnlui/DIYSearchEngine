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
