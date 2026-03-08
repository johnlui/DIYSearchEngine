package tools

import (
	"testing"
	"time"
)

func TestHexTableName(t *testing.T) {
	cases := map[string]string{
		HexTableName("pages", 0):    "pages_00",
		HexTableName("pages", 15):   "pages_0f",
		HexTableName("pages", 16):   "pages_10",
		HexTableName("status", 255): "status_ff",
	}

	for got, want := range cases {
		if got != want {
			t.Fatalf("HexTableName() = %q, want %q", got, want)
		}
	}
}

func TestMD5TableName(t *testing.T) {
	if got := MD5TableName("pages", "https://example.com"); got != "pages_c9" {
		t.Fatalf("MD5TableName() = %q", got)
	}
}

func TestMinuteBucketKey(t *testing.T) {
	now := time.Unix(1700000123, 0)
	if got := MinuteBucketKey("prefix_", now); got != "prefix_28333335" {
		t.Fatalf("MinuteBucketKey() = %q", got)
	}
}

func TestWindowBucketKey(t *testing.T) {
	now := time.Unix(1700000123, 0)
	if got := WindowBucketKey("prefix_", "example.com", 60, now); got != "prefix_example.com_60s_28333335" {
		t.Fatalf("WindowBucketKey() = %q", got)
	}
}
