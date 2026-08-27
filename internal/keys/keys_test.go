package keys

import (
	"strings"
	"testing"
	"time"

	"github.com/johnlui/enterprise-search-engine/tools"
)

func TestKeyBuilders(t *testing.T) {
	now := time.Unix(1704067265, 0)

	if got, want := Minute(SpiderResultPerMinute, now), tools.MinuteBucketKey(SpiderResultPerMinute, now); got != want {
		t.Fatalf("Minute() = %q, want %q", got, want)
	}
	if got, want := MinuteOffset("prefix_", now, 120), "prefix_28401119"; got != want {
		t.Fatalf("MinuteOffset() = %q, want %q", got, want)
	}
	if got, want := Day(HostCountsAll, now), "host_counts_all_19723"; got != want {
		t.Fatalf("Day() = %q, want %q", got, want)
	}
	if got := CrawlFailureCount("https://fail.example"); !strings.HasPrefix(got, CrawlFailureCountPrefix) || len(got) != len(CrawlFailureCountPrefix)+32 {
		t.Fatalf("CrawlFailureCount() = %q", got)
	}
	if got, want := CrawlRateLimit("example.com", 60, now), tools.WindowBucketKey(CrawlRateLimitPrefix, "example.com", 60, now); got != want {
		t.Fatalf("CrawlRateLimit() = %q, want %q", got, want)
	}
	if got, want := StatusQueueCursor("status_01"), "table_status_01_max_into_queue_id"; got != want {
		t.Fatalf("StatusQueueCursor() = %q, want %q", got, want)
	}
}
