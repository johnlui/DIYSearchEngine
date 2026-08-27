package tools

import (
	"testing"

	"github.com/johnlui/enterprise-search-engine/models"
)

func TestCurlMalformedURLCountsTowardRetirement(t *testing.T) {
	original := countCurlFailure
	defer func() {
		countCurlFailure = original
	}()

	calls := 0
	countCurlFailure = func(status models.Status) int {
		calls++
		if status.Url != "https://example.com/\n" {
			t.Fatalf("unexpected status.Url = %q", status.Url)
		}
		if calls >= 3 {
			return 4
		}
		return 2
	}

	status := models.Status{Url: "https://example.com/\n"}

	for i := 1; i <= 2; i++ {
		_, code := Curl(status)
		if code != 2 {
			t.Fatalf("attempt %d returned %d, want 2", i, code)
		}
	}

	_, code := Curl(status)
	if code != 4 {
		t.Fatalf("third attempt returned %d, want 4", code)
	}

	if calls != 3 {
		t.Fatalf("countCurlFailure called %d times, want 3", calls)
	}
}

func TestSetCurlFailureCounter(t *testing.T) {
	original := countCurlFailure
	t.Cleanup(func() {
		countCurlFailure = original
	})

	SetCurlFailureCounter(func(models.Status) int {
		return 4
	})
	_, code := Curl(models.Status{Url: "https://example.com/\n"})
	if code != 4 {
		t.Fatalf("custom failure code = %d, want 4", code)
	}

	SetCurlFailureCounter(nil)
	_, code = Curl(models.Status{Url: "https://example.com/\n"})
	if code != 2 {
		t.Fatalf("default failure code = %d, want 2", code)
	}
}
