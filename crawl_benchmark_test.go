package main

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func BenchmarkCollectDiscoveredLinks(b *testing.B) {
	domain1BlackList = map[string]struct{}{}

	var html strings.Builder
	html.WriteString("<html><body>")
	for i := 0; i < 2000; i++ {
		html.WriteString(`<a href="https://example.com/path/`)
		html.WriteString(strings.Repeat("a", i%5+1))
		html.WriteString(`?q=1#frag">example</a>`)
	}
	for i := 0; i < 2000; i++ {
		html.WriteString(`<a href="https://blocked.com/item">blocked</a>`)
	}
	html.WriteString("</body></html>")

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html.String()))
	if err != nil {
		b.Fatalf("NewDocumentFromReader() error = %v", err)
	}

	domain1BlackList["blocked.com"] = struct{}{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = collectDiscoveredLinks(doc)
	}
}
